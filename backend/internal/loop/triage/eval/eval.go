// Package eval measures triage against a hand-labelled set (#376: 人の判断と
// 8 割以上一致). It runs the real Triager and draft.Builder on an in-memory
// store, so nothing is written to Firestore, and compares the category, the
// priority a draft would carry, and the bundling with the labels.
package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/draft"
	"github.com/jphacks/kz_2601/backend/internal/loop/pii"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
)

// Case is one labelled feedback. Bundle names the expected bundle; cases
// with the same Bundle are the same problem, "" is a bundle of its own.
// Priority is what a human would give the bundle (P1–P3).
type Case struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Category string `json:"category"`
	Priority string `json:"priority"`
	Bundle   string `json:"bundle,omitempty"`
}

// Load reads JSON lines; blank lines and lines starting with "#" are skipped.
func Load(r io.Reader) ([]Case, error) {
	var out []Case
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		var c Case
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		switch {
		case c.ID == "" || strings.TrimSpace(c.Text) == "":
			return nil, fmt.Errorf("line %d: id and text are required", n)
		case seen[c.ID]:
			return nil, fmt.Errorf("line %d: duplicate id %q", n, c.ID)
		case c.Category == "" || c.Priority == "":
			return nil, fmt.Errorf("line %d (id %s): not labelled yet: fill category and priority", n, c.ID)
		case !slices.Contains(triage.Categories, c.Category):
			return nil, fmt.Errorf("line %d: unknown category %q", n, c.Category)
		case !slices.Contains(priorities, c.Priority):
			return nil, fmt.Errorf("line %d: unknown priority %q", n, c.Priority)
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	return out, sc.Err()
}

var priorities = []string{"P1", "P2", "P3"}

// Score is matched out of total.
type Score struct{ Matched, Total int }

// Rate is Matched / Total (0 when Total is 0).
func (s Score) Rate() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Matched) / float64(s.Total)
}

func (s Score) String() string {
	return fmt.Sprintf("%d/%d (%.0f%%)", s.Matched, s.Total, 100*s.Rate())
}

// Miss is one case where the model and the label differ.
type Miss struct {
	ID, Field, Want, Got string
}

// Report is the result of one evaluation.
type Report struct {
	Cases    int
	Category Score
	Priority Score
	// Together counts pairs labelled as the same bundle that got the same
	// cluster; Apart counts pairs labelled as different bundles that got
	// different clusters.
	Together Score
	Apart    Score
	// CategoryTable and PriorityTable count (person's label, model's
	// answer) per case; an unclassified case is "(held: …)" or "(none)".
	CategoryTable Confusion
	PriorityTable Confusion
	Misses        []Miss
	Usage         triage.Usage
	// Drafts are the drafts the run built (one per bundle the model made).
	Drafts []loop.Draft
}

// MaxCases bounds one set: with the triage batch of 20 a run makes at most
// 10 model calls.
const MaxCases = 200

// Errors of Run.
var (
	// ErrTooManyCases: the set has more than MaxCases cases (nothing is called).
	ErrTooManyCases = errors.New("triage eval: too many cases")
	// ErrStopped: Hooks.BeforeCall refused a call; the report is partial.
	ErrStopped = errors.New("triage eval: stopped")
)

// Hooks are the per-call checks the real triage run has (the loop's stop
// switch, the monthly cap, the usage record). The zero Hooks checks nothing
// (an offline model).
type Hooks struct {
	BeforeCall func(ctx context.Context) error
	AfterCall  func(ctx context.Context, u triage.Usage, callErr error) error
}

