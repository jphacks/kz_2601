// Package triage classifies, clusters and prioritises inbox items with an
// LLM (#376) under the constraints of #366: the model gets no tools and no
// secrets, the feedback is data inside per-call delimiters, and only output
// that passes the schema and 検査器 1 (check.go) is used. Everything else is
// held for the weekly manual triage.
package triage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/pii"
)

// Usage is the token accounting of one call.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Total is input + output.
func (u Usage) Total() int { return u.InputTokens + u.OutputTokens }

// Model is one LLM call: a system prompt and one user message in, text out.
// It has no way to pass tools; implementations must not add any.
type Model interface {
	Complete(ctx context.Context, system, user string, schema json.RawMessage) (string, Usage, error)
}

// Defaults.
const (
	DefaultBatch    = 20
	DefaultMaxItems = 200
)

// Triager runs one triage pass.
type Triager struct {
	Store loop.Store
	Model Model
	// Batch is how many distinct texts go into one call (<= 50).
	Batch int
	// MaxItems bounds one run (cost cap, #365 選択肢 5).
	MaxItems int
	Now      func() time.Time
	// BeforeCall, when set, may refuse a call (stop switch, monthly budget:
	// #375). A non-nil error stops the run before the call.
	BeforeCall func(ctx context.Context) error
	// AfterCall, when set, receives each call's usage (budget, audit). An
	// error stops the run.
	AfterCall func(ctx context.Context, u Usage, callErr error) error
	// OnItem, when set, is called for each item whose status changed. An
	// error stops the run.
	OnItem func(ctx context.Context, it loop.Item) error
	// nonce is set by tests; "" = random per call.
	nonce string
}

// Result counts one run.
type Result struct {
	Calls      int
	Items      int
	Triaged    int
	Duplicates int // items that copied their group's verdict
	Spam       int
	Held       int
	Rejected   int // items whose verdict was rejected or missing
	Usage      Usage
	// Stopped is why the run ended early ("" = it did not).
	Stopped string
}

func (t *Triager) now() time.Time {
	if t.Now != nil {
		return t.Now().UTC()
	}
	return time.Now().UTC()
}

// group is one distinct text: the items that normalise to it.
type group struct {
	key   string
	items []loop.Item
}

