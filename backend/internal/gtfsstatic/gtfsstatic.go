// Package gtfsstatic reads the parts of a static GTFS-JP zip that the bus
// ops need (#1692): stop and route names and the scheduled time of each
// trip at each stop. It only reads; fetching the zip and keeping it fresh
// are left to the caller.
//
// The zip comes from an operator, so it is treated as untrusted: the zip
// size, each file's decompressed size, the total decompressed size and each
// file's row count are capped (a zip bomb stops at the cap, not at the
// size its header claims). CSV may carry a UTF-8 BOM, CRLF line ends and
// quoted fields.
package gtfsstatic

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Limits caps what one zip may hold. A zero field takes the default.
type Limits struct {
	ZipBytes  int64 // the zip itself
	FileBytes int64 // one file, decompressed
	Total     int64 // all read files, decompressed
	Rows      int   // data rows of one file
}

// DefaultLimits fit a large city operator (stop_times of a few hundred
// thousand rows) with room to spare.
var DefaultLimits = Limits{ZipBytes: 64 << 20, FileBytes: 256 << 20, Total: 512 << 20, Rows: 2_000_000}

func (l Limits) orDefault() Limits {
	if l.ZipBytes <= 0 {
		l.ZipBytes = DefaultLimits.ZipBytes
	}
	if l.FileBytes <= 0 {
		l.FileBytes = DefaultLimits.FileBytes
	}
	if l.Total <= 0 {
		l.Total = DefaultLimits.Total
	}
	if l.Rows <= 0 {
		l.Rows = DefaultLimits.Rows
	}
	return l
}

// MaxNameRunes cuts a long stop or route name.
const MaxNameRunes = 64

// maxHours bounds a stop time's hours (GTFS allows past 24:00 for trips
// that run after midnight).
const maxHours = 72

// Key is one stop of one trip.
type Key struct{ Trip, Stop string }

// Table is what was read. Times are seconds from the service day's
// "noon minus 12h" (GTFS), so 25:10:00 is 90600.
type Table struct {
	Stops     map[string]string // stop_id -> stop_name
	Routes    map[string]string // route_id -> short name, else long name
	TripRoute map[string]string // trip_id -> route_id
	Times     map[Key]int       // (trip_id, stop_id) -> arrival, else departure; the first visit
}

// StopName is the stop's name, or "" when unknown.
func (t *Table) StopName(id string) string {
	if t == nil {
		return ""
	}
	return t.Stops[id]
}

// RouteName is the route's name, or "" when unknown.
func (t *Table) RouteName(id string) string {
	if t == nil {
		return ""
	}
	return t.Routes[id]
}

// RouteOf is the trip's route_id, or "" when unknown.
func (t *Table) RouteOf(trip string) string {
	if t == nil {
		return ""
	}
	return t.TripRoute[trip]
}

// Scheduled is the trip's time at the stop.
func (t *Table) Scheduled(trip, stop string) (int, bool) {
	if t == nil {
		return 0, false
	}
	s, ok := t.Times[Key{trip, stop}]
	return s, ok
}

var (
	ErrTooLarge = errors.New("gtfsstatic: over the size or row limit")
	ErrBad      = errors.New("gtfsstatic: not a GTFS zip")
)

// ReadFile reads a zip on disk.
func ReadFile(path string, lim Limits) (*Table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return Read(f, st.Size(), lim)
}

