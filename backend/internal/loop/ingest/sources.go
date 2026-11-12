package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	androidpublisher "google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/googleapi"

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

// Google Play reviews (#2147). Off unless LOOP_SOURCE_GOOGLEPLAY=on; not in
// the default LOOP_AI_SOURCES. The cursor is the Runner's
// loop_control/cursor-google_play_review; masking happens at Normalize like
// every other source.
const (
	EnvGooglePlayOn      = "LOOP_SOURCE_GOOGLEPLAY"
	EnvGooglePlayPackage = "LOOP_GOOGLEPLAY_PACKAGE" // the app's package name
)

// ErrGooglePlayOff is returned by GooglePlayConfigFromEnv when the source is off.
var ErrGooglePlayOff = errors.New("google play reviews are off (" + EnvGooglePlayOn + " != on)")

// GooglePlayLister is the slice of reviews.list GooglePlayReviews uses,
// small enough to fake in tests. token "" is the first page.
type GooglePlayLister interface {
	ListReviews(ctx context.Context, packageName, token string) (GooglePlayPage, error)
}

// GooglePlayReviews adapts reviews.list to Fetcher.
type GooglePlayReviews struct {
	Client  GooglePlayLister
	Package string
	// MaxPages bounds one run (100 reviews a page). Default 5.
	MaxPages int
}

// Source implements Fetcher.
func (GooglePlayReviews) Source() loop.Source { return SourceGooglePlay }

// Fetch implements Fetcher: newest first (Play's order), stopping at the
// first review last changed before since.
func (g GooglePlayReviews) Fetch(ctx context.Context, since time.Time) ([]Raw, error) {
	pages := g.MaxPages
	if pages <= 0 {
		pages = 5
	}
	out := []Raw{}
	token := ""
	for p := 0; p < pages; p++ {
		page, err := g.Client.ListReviews(ctx, g.Package, token)
		if err != nil {
			return nil, fmt.Errorf("google play: %w", err)
		}
		for _, r := range page.Reviews {
			if r.CreatedAt.Before(since) {
				return out, nil
			}
			out = append(out, r.Raw())
		}
		if page.NextPageToken == "" {
			return out, nil
		}
		if page.NextPageToken == token {
			return nil, errors.New("google play: the page token did not move")
		}
		token = page.NextPageToken
	}
	slog.Warn("loop ingest: google play stopped at the page cap; older reviews after the cursor may be skipped", "pages", pages)
	return out, nil
}

// GooglePlayConfigFromEnv reads LOOP_SOURCE_GOOGLEPLAY and the package. The
// reader uses the application default credentials (the job's service
// account, invited in the Play Console); no key is read from the env.
func GooglePlayConfigFromEnv(getenv func(string) string) (GooglePlayReviews, error) {
	if getenv(EnvGooglePlayOn) != "on" {
		return GooglePlayReviews{}, ErrGooglePlayOff
	}
	pkg := strings.TrimSpace(getenv(EnvGooglePlayPackage))
	if pkg == "" {
		return GooglePlayReviews{}, fmt.Errorf("%s=on needs %s", EnvGooglePlayOn, EnvGooglePlayPackage)
	}
	return GooglePlayReviews{Client: &playAPI{}, Package: pkg}, nil
}

// playAPI is the GooglePlayLister over the Play Developer API. The service is
// made on the first call. Error texts are the API's status, never a body.
type playAPI struct {
	once sync.Once
	svc  *androidpublisher.Service
	err  error
}

// ListReviews implements GooglePlayLister: the answer goes through the same
// parser as the testdata.
func (p *playAPI) ListReviews(ctx context.Context, pkg, token string) (GooglePlayPage, error) {
	p.once.Do(func() { p.svc, p.err = androidpublisher.NewService(ctx) })
	if p.err != nil {
		return GooglePlayPage{}, p.err
	}
	call := p.svc.Reviews.List(pkg).MaxResults(100).Context(ctx)
	if token != "" {
		call = call.Token(token)
	}
	resp, err := call.Do()
	if gerr := (*googleapi.Error)(nil); errors.As(err, &gerr) {
		return GooglePlayPage{}, fmt.Errorf("status %d", gerr.Code)
	}
	if err != nil {
		return GooglePlayPage{}, err
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return GooglePlayPage{}, err
	}
	return ParseGooglePlayReviews(bytes.NewReader(b))
}
