// Package ingest copies feedback from every source into the loop's inbox
// (#368): one common shape, masked on the way in (#367), so no later step
// ever reads unmasked text.
package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/pii"
)

// MaxTextRunes bounds the stored text (in-app feedback is at most 2000).
const MaxTextRunes = 2000

// DefaultMaxMaskRatio holds back text masked beyond 3 割 (#367 decision 7).
const DefaultMaxMaskRatio = 0.3

// Raw is one piece of feedback as a source returns it, before masking.
// Text is untrusted and possibly personal: it only flows into pii.Mask.
type Raw struct {
	Source   loop.Source
	SourceID string
	UID      string // in-app only
	Text     string
	Kind     string
	// ConsentVersion is the in-app consent the sender agreed to ("" for
	// other sources).
	ConsentVersion string
	Rating         int
	Count          int
	HasScreenshot  bool
	HasDSL         bool
	AppVersion     string
	OSMajor        string
	CreatedAt      time.Time
	// SourceRetention bounds the copy's expiry by the source's (feedback: 180
	// days). 0 = only loop.Retention.
	SourceRetention time.Duration
}

// Fetcher reads one source.
type Fetcher interface {
	Source() loop.Source
	// Fetch returns what was created at or after since (it may return a
	// little more; ingest is idempotent).
	Fetch(ctx context.Context, since time.Time) ([]Raw, error)
}

// Policy decides which items may reach the triage LLM.
type Policy struct {
	// ConsentVersions are the in-app consent versions whose text says an AI
	// reads it (#367 decision 2). Empty = no in-app feedback goes to the AI.
	ConsentVersions map[string]bool
	// AISources are the sources whose items may go to the AI at all.
	AISources map[loop.Source]bool
	// MaxMaskRatio: more masked than this = held (0 = DefaultMaxMaskRatio).
	MaxMaskRatio float64
}

// Users tells whether an account is deleted (store.Accounts.UserDeleted).
type Users interface {
	UserDeleted(ctx context.Context, uid string) (bool, error)
}

// Stats counts one run.
type Stats struct {
	Fetched int
	Created int
	// Held: stored but kept from the AI.
	Held int
	// Skipped: not stored (sender's account deleted, or already ingested).
	Skipped int
}