// Read reads a zip of size bytes.
func Read(r io.ReaderAt, size int64, lim Limits) (*Table, error) {
	lim = lim.orDefault()
	if size > lim.ZipBytes {
		return nil, ErrTooLarge
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBad, err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		// GTFS files sit at the root; some zips nest them in one folder.
		name := f.Name[strings.LastIndex(f.Name, "/")+1:]
		if _, dup := files[name]; !dup {
			files[name] = f
		}
	}
	rd := &reader{lim: lim, budget: lim.Total}
	t := &Table{Stops: map[string]string{}, Routes: map[string]string{}, TripRoute: map[string]string{}, Times: map[Key]int{}}
	steps := []struct {
		name string
		cols []string
		need func(has []bool) bool // whether the header has what is needed
		row  func([]string) error
	}{
		{"stops.txt", []string{"stop_id", "stop_name"}, all, func(v []string) error {
			if v[0] != "" {
				t.Stops[v[0]] = name(v[1])
			}
			return nil
		}},
		{"routes.txt", []string{"route_id", "route_short_name", "route_long_name"}, func(h []bool) bool { return h[0] && (h[1] || h[2]) }, func(v []string) error {
			if v[0] != "" {
				n := name(v[1])
				if n == "" {
					n = name(v[2])
				}
				t.Routes[v[0]] = n
			}
			return nil
		}},
		{"trips.txt", []string{"trip_id", "route_id"}, all, func(v []string) error {
			if v[0] != "" {
				t.TripRoute[v[0]] = v[1]
			}
			return nil
		}},
		{"stop_times.txt", []string{"trip_id", "stop_id", "arrival_time", "departure_time"}, func(h []bool) bool { return h[0] && h[1] && (h[2] || h[3]) }, func(v []string) error {
			if v[0] == "" || v[1] == "" {
				return nil
			}
			at := v[2]
			if at == "" {
				at = v[3]
			}
			if at == "" {
				return nil // a stop without a time (not a timepoint)
			}
			s, err := ParseTime(at)
			if err != nil {
				return err
			}
			k := Key{v[0], v[1]}
			if _, seen := t.Times[k]; !seen {
				t.Times[k] = s
			}
			return nil
		}},
	}
	for _, s := range steps {
		f, ok := files[s.name]
		if !ok {
			return nil, fmt.Errorf("%w: %s is missing", ErrBad, s.name)
		}
		if err := rd.file(f, s.cols, s.need, s.row); err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return t, nil
}

type reader struct {
	lim    Limits
	budget int64 // decompressed bytes left for all files
}

// all needs every column.
func all(has []bool) bool {
	for _, h := range has {
		if !h {
			return false
		}
	}
	return true
}

// file reads one CSV file; row gets the wanted columns in cols order ("" for
// a column the file does not have).
func (rd *reader) file(f *zip.File, cols []string, need func([]bool) bool, row func([]string) error) error {
	capBytes := min(rd.lim.FileBytes, rd.budget)
	if f.UncompressedSize64 > uint64(capBytes) {
		return ErrTooLarge // the header already says too much
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBad, err)
	}
	defer func() { _ = rc.Close() }()
	// The header's size is not trusted: read at most cap+1 and count.
	lr := &countReader{r: io.LimitReader(rc, capBytes+1)}
	br := bufio.NewReader(lr)
	if b, _ := br.Peek(3); bytes.Equal(b, []byte("\xef\xbb\xbf")) {
		_, _ = br.Discard(3) // a UTF-8 BOM, before any quote
	}
	cr := csv.NewReader(br)
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = true
	cr.LazyQuotes = true

	head, err := cr.Read()
	if err != nil {
		if lr.n > capBytes {
			return ErrTooLarge
		}
		return fmt.Errorf("%w: no header", ErrBad)
	}
	idx := make([]int, len(cols))
	has := make([]bool, len(cols))
	for i, c := range cols {
		idx[i] = -1
		for j, h := range head {
			if strings.TrimSpace(h) == c {
				idx[i], has[i] = j, true
				break
			}
		}
	}
	if !need(has) {
		return fmt.Errorf("%w: a required column is missing", ErrBad)
	}
	vals := make([]string, len(cols))
	for rows := 0; ; rows++ {
		rec, err := cr.Read()
		if lr.n > capBytes {
			return ErrTooLarge
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrBad, err)
		}
		if rows >= rd.lim.Rows {
			return ErrTooLarge
		}
		for i, j := range idx {
			vals[i] = ""
			if j >= 0 && j < len(rec) {
				vals[i] = strings.TrimSpace(rec[j])
			}
		}
		if err := row(vals); err != nil {
			return err
		}
	}
	rd.budget -= lr.n
	return nil
}

type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// name is a stop or route name made valid UTF-8 and at most MaxNameRunes.
func name(s string) string {
	s = strings.TrimSpace(strings.ToValidUTF8(s, ""))
	if r := []rune(s); len(r) > MaxNameRunes {
		return string(r[:MaxNameRunes])
	}
	return s
}

// ParseTime reads a GTFS time "H:MM:SS" / "HH:MM:SS" (hours may pass 24)
// into seconds.
func ParseTime(s string) (int, error) {
	p := bytes.Split([]byte(s), []byte(":"))
	if len(p) != 3 || len(p[0]) < 1 || len(p[0]) > 2 || len(p[1]) != 2 || len(p[2]) != 2 {
		return 0, fmt.Errorf("%w: time %q", ErrBad, s)
	}
	var n [3]int
	for i, b := range p {
		v, err := strconv.Atoi(string(b))
		if err != nil || v < 0 {
			return 0, fmt.Errorf("%w: time %q", ErrBad, s)
		}
		n[i] = v
	}
	if n[0] >= maxHours || n[1] > 59 || n[2] > 59 {
		return 0, fmt.Errorf("%w: time %q", ErrBad, s)
	}
	return n[0]*3600 + n[1]*60 + n[2], nil
}