// Run triages cases with m and compares the result with the labels. A case
// the model did not classify (held, rejected) counts as a miss. h is
// called around every model call, as in the triage run. Each text is masked
// (pii.Mask) before the model sees it, as ingest does, so a hand-written set
// cannot send personal data either.
func Run(ctx context.Context, m triage.Model, cases []Case, h Hooks) (Report, error) {
	rep := Report{Cases: len(cases), CategoryTable: newConfusion(triage.Categories), PriorityTable: newConfusion(priorities)}
	if len(cases) > MaxCases {
		return rep, fmt.Errorf("%w: %d (at most %d)", ErrTooManyCases, len(cases), MaxCases)
	}
	s := loop.NewMemory()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := make([]string, len(cases))
	for i, c := range cases {
		ids[i] = loop.ItemID(loop.SourceInApp, c.ID)
		it := loop.Item{ID: ids[i], Source: loop.SourceInApp, SourceID: c.ID, Text: pii.Mask(c.Text).Text, AIAllowed: true,
			Status: loop.ItemNew, IngestedAt: now.Add(time.Duration(i) * time.Second), ExpiresAt: now.Add(loop.Retention)}
		if _, err := s.PutItem(ctx, it); err != nil {
			return rep, err
		}
	}
	clock := func() time.Time { return now.Add(time.Hour) }
	tr := &triage.Triager{Store: s, Model: m, MaxItems: max(len(cases), 1), Now: clock,
		BeforeCall: h.BeforeCall, AfterCall: h.AfterCall}
	res, err := tr.Run(ctx)
	rep.Usage = res.Usage
	if err != nil {
		return rep, err
	}
	if res.Stopped != "" {
		return rep, fmt.Errorf("%w: %s", ErrStopped, res.Stopped)
	}
	// the priority a human sees is the draft's: computed over the bundle
	if _, err := (&draft.Builder{Store: s, Now: clock, MaxNew: len(cases) + 1}).Run(ctx); err != nil {
		return rep, err
	}
	drafts, err := s.ListDrafts(ctx, loop.DraftQuery{Limit: len(cases) + 1})
	if err != nil {
		return rep, err
	}
	rep.Drafts = drafts
	draftPriority := map[string]string{}
	for _, d := range drafts {
		draftPriority[d.ID] = d.Priority
	}

	got := make([]loop.Item, len(cases))
	for i, c := range cases {
		it, err := s.GetItem(ctx, ids[i])
		if err != nil {
			return rep, err
		}
		got[i] = *it
		p := it.Priority
		if dp, ok := draftPriority[it.DraftID]; ok {
			p = dp
		}
		rep.Category.Total++
		rep.Priority.Total++
		rep.CategoryTable.Add(c.Category, orNone(it.Category, it))
		rep.PriorityTable.Add(c.Priority, orNone(p, it))
		if it.Category == c.Category {
			rep.Category.Matched++
		} else {
			rep.Misses = append(rep.Misses, Miss{c.ID, "category", c.Category, orNone(it.Category, it)})
		}
		if p == c.Priority {
			rep.Priority.Matched++
		} else {
			rep.Misses = append(rep.Misses, Miss{c.ID, "priority", c.Priority, orNone(p, it)})
		}
	}
	for i := range cases {
		for j := i + 1; j < len(cases); j++ {
			wantSame := cases[i].Bundle != "" && cases[i].Bundle == cases[j].Bundle
			gotSame := got[i].ClusterKey != "" && got[i].ClusterKey == got[j].ClusterKey
			sc := &rep.Apart
			if wantSame {
				sc = &rep.Together
			}
			sc.Total++
			if wantSame == gotSame {
				sc.Matched++
				continue
			}
			field := "bundle-apart"
			if wantSame {
				field = "bundle-together"
			}
			rep.Misses = append(rep.Misses, Miss{cases[i].ID + "+" + cases[j].ID, field,
				cases[i].Bundle + "/" + cases[j].Bundle, got[i].ClusterKey + "/" + got[j].ClusterKey})
		}
	}
	sort.SliceStable(rep.Misses, func(i, j int) bool { return rep.Misses[i].Field < rep.Misses[j].Field })
	return rep, nil
}

func orNone(v string, it *loop.Item) string {
	if v != "" {
		return v
	}
	if it.HoldReason != "" {
		return "(held: " + it.HoldReason + ")"
	}
	return "(none)"
}

// String is the printed report.
func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "triage-eval: %d cases\n", r.Cases)
	fmt.Fprintf(&b, "  category: %s\n", r.Category)
	fmt.Fprintf(&b, "  priority: %s\n", r.Priority)
	fmt.Fprintf(&b, "  bundle:   together %s, apart %s (drafts %d)\n", r.Together, r.Apart, len(r.Drafts))
	b.WriteString(r.verdict(Bar))
	b.WriteString("\n  category (rows: person, columns: model)\n")
	r.CategoryTable.write(&b)
	b.WriteString("\n  priority (rows: person, columns: model)\n")
	r.PriorityTable.write(&b)
	b.WriteString("\n  bundle pairs (rows: person, columns: model)\n")
	bundleTable(r.Together, r.Apart).write(&b)
	b.WriteString("\n")
	for _, m := range r.Misses {
		fmt.Fprintf(&b, "  miss %-16s %s: want %s, got %s\n", m.Field, m.ID, m.Want, m.Got)
	}
	return b.String()
}