// Runner ingests sources into a Store.
type Runner struct {
	Store  loop.Store
	Policy Policy
	Users  Users // nil = no deletion check (sources without a uid)
	Now    func() time.Time
	// OnItem, when set, is called for each stored item (audit, #375).
	OnItem func(ctx context.Context, it loop.Item)
	// Overlap re-reads this much before the cursor (late writes). Default 1h.
	Overlap time.Duration
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

// Run fetches f from its cursor, stores new items and advances the cursor.
func (r *Runner) Run(ctx context.Context, f Fetcher) (Stats, error) {
	var st Stats
	src := f.Source()
	cursor, err := r.Store.Cursor(ctx, src)
	if err != nil {
		return st, fmt.Errorf("cursor %s: %w", src, err)
	}
	now := r.now()
	since := cursor
	if !since.IsZero() {
		overlap := r.Overlap
		if overlap <= 0 {
			overlap = time.Hour
		}
		since = since.Add(-overlap)
	} else {
		// first run: never further back than the loop keeps things
		since = now.Add(-loop.Retention)
	}
	raws, err := f.Fetch(ctx, since)
	if err != nil {
		return st, fmt.Errorf("fetch %s: %w", src, err)
	}
	st.Fetched = len(raws)
	high := cursor
	for _, raw := range raws {
		if raw.CreatedAt.After(high) {
			high = raw.CreatedAt
		}
		if raw.UID != "" && r.Users != nil {
			deleted, err := r.Users.UserDeleted(ctx, raw.UID)
			if err != nil {
				return st, fmt.Errorf("user check: %w", err)
			}
			if deleted {
				st.Skipped++
				continue
			}
		}
		it := Normalize(raw, r.Policy, now)
		created, err := r.Store.PutItem(ctx, it)
		if err != nil {
			return st, fmt.Errorf("put item: %w", err)
		}
		if !created {
			st.Skipped++
			continue
		}
		st.Created++
		if it.Status == loop.ItemHeld {
			st.Held++
		}
		if r.OnItem != nil {
			r.OnItem(ctx, it)
		}
	}
	if high.After(cursor) {
		if err := r.Store.SetCursor(ctx, src, high); err != nil {
			return st, fmt.Errorf("set cursor: %w", err)
		}
	}
	return st, nil
}

// Normalize masks raw into an inbox item and decides whether the AI may read it.
func Normalize(raw Raw, p Policy, now time.Time) loop.Item {
	m := pii.Mask(truncate(raw.Text, MaxTextRunes))
	text := strings.TrimSpace(m.Text)
	it := loop.Item{
		ID:            loop.ItemID(raw.Source, raw.SourceID),
		Source:        raw.Source,
		SourceID:      raw.SourceID,
		UID:           raw.UID,
		Text:          text,
		Kind:          truncate(pii.Clean(raw.Kind), 40),
		Rating:        raw.Rating,
		Count:         raw.Count,
		HasScreenshot: raw.HasScreenshot,
		HasDSL:        raw.HasDSL,
		AppVersion:    truncate(pii.Clean(raw.AppVersion), 32),
		OSMajor:       truncate(pii.Clean(raw.OSMajor), 32),
		Status:        loop.ItemNew,
		AIAllowed:     true,
		CreatedAt:     raw.CreatedAt.UTC(),
		IngestedAt:    now,
		ExpiresAt:     loop.ExpiresFrom(now, raw.CreatedAt, raw.SourceRetention),
	}
	if len(m.Counts) > 0 {
		it.MaskCounts = map[string]int{}
		for k, v := range m.Counts {
			it.MaskCounts[string(k)] = v
		}
	}
	maxRatio := p.MaxMaskRatio
	if maxRatio <= 0 {
		maxRatio = DefaultMaxMaskRatio
	}
	hold := ""
	switch {
	case text == "":
		hold = loop.HoldEmpty
	case !p.AISources[raw.Source]:
		hold = loop.HoldSource
	case raw.Source == loop.SourceInApp && !p.ConsentVersions[raw.ConsentVersion]:
		hold = loop.HoldConsent
	case m.Ratio > maxRatio:
		hold = loop.HoldMasked
	}
	if hold != "" {
		it.AIAllowed, it.HoldReason, it.Status = false, hold, loop.ItemHeld
	}
	return it
}

// OSMajor turns "iOS 26.1" / "26.1.2" into "iOS 26" / "26": the minor
// version adds little and helps identify a tester (#367).
func OSMajor(os string) string {
	os = strings.TrimSpace(os)
	i := strings.LastIndexByte(os, ' ')
	name, ver := "", os
	if i >= 0 {
		name, ver = os[:i+1], os[i+1:]
	}
	if j := strings.IndexByte(ver, '.'); j >= 0 {
		ver = ver[:j]
	}
	return name + ver
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// ParseList splits a comma separated env value into a set.
func ParseList(v string) map[string]bool {
	out := map[string]bool{}
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out[s] = true
		}
	}
	return out
}

// PolicyFromEnv reads LOOP_CONSENT_VERSIONS (default empty: no in-app
// feedback reaches the AI until the consent text says an AI reads it) and
// LOOP_AI_SOURCES (default "in_app,crash"; store reviews only when listed).
func PolicyFromEnv(getenv func(string) string) Policy {
	srcs := getenv("LOOP_AI_SOURCES")
	if srcs == "" {
		srcs = string(loop.SourceInApp) + "," + string(loop.SourceCrash)
	}
	p := Policy{ConsentVersions: ParseList(getenv("LOOP_CONSENT_VERSIONS")), AISources: map[loop.Source]bool{}}
	for s := range ParseList(srcs) {
		p.AISources[loop.Source(s)] = true
	}
	return p
}
