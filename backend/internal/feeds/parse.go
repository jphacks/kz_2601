package feeds

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// Text caps applied to everything taken from a source.
const (
	maxTitleRunes    = 200
	maxSubtitleRunes = 120
	maxURLBytes      = 2048
)

// clean removes control / invisible characters, folds whitespace and clips.
func clean(s string, maxRunes int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case r == utf8.RuneError:
			continue
		case unicode.IsSpace(r):
			space = b.Len() > 0
			continue
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	out := b.String()
	if utf8.RuneCountInString(out) > maxRunes {
		out = string([]rune(out)[:maxRunes-1]) + "…"
	}
	return out
}

// httpsURL keeps an https link without credentials; anything else is dropped.
func httpsURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) > maxURLBytes {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	return u.String()
}

// ---- weather (気象庁 forecast/{office}.json) --------------------------------

type jmaReport struct {
	TimeSeries []struct {
		TimeDefines []string `json:"timeDefines"`
		Areas       []struct {
			WeatherCodes []string `json:"weatherCodes"`
			Weathers     []string `json:"weathers"`
			Pops         []string `json:"pops"`
			TempsMin     []string `json:"tempsMin"`
			TempsMax     []string `json:"tempsMax"`
		} `json:"areas"`
	} `json:"timeSeries"`
}

