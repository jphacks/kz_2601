package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

// 「無い機能を頼まれた」 (#1769): the AI service writes one ai.unsupported
// line each time a generation declares that it replaced or could not do a
// requested part (internal/ai/unsupported_log.go). A finished UTC day of
// lines with the same requested phrase becomes one inbox item: the phrase,
// the substitute, the count and the platforms. The app hash is used only to
// count distinct apps; it never reaches the item, nor does uid_hash.

// EnvUnsupportedOn turns the source on.
const EnvUnsupportedOn = "LOOP_SOURCE_UNSUPPORTED"

// EnvUnsupportedLogs: comma-separated `gcloud logging read --format=json`
// exports of ai.unsupported lines, read when LOOP_GENFAIL_SOURCE is file
// (the default). With LOOP_GENFAIL_SOURCE=logging the Cloud Logging reader
// of the generation failures (LOOP_GENFAIL_PROJECT / _SERVICES / _MAX_LINES)
// is used instead.
const EnvUnsupportedLogs = "LOOP_UNSUPPORTED_LOGS"

// ErrUnsupportedOff is returned by UnsupportedConfigFromEnv when the source is off.
var ErrUnsupportedOff = errors.New("unsupported-feature declarations are off (" + EnvUnsupportedOn + " != on)")

const unsupportedEvent = "ai.unsupported"

// UnsupportedEvent is one ai.unsupported line. Requested / Substitute are
// masked by the writer and masked again at ingest.
type UnsupportedEvent struct {
	At         time.Time
	Requested  string
	Substitute string
	Platform   string
	Mode       string
	// AppHash is only ever counted (distinct apps); it is not stored.
	AppHash string
}

// UnsupportedLogSource reads ai.unsupported lines in [since, until].
type UnsupportedLogSource interface {
	UnsupportedEvents(ctx context.Context, since, until time.Time) ([]UnsupportedEvent, error)
}

// UnsupportedLogFiles reads exports (the same shapes as GenLogFiles).
type UnsupportedLogFiles struct{ Paths []string }

// UnsupportedEvents implements UnsupportedLogSource.
func (u UnsupportedLogFiles) UnsupportedEvents(_ context.Context, since, until time.Time) ([]UnsupportedEvent, error) {
	var out []UnsupportedEvent
	for _, p := range u.Paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		evs, err := ParseUnsupportedLogs(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, evs...)
	}
	return unsupportedInRange(out, since, until), nil
}

// StaticUnsupportedLogs is an UnsupportedLogSource over a fixed list (tests).
type StaticUnsupportedLogs []UnsupportedEvent

// UnsupportedEvents implements UnsupportedLogSource.
func (s StaticUnsupportedLogs) UnsupportedEvents(_ context.Context, since, until time.Time) ([]UnsupportedEvent, error) {
	return unsupportedInRange(s, since, until), nil
}

func unsupportedInRange(evs []UnsupportedEvent, since, until time.Time) []UnsupportedEvent {
	out := []UnsupportedEvent{}
	for _, e := range evs {
		if !e.At.Before(since) && !e.At.After(until) {
			out = append(out, e)
		}
	}
	return out
}

