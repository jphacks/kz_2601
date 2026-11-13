package gtfsstatic

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// zipDir zips the files of testdata/<dir> (under prefix inside the zip).
func zipDir(t *testing.T, dir, prefix string) []byte {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join("testdata", dir))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join("testdata", dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(prefix + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(b)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// zipFiles zips name -> content.
func zipFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range files {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(c))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func read(b []byte, lim Limits) (*Table, error) { return Read(bytes.NewReader(b), int64(len(b)), lim) }

func TestReadBasic(t *testing.T) {
	for _, prefix := range []string{"", "feed/"} { // some zips nest the files in a folder
		tb, err := read(zipDir(t, "basic", prefix), Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if tb.StopName("S1") != "香林坊" || tb.StopName("S3") != "金沢駅東口" || tb.StopName("S9") != "" {
			t.Fatalf("stops %v", tb.Stops)
		}
		// short name first, long name when there is none
		if tb.RouteName("R1") != "10" || tb.RouteName("R2") != "兼六園シャトル" {
			t.Fatalf("routes %v", tb.Routes)
		}
		if tb.TripRoute["T3"] != "R2" {
			t.Fatalf("trips %v", tb.TripRoute)
		}
		for k, want := range map[Key]int{
			{"T1", "S1"}: 8*3600 + 30*60,
			{"T1", "S2"}: 8*3600 + 36*60, // arrival before departure
			{"T2", "S1"}: 24*3600 + 10*60,
			{"T3", "S1"}: 9*3600 + 5*60, // the first visit of a loop
		} {
			if got, ok := tb.Scheduled(k.Trip, k.Stop); !ok || got != want {
				t.Fatalf("%v = %d %v, want %d", k, got, ok, want)
			}
		}
		if _, ok := tb.Scheduled("T1", "S3"); ok {
			t.Fatal("a stop without a time has one")
		}
	}
}

func TestReadMessyCSV(t *testing.T) {
	tb, err := read(zipDir(t, "messy", ""), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if tb.StopName("S1") != "香林坊, 南口" || tb.StopName("S2") != `"武蔵" ヶ辻` {
		t.Fatalf("stops %q", tb.Stops)
	}
	if tb.RouteName("R1") != "香林坊-金沢駅" {
		t.Fatalf("routes %q", tb.Routes)
	}
	if s, ok := tb.Scheduled("T1", "S2"); !ok || s != 8*3600+36*60 {
		t.Fatalf("departure only: %d %v", s, ok)
	}
}

func TestNilTable(t *testing.T) {
	var tb *Table
	if tb.StopName("S1") != "" || tb.RouteName("R1") != "" {
		t.Fatal("nil table has names")
	}
	if _, ok := tb.Scheduled("T1", "S1"); ok {
		t.Fatal("nil table has times")
	}
}

func TestReadRefusesBadZips(t *testing.T) {
	good := map[string]string{
		"stops.txt":      "stop_id,stop_name\nS1,a\n",
		"routes.txt":     "route_id,route_short_name\nR1,1\n",
		"trips.txt":      "trip_id,route_id\nT1,R1\n",
		"stop_times.txt": "trip_id,stop_id,arrival_time,departure_time\nT1,S1,08:00:00,08:00:00\n",
	}
	if _, err := read(zipFiles(t, good), Limits{}); err != nil {
		t.Fatal(err)
	}
	with := func(name, body string) map[string]string {
		m := map[string]string{}
		for k, v := range good {
			m[k] = v
		}
		if body == "" {
			delete(m, name)
		} else {
			m[name] = body
		}
		return m
	}
	for name, files := range map[string]map[string]string{
		"no stop_times":    with("stop_times.txt", ""),
		"no stop_name":     with("stops.txt", "stop_id,stop_desc\nS1,a\n"),
		"no route name":    with("routes.txt", "route_id,route_type\nR1,3\n"),
		"no times":         with("stop_times.txt", "trip_id,stop_id\nT1,S1\n"),
		"bad time":         with("stop_times.txt", "trip_id,stop_id,arrival_time\nT1,S1,8:0:0\n"),
		"time too late":    with("stop_times.txt", "trip_id,stop_id,arrival_time\nT1,S1,99:00:00\n"),
		"empty stops file": with("stops.txt", " "),
	} {
		if _, err := read(zipFiles(t, files), Limits{}); !errors.Is(err, ErrBad) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := read([]byte("not a zip"), Limits{}); !errors.Is(err, ErrBad) {
		t.Fatalf("not a zip: %v", err)
	}
}

func TestReadLimits(t *testing.T) {
	z := zipDir(t, "basic", "")
	for name, lim := range map[string]Limits{
		"zip":   {ZipBytes: int64(len(z)) - 1},
		"rows":  {Rows: 6}, // stop_times.txt has 7
		"file":  {FileBytes: 100},
		"total": {Total: 400},
	} {
		if _, err := read(z, lim); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := read(z, Limits{Rows: 7}); err != nil {
		t.Fatalf("at the row limit: %v", err)
	}
}

// A small zip that inflates to a lot stops at the cap.
func TestReadZipBomb(t *testing.T) {
	rows := "trip_id,stop_id,arrival_time\n" + strings.Repeat("T1,S1,08:00:00\n", 1<<20) // ~15 MB
	files := map[string]string{
		"stops.txt": "stop_id,stop_name\nS1,a\n", "routes.txt": "route_id,route_short_name\nR1,1\n",
		"trips.txt": "trip_id,route_id\nT1,R1\n", "stop_times.txt": rows,
	}
	z := zipFiles(t, files)
	if len(z) > 1<<20 {
		t.Fatalf("zip %d bytes", len(z))
	}
	if _, err := read(z, Limits{FileBytes: 1 << 20}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("header size: %v", err)
	}

	// The header lies (says 64 bytes): the read still stops.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range files {
		var comp bytes.Buffer
		fw, _ := flate.NewWriter(&comp, flate.BestCompression)
		_, _ = fw.Write([]byte(c))
		_ = fw.Close()
		size := uint64(len(c))
		if n == "stop_times.txt" {
			size = 64
		}
		w, err := zw.CreateRaw(&zip.FileHeader{Name: n, Method: zip.Deflate, CRC32: crc32.ChecksumIEEE([]byte(c)),
			CompressedSize64: uint64(comp.Len()), UncompressedSize64: size})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(comp.Bytes())
	}
	_ = zw.Close()
	if _, err := read(buf.Bytes(), Limits{FileBytes: 1 << 20}); err == nil {
		t.Fatal("a lying header was read")
	}
}

func TestParseTime(t *testing.T) {
	for s, want := range map[string]int{"0:00:00": 0, "08:30:15": 30615, "25:10:00": 90600, "71:59:59": 259199} {
		if got, err := ParseTime(s); err != nil || got != want {
			t.Fatalf("%s = %d %v", s, got, err)
		}
	}
	for _, s := range []string{"", "8:30", "08:60:00", "08:30:60", "-1:00:00", "123:00:00", "72:00:00", "aa:bb:cc", "08:3:00"} {
		if _, err := ParseTime(s); err == nil {
			t.Fatalf("%q accepted", s)
		}
	}
}