// parseWeather reads the daily rows: the short-term report (index 0) gives
// today's and tomorrow's wording, the weekly report (index 1) the 7 days with
// probability of precipitation and min / max. The first area of each series
// (the office's main area) is used.
func parseWeather(body []byte) ([]Item, error) {
	var reports []jmaReport
	if err := json.Unmarshal(body, &reports); err != nil {
		return nil, fmt.Errorf("weather: %w", err)
	}
	days := map[string]*Item{}
	day := func(t string) *Item {
		if len(t) < 10 {
			return nil
		}
		d := t[:10]
		if _, err := time.Parse(time.DateOnly, d); err != nil {
			return nil
		}
		if days[d] == nil {
			days[d] = &Item{Date: d}
		}
		return days[d]
	}
	at := func(xs []string, i int) string {
		if i < len(xs) {
			return strings.TrimSpace(xs[i])
		}
		return ""
	}
	if len(reports) > 1 {
		ts := reports[1].TimeSeries
		if len(ts) > 0 && len(ts[0].Areas) > 0 {
			a := ts[0].Areas[0]
			for i, t := range ts[0].TimeDefines {
				if it := day(t); it != nil {
					setCode(it, at(a.WeatherCodes, i))
					if p, err := strconv.Atoi(at(a.Pops, i)); err == nil && p >= 0 && p <= 100 {
						it.Pop = &p
					}
				}
			}
		}
		if len(ts) > 1 && len(ts[1].Areas) > 0 {
			a := ts[1].Areas[0]
			for i, t := range ts[1].TimeDefines {
				if it := day(t); it != nil {
					it.Min, it.Max = temp(at(a.TempsMin, i)), temp(at(a.TempsMax, i))
				}
			}
		}
	}
	if len(reports) > 0 {
		ts := reports[0].TimeSeries
		if len(ts) > 0 && len(ts[0].Areas) > 0 {
			a := ts[0].Areas[0]
			for i, t := range ts[0].TimeDefines {
				if it := day(t); it != nil {
					setCode(it, at(a.WeatherCodes, i))
					if w := clean(strings.ReplaceAll(at(a.Weathers, i), "　", " "), maxSubtitleRunes); w != "" {
						it.Subtitle = w
					}
				}
			}
		}
	}
	out := make([]Item, 0, len(days))
	for _, it := range days {
		if it.Title != "" {
			out = append(out, *it)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("weather: no days")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

func temp(s string) *float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < -60 || v > 60 {
		return nil
	}
	return &v
}

// setCode maps a JMA weather code (100s 晴 / 200s 曇 / 300s 雨 / 400s 雪)
// to a short title and an SF Symbol.
func setCode(it *Item, code string) {
	if len(code) != 3 {
		return
	}
	switch code[0] {
	case '1':
		it.Title, it.Icon = "晴れ", "sun.max"
	case '2':
		it.Title, it.Icon = "くもり", "cloud"
	case '3':
		it.Title, it.Icon = "雨", "cloud.rain"
	case '4':
		it.Title, it.Icon = "雪", "cloud.snow"
	}
}

// ---- holidays (内閣府 syukujitsu.csv, Shift_JIS) ------------------------------

func parseHolidays(body []byte) ([]Item, error) {
	r := csv.NewReader(transform.NewReader(bytes.NewReader(body), japanese.ShiftJIS.NewDecoder()))
	r.FieldsPerRecord = -1
	var out []Item
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("holidays: %w", err)
		}
		if len(rec) < 2 {
			continue
		}
		d, err := time.Parse("2006/1/2", strings.TrimSpace(rec[0]))
		if err != nil {
			continue // header
		}
		name := clean(rec[1], 40)
		if name == "" {
			continue
		}
		out = append(out, Item{Date: d.Format(time.DateOnly), Title: name, Subtitle: weekdayJA[d.Weekday()] + "曜日"})
	}
	if len(out) == 0 {
		return nil, errors.New("holidays: no rows")
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

var weekdayJA = [...]string{"日", "月", "火", "水", "木", "金", "土"}

// ---- rss (RSS 2.0 / RSS 1.0 RDF / Atom) -----------------------------------

type xmlLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Text string `xml:",chardata"`
}

type xmlEntry struct {
	Title     string    `xml:"title"`
	Links     []xmlLink `xml:"link"`
	PubDate   string    `xml:"pubDate"`
	DCDate    string    `xml:"http://purl.org/dc/elements/1.1/ date"`
	Updated   string    `xml:"updated"`
	Published string    `xml:"published"`
}

type xmlFeed struct {
	Channel struct {
		Items []xmlEntry `xml:"item"`
	} `xml:"channel"`
	Items   []xmlEntry `xml:"item"`  // RSS 1.0: items are siblings of channel
	Entries []xmlEntry `xml:"entry"` // Atom
}

func parseRSS(body []byte) ([]Item, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.CharsetReader = charsetReader
	var f xmlFeed
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("rss: %w", err)
	}
	entries := append(append(f.Channel.Items, f.Items...), f.Entries...)
	out := make([]Item, 0, len(entries))
	for _, e := range entries {
		title := clean(e.Title, maxTitleRunes)
		if title == "" {
			continue
		}
		it := Item{Title: title, URL: entryLink(e.Links)}
		for _, raw := range []string{e.PubDate, e.DCDate, e.Updated, e.Published} {
			if t, ok := parseFeedTime(raw); ok {
				it.Date = t.In(jst).Format(time.RFC3339)
				break
			}
		}
		out = append(out, it)
		if len(out) >= maxStoredItems {
			break
		}
	}
	// Newest first; undated items keep their place after the dated ones.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}

func entryLink(ls []xmlLink) string {
	for _, l := range ls {
		if l.Rel == "" || l.Rel == "alternate" {
			if u := httpsURL(l.Href); u != "" {
				return u
			}
			if u := httpsURL(l.Text); u != "" {
				return u
			}
		}
	}
	return ""
}

var feedTimeLayouts = []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", "2006-01-02T15:04:05Z07:00", "2006-01-02"}

func parseFeedTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, l := range feedTimeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// charsetReader accepts the encodings Japanese public sites use.
func charsetReader(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "utf-8", "utf8", "":
		return in, nil
	case "shift_jis", "shift-jis", "sjis", "windows-31j", "cp932":
		return transform.NewReader(in, japanese.ShiftJIS.NewDecoder()), nil
	case "euc-jp":
		return transform.NewReader(in, japanese.EUCJP.NewDecoder()), nil
	}
	return nil, fmt.Errorf("rss: unsupported charset %q", label)
}
