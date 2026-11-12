package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/googleapi"
	logging "google.golang.org/api/logging/v2"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// InApp reads feedback/{id} (#319).
type InApp struct {
	Reader store.FeedbackReader
	// Limit per run (<= 500, store's cap). Default 500.
	Limit int
}

// Source implements Fetcher.
func (InApp) Source() loop.Source { return loop.SourceInApp }

// Fetch implements Fetcher. The screenshot and DSL are never copied: only
// whether they exist (#367: スクショ・DSL は渡さない).
func (a InApp) Fetch(ctx context.Context, since time.Time) ([]Raw, error) {
	limit := a.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	fbs, err := a.Reader.ListFeedback(ctx, store.FeedbackQuery{Since: since, Limit: limit})
	if err != nil {
		return nil, err
	}
	if len(fbs) == limit {
		slog.Warn("loop ingest: in-app page is full; older feedback after the cursor may be skipped", "limit", limit)
	}
	out := make([]Raw, 0, len(fbs))
	for _, fb := range fbs {
		r := Raw{
			Source: loop.SourceInApp, SourceID: fb.ID, UID: fb.UID, Text: fb.Message, Kind: fb.Kind,
			ConsentVersion: fb.ConsentVersion, CreatedAt: fb.CreatedAt, SourceRetention: store.FeedbackRetention,
			HasScreenshot: slices.Contains(fb.Listed, "screenshot"), HasDSL: slices.Contains(fb.Listed, "dsl") || fb.AppID != "",
		}
		if d := fb.Device; d != nil {
			r.AppVersion, r.OSMajor = d.AppVersion, OSMajor(d.OS)
		}
		out = append(out, r)
	}
	return out, nil
}

// CrashSpike is a jump in one crash issue, recorded as one item (#368:
// 「クラッシュの急増を 1 件のフィードバックとして」). Only the issue id, its
// type, the count and the app version are kept (#367: no uid, no stack).
type CrashSpike struct {
	IssueID    string
	Type       string // e.g. "EXC_BAD_ACCESS" / the exception class
	Count      int
	AppVersion string
	At         time.Time
}

// CrashSource is the hook a Crashlytics reader implements. Crashlytics has no
// cheap query API: the planned reader is its BigQuery export (#296), which is
// not set up yet. Until then an operator records a spike by hand with
// `feedback-loop ingest-crash` (StaticCrashes).
type CrashSource interface {
	Spikes(ctx context.Context, since time.Time) ([]CrashSpike, error)
}

// StaticCrashes is a CrashSource over a fixed list (the CLI and tests).
type StaticCrashes []CrashSpike

// Spikes implements CrashSource.
func (s StaticCrashes) Spikes(_ context.Context, since time.Time) ([]CrashSpike, error) {
	out := []CrashSpike{}
	for _, c := range s {
		if !c.At.Before(since) {
			out = append(out, c)
		}
	}
	return out, nil
}

// Crashes adapts a CrashSource to Fetcher.
type Crashes struct{ Src CrashSource }

// Source implements Fetcher.
func (Crashes) Source() loop.Source { return loop.SourceCrash }

// Fetch implements Fetcher.
func (c Crashes) Fetch(ctx context.Context, since time.Time) ([]Raw, error) {
	spikes, err := c.Src.Spikes(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]Raw, 0, len(spikes))
	for _, s := range spikes {
		out = append(out, CrashRaw(s))
	}
	return out, nil
}

// CrashRaw is the inbox form of one spike: a fixed template, nothing a user
// typed. One item per issue, version and day.
func CrashRaw(s CrashSpike) Raw {
	day := s.At.UTC().Format("2006-01-02")
	return Raw{
		Source: loop.SourceCrash, SourceID: s.IssueID + "@" + s.AppVersion + "@" + day,
		Text: fmt.Sprintf("クラッシュの急増: 種類 %s / %d 件 / 版 %s", truncate(s.Type, 80), s.Count, s.AppVersion),
		Kind: "crash", Count: s.Count, AppVersion: s.AppVersion, CreatedAt: s.At,
	}
}

// LoggingLister is the slice of the Cloud Logging API GenLogLogging uses
// (entries.list), small enough to fake in tests (#2146).
type LoggingLister interface {
	ListEntries(ctx context.Context, req LoggingListRequest) (LoggingPage, error)
}

// LoggingListRequest is one entries.list call.
type LoggingListRequest struct {
	Project   string
	Filter    string
	PageSize  int
	PageToken string
}