// Run triages up to MaxItems new items.
func (t *Triager) Run(ctx context.Context) (Result, error) {
	var res Result
	maxItems := t.MaxItems
	if maxItems <= 0 {
		maxItems = DefaultMaxItems
	}
	batch := t.Batch
	if batch <= 0 || batch > 50 {
		batch = DefaultBatch
	}
	items, err := t.Store.ListItems(ctx, loop.ItemQuery{Status: loop.ItemNew, Limit: maxItems})
	if err != nil {
		return res, err
	}
	groups := groupItems(items)
	res.Items = len(items)
	clusters, err := t.knownClusters(ctx)
	if err != nil {
		return res, err
	}
	for start := 0; start < len(groups); start += batch {
		if t.BeforeCall != nil {
			if err := t.BeforeCall(ctx); err != nil {
				res.Stopped = err.Error()
				return res, nil
			}
		}
		chunk := groups[start:min(start+batch, len(groups))]
		if err := t.call(ctx, chunk, clusters, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// knownClusters are the cluster keys of drafts still waiting for a human, so
// new feedback can join them. Keys are internal slugs, not shown publicly.
func (t *Triager) knownClusters(ctx context.Context) ([]string, error) {
	ds, err := t.Store.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPending, Limit: 200})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, d := range ds {
		if d.ClusterKey != "" && !seen[d.ClusterKey] {
			seen[d.ClusterKey] = true
			out = append(out, d.ClusterKey)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (t *Triager) call(ctx context.Context, chunk []group, clusters []string, res *Result) error {
	nonce := t.nonce
	if nonce == "" {
		var b [6]byte
		_, _ = rand.Read(b[:])
		nonce = hex.EncodeToString(b[:])
	}
	sent := map[string]bool{}
	byLocal := map[string]group{}
	for i, g := range chunk {
		id := "i" + strconv.Itoa(i+1)
		sent[id] = true
		byLocal[id] = g
	}
	user := BuildUser(nonce, chunk, clusters)
	text, usage, err := t.Model.Complete(ctx, System, user, SchemaJSON())
	res.Calls++
	res.Usage.InputTokens += usage.InputTokens
	res.Usage.OutputTokens += usage.OutputTokens
	if t.AfterCall != nil {
		if aerr := t.AfterCall(ctx, usage, err); aerr != nil {
			return aerr
		}
	}
	if err != nil {
		// the call failed: leave the items new for the next run
		return fmt.Errorf("triage call: %w", err)
	}
	good, _, perr := Parse(text, sent)
	now := t.now()
	if perr != nil {
		// fail closed: nothing from this response is used
		for _, g := range chunk {
			if err := t.hold(ctx, g, loop.HoldRejected, res); err != nil {
				return err
			}
			res.Rejected += len(g.items)
		}
		return nil
	}
	done := map[string]bool{}
	for _, v := range good {
		done[v.ID] = true
		g := byLocal[v.ID]
		if v.PIISuspected {
			if err := t.hold(ctx, g, loop.HoldPII, res); err != nil {
				return err
			}
			continue
		}
		for i, it := range g.items {
			it.Category, it.Area, it.Impact, it.Summary, it.ClusterKey, it.TriagedAt = v.Category, v.Area, v.Impact, v.Summary, v.Cluster, now
			it.Priority = Priority(v.Category, v.Impact, v.Area, max(len(g.items), it.Count))
			switch v.Category {
			case "spam":
				it.Status = loop.ItemSpam
				res.Spam++
			case "praise":
				it.Status, it.HoldReason = loop.ItemHeld, loop.HoldPraise
				res.Held++
			default:
				it.Status = loop.ItemTriaged
				res.Triaged++
			}
			if i > 0 {
				res.Duplicates++
			}
			if err := t.update(ctx, it); err != nil {
				return err
			}
		}
	}
	// rows the model left out, or whose summary failed 検査器 1
	for id, g := range byLocal {
		if done[id] {
			continue
		}
		if err := t.hold(ctx, g, loop.HoldRejected, res); err != nil {
			return err
		}
		res.Rejected += len(g.items)
	}
	return nil
}

func (t *Triager) hold(ctx context.Context, g group, reason string, res *Result) error {
	for _, it := range g.items {
		it.Status, it.HoldReason, it.AIAllowed = loop.ItemHeld, reason, false
		res.Held++
		if err := t.update(ctx, it); err != nil {
			return err
		}
	}
	return nil
}

func (t *Triager) update(ctx context.Context, it loop.Item) error {
	if err := t.Store.UpdateItem(ctx, it); err != nil && !errors.Is(err, loop.ErrNotFound) {
		return err
	}
	if t.OnItem != nil {
		return t.OnItem(ctx, it)
	}
	return nil
}

// groupItems merges items whose normalised text is identical (the cheap,
// embedding-free first step of clustering); the LLM sees each text once.
func groupItems(items []loop.Item) []group {
	idx := map[string]int{}
	out := []group{}
	for _, it := range items {
		if !it.AIAllowed {
			continue
		}
		k := NormalizeText(it.Text)
		if i, ok := idx[k]; ok {
			out[i].items = append(out[i].items, it)
			continue
		}
		idx[k] = len(out)
		out = append(out, group{key: k, items: []loop.Item{it}})
	}
	return out
}

// NormalizeText folds width and case and drops spaces and punctuation, so
// 「落ちる!」 and 「落ちる」 are one text.
func NormalizeText(s string) string {
	s = strings.ToLower(pii.Clean(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '[' || r == ']' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Priority is computed from the verdict and the number of reports, not taken
// from the model: P1 / P2 / P3.
func Priority(category, impact, area string, count int) string {
	score := map[string]int{"bug": 3, "feature": 2, "question": 1}[category]
	if score == 0 {
		return "P3"
	}
	score += map[string]int{"high": 2, "medium": 1}[impact]
	if area == "billing" && category == "bug" {
		score++
	}
	switch {
	case count >= 5:
		score += 2
	case count >= 2:
		score++
	}
	switch {
	case score >= 6:
		return "P1"
	case score >= 4:
		return "P2"
	}
	return "P3"
}
