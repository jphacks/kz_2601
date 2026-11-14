package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

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