// LoggingPage is one page of entries, each in the `gcloud logging read
// --format=json` entry shape ({"timestamp": ..., "jsonPayload": {...}}).
type LoggingPage struct {
	Entries       []json.RawMessage
	NextPageToken string
}

// Paging defaults of GenLogLogging.
const (
	DefaultGenFailPageSize   = 1000
	DefaultGenFailMaxEntries = 50000
)

// ErrGenFailTooMany is returned when the range has more lines than
// MaxEntries: a cut-off read would drop the newest lines and skew the rates.
var ErrGenFailTooMany = errors.New("generation-failure logs: too many ai.generate lines")

// GenLogLogging is a GenLogSource reading the ai.generate lines straight from
// the Cloud Logging API (LOOP_GENFAIL_SOURCE=logging). It asks only for the
// GenFailFields, page by page, up to MaxEntries lines.
type GenLogLogging struct {
	Client     LoggingLister
	Project    string
	Services   []string
	PageSize   int // default DefaultGenFailPageSize (the API caps it at 1000)
	MaxEntries int // default DefaultGenFailMaxEntries
}

// GenerateEvents implements GenLogSource.
func (g GenLogLogging) GenerateEvents(ctx context.Context, since, until time.Time) ([]GenEvent, error) {
	filter, err := GenFailFilter{Since: since, Until: until, Services: g.Services}.Build()
	if err != nil {
		return nil, err
	}
	size, limit := g.PageSize, g.MaxEntries
	if size <= 0 || size > DefaultGenFailPageSize {
		size = DefaultGenFailPageSize
	}
	if limit <= 0 {
		limit = DefaultGenFailMaxEntries
	}
	out := []GenEvent{}
	read, token := 0, ""
	for {
		page, err := g.Client.ListEntries(ctx, LoggingListRequest{Project: g.Project, Filter: filter, PageSize: size, PageToken: token})
		if err != nil {
			return nil, fmt.Errorf("logging entries.list: %w", err)
		}
		if read += len(page.Entries); read > limit {
			return nil, fmt.Errorf("%w: more than %d (narrow %s or raise %s)", ErrGenFailTooMany, limit, EnvGenFailServices, EnvGenFailMaxEntries)
		}
		for _, raw := range page.Entries {
			if ev, ok := decodeGenEntry(raw); ok {
				out = append(out, ev)
			}
		}
		if page.NextPageToken == "" {
			break
		}
		if page.NextPageToken == token {
			return nil, errors.New("logging entries.list: the page token did not move")
		}
		token = page.NextPageToken
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return genInRange(out, since, until), nil
}

// loggingAPI is the LoggingLister over google.golang.org/api/logging/v2 with
// the application default credentials (roles/logging.viewer on the project).
// The service is made on the first call.
type loggingAPI struct {
	once sync.Once
	svc  *logging.Service
	err  error
}

// loggingEntryFields asks for the timestamp and the decoder's fields only, so
// uid_hash, app_hash and anything else never leave Cloud Logging.
func loggingEntryFields() googleapi.Field {
	fs := make([]string, len(GenFailFields))
	for i, f := range GenFailFields {
		fs[i] = "jsonPayload/" + f
	}
	return googleapi.Field("nextPageToken,entries(timestamp," + strings.Join(fs, ",") + ")")
}

// ListEntries implements LoggingLister.
func (l *loggingAPI) ListEntries(ctx context.Context, req LoggingListRequest) (LoggingPage, error) {
	l.once.Do(func() { l.svc, l.err = logging.NewService(ctx) })
	if l.err != nil {
		return LoggingPage{}, l.err
	}
	resp, err := l.svc.Entries.List(&logging.ListLogEntriesRequest{
		ResourceNames: []string{"projects/" + req.Project}, Filter: req.Filter, OrderBy: "timestamp asc",
		PageSize: int64(req.PageSize), PageToken: req.PageToken,
	}).Fields(loggingEntryFields()).Context(ctx).Do()
	if err != nil {
		return LoggingPage{}, err
	}
	page := LoggingPage{NextPageToken: resp.NextPageToken}
	for _, e := range resp.Entries {
		raw, err := json.Marshal(struct {
			Timestamp   string          `json:"timestamp"`
			JSONPayload json.RawMessage `json:"jsonPayload,omitempty"`
		}{e.Timestamp, json.RawMessage(e.JsonPayload)})
		if err != nil {
			return LoggingPage{}, err
		}
		page.Entries = append(page.Entries, raw)
	}
	return page, nil
}