// ParseUnsupportedLogs decodes the ai.unsupported lines of an export (a JSON
// array of entries, or JSON lines of entries or bare slog records). Other
// events and lines that do not decode are skipped.
func ParseUnsupportedLogs(r io.Reader) ([]UnsupportedEvent, error) {
	raws, err := splitLogRecords(r)
	if err != nil {
		return nil, err
	}
	out := []UnsupportedEvent{}
	for _, raw := range raws {
		if ev, ok := decodeUnsupportedEntry(raw); ok {
			out = append(out, ev)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// unsupportedPayload holds only the fields the source reads (never uid_hash).
type unsupportedPayload struct {
	Time       string `json:"time"`
	Event      string `json:"event"`
	Requested  string `json:"requested"`
	Substitute string `json:"substitute"`
	Platform   string `json:"platform"`
	Mode       string `json:"mode"`
	AppHash    string `json:"app_hash"`
}

// maxUnsupportedRunes bounds a phrase read from the log.
const maxUnsupportedRunes = 160

func decodeUnsupportedEntry(raw json.RawMessage) (UnsupportedEvent, bool) {
	var e struct {
		Timestamp   string              `json:"timestamp"`
		JSONPayload *unsupportedPayload `json:"jsonPayload"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return UnsupportedEvent{}, false
	}
	p, ts := e.JSONPayload, e.Timestamp
	if p == nil { // bare slog record
		p = &unsupportedPayload{}
		if err := json.Unmarshal(raw, p); err != nil {
			return UnsupportedEvent{}, false
		}
		ts = p.Time
	}
	if p.Event != unsupportedEvent {
		return UnsupportedEvent{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return UnsupportedEvent{}, false
	}
	req := truncate(strings.TrimSpace(p.Requested), maxUnsupportedRunes)
	if req == "" {
		return UnsupportedEvent{}, false
	}
	return UnsupportedEvent{
		At: at.UTC(), Requested: req, Substitute: truncate(strings.TrimSpace(p.Substitute), maxUnsupportedRunes),
		Platform: plainLabel(p.Platform), Mode: plainLabel(p.Mode), AppHash: plainLabel(p.AppHash),
	}, true
}

// UnsupportedGroup is one phrase on one finished UTC day.
type UnsupportedGroup struct {
	Day       string // 2006-01-02 (UTC)
	Requested string // the first spelling seen
	// Substitutes are the distinct substitutes seen (at most 3 kept).
	Substitutes []string
	Count       int
	Apps        int // distinct app hashes (0 when the lines had none)
	Platforms   map[string]int
	Last        time.Time
}

// GroupUnsupported groups events by (UTC day, normalised phrase), keeping
// only days that ended at or before until, so a day's count is final when
// it is ingested (the item is written once; later lines of that day would
// not update it).
func GroupUnsupported(evs []UnsupportedEvent, until time.Time) []UnsupportedGroup {
	type key struct{ day, norm string }
	groups := map[key]*UnsupportedGroup{}
	apps := map[key]map[string]bool{}
	cut := until.UTC().Truncate(24 * time.Hour)
	for _, e := range evs {
		if !e.At.Before(cut) {
			continue
		}
		norm := normalizePhrase(e.Requested)
		if norm == "" {
			continue
		}
		k := key{e.At.UTC().Format("2006-01-02"), norm}
		g, ok := groups[k]
		if !ok {
			g = &UnsupportedGroup{Day: k.day, Requested: e.Requested, Platforms: map[string]int{}}
			groups[k] = g
			apps[k] = map[string]bool{}
		}
		g.Count++
		if e.At.After(g.Last) {
			g.Last = e.At
		}
		if e.Platform != "" {
			g.Platforms[e.Platform]++
		}
		if s := strings.TrimSpace(e.Substitute); s != "" && len(g.Substitutes) < 3 && !containsFold(g.Substitutes, s) {
			g.Substitutes = append(g.Substitutes, s)
		}
		if e.AppHash != "" {
			apps[k][e.AppHash] = true
		}
	}
	out := make([]UnsupportedGroup, 0, len(groups))
	for k, g := range groups {
		g.Apps = len(apps[k])
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Day != out[j].Day {
			return out[i].Day < out[j].Day
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Requested < out[j].Requested
	})
	return out
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if normalizePhrase(x) == normalizePhrase(s) {
			return true
		}
	}
	return false
}

// normalizePhrase lowercases and drops spaces and punctuation, so "カレンダー 表示"
// and "カレンダー表示" are one phrase.
func normalizePhrase(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// UnsupportedRaw is the inbox form of one group: a fixed template over the
// masked phrases and counts. One item per phrase and day.
func UnsupportedRaw(g UnsupportedGroup) Raw {
	sum := sha256.Sum256([]byte(normalizePhrase(g.Requested)))
	sub := "代替なし"
	if len(g.Substitutes) > 0 {
		sub = strings.Join(g.Substitutes, " / ") + " で代替"
	}
	parts := []string{fmt.Sprintf("%d 件", g.Count)}
	if g.Apps > 0 {
		parts = append(parts, fmt.Sprintf("%d つのアプリ", g.Apps))
	}
	if len(g.Platforms) > 0 {
		names := make([]string, 0, len(g.Platforms))
		for p := range g.Platforms {
			names = append(names, p)
		}
		sort.Strings(names)
		ps := make([]string, len(names))
		for i, p := range names {
			ps[i] = fmt.Sprintf("%s %d", p, g.Platforms[p])
		}
		parts = append(parts, strings.Join(ps, " / "))
	}
	return Raw{
		Source: loop.SourceUnsupported, SourceID: hex.EncodeToString(sum[:8]) + "@" + g.Day,
		// the day stays out of the text: a date is masked as a number and
		// would push a short item over the mask ratio (it is in SourceID
		// and CreatedAt)
		Text: fmt.Sprintf("無い機能を頼まれた(生成が代替・申告): 「%s」(%s)・%s", g.Requested, sub, strings.Join(parts, "・")),
		Kind: "unsupported", Count: g.Count, CreatedAt: g.Last,
	}
}

// Unsupported adapts an UnsupportedLogSource to Fetcher.
type Unsupported struct {
	Logs UnsupportedLogSource
	Now  func() time.Time
}

// Source implements Fetcher.
func (Unsupported) Source() loop.Source { return loop.SourceUnsupported }

// Fetch implements Fetcher: the groups of the finished days in [since, now].
func (u Unsupported) Fetch(ctx context.Context, since time.Time) ([]Raw, error) {
	now := time.Now
	if u.Now != nil {
		now = u.Now
	}
	end := now().UTC()
	if end.Before(since) {
		return []Raw{}, nil
	}
	evs, err := u.Logs.UnsupportedEvents(ctx, since, end)
	if err != nil {
		return nil, err
	}
	groups := GroupUnsupported(evs, end)
	out := make([]Raw, 0, len(groups))
	for _, g := range groups {
		out = append(out, UnsupportedRaw(g))
	}
	return out, nil
}

// UnsupportedConfigFromEnv reads LOOP_SOURCE_UNSUPPORTED and picks the log
// reader: LOOP_UNSUPPORTED_LOGS files, or with LOOP_GENFAIL_SOURCE=logging
// the generation failures' Cloud Logging reader. On without logs is an error.
func UnsupportedConfigFromEnv(getenv func(string) string) (Unsupported, error) {
	if getenv(EnvUnsupportedOn) != "on" {
		return Unsupported{}, ErrUnsupportedOff
	}
	switch src := strings.TrimSpace(getenv(EnvGenFailSource)); src {
	case "", "file":
		paths := []string{}
		for p := range ParseList(getenv(EnvUnsupportedLogs)) {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		if len(paths) == 0 {
			return Unsupported{}, fmt.Errorf("%s=on needs %s (gcloud logging read exports) or %s=logging", EnvUnsupportedOn, EnvUnsupportedLogs, EnvGenFailSource)
		}
		return Unsupported{Logs: UnsupportedLogFiles{Paths: paths}}, nil
	case "logging":
		api, err := genLogAPIFromEnv(getenv)
		if err != nil {
			return Unsupported{}, err
		}
		return Unsupported{Logs: api}, nil
	default:
		return Unsupported{}, fmt.Errorf("%s: want file or logging, got %q", EnvGenFailSource, src)
	}
}
