// Package draft turns triaged clusters into issue drafts that wait in a
// private review queue (loop_drafts) for a human (#369). Nothing here posts
// to GitHub; Approve does, only when a human runs it and a target repo is
// configured. Drafts are built from the checked summaries with a fixed
// template (no second LLM call), and pass the exit check (Check, #366 検査器 2)
// when stored and again right before they are posted.
//
// A cluster that already has a published issue that is not fixed yet gets an
// append draft instead: a comment with the new counts and summaries for that
// issue (Draft.AppendIssue), approved and checked the same way.
package draft

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
)

// Draftable categories (spam and praise never become issues).
var draftable = map[string]string{"bug": "不具合", "feature": "要望", "question": "使い方の質問"}

// MaxExamples is how many other summaries a draft lists.
const MaxExamples = 3

// Builder groups triaged items into drafts.
type Builder struct {
	Store loop.Store
	Now   func() time.Time
	// MaxNew bounds new drafts per run (#365: 1 週の下書きの数に上限). Default 20.
	MaxNew int
	// OnDraft, when set, is called for each draft created or extended. An
	// error stops the run.
	OnDraft func(ctx context.Context, d loop.Draft, created bool) error
	// OnItem, when set, is called for each item whose status changed. An
	// error stops the run.
	OnItem func(ctx context.Context, it loop.Item) error
}

// Result counts one run.
type Result struct {
	Created  int
	Extended int
	Items    int
	Held     int // clusters whose draft failed the exit check
	Capped   int // clusters left for the next run (MaxNew)
}

func (b *Builder) now() time.Time {
	if b.Now != nil {
		return b.Now().UTC()
	}
	return time.Now().UTC()
}

// Run drafts every triaged cluster.
func (b *Builder) Run(ctx context.Context) (Result, error) {
	var res Result
	items, err := b.Store.ListItems(ctx, loop.ItemQuery{Status: loop.ItemTriaged, Limit: 500})
	if err != nil {
		return res, err
	}
	clusters := map[string][]loop.Item{}
	keys := []string{}
	for _, it := range items {
		if _, ok := draftable[it.Category]; !ok || it.ClusterKey == "" {
			continue
		}
		if _, ok := clusters[it.ClusterKey]; !ok {
			keys = append(keys, it.ClusterKey)
		}
		clusters[it.ClusterKey] = append(clusters[it.ClusterKey], it)
	}
	sort.Strings(keys)
	pending, err := b.Store.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPending, Limit: 500})
	if err != nil {
		return res, err
	}
	open := map[string]loop.Draft{}
	for _, d := range pending {
		open[d.ClusterKey] = d
	}
	filed, err := FiledIssues(ctx, b.Store)
	if err != nil {
		return res, err
	}
	maxNew := b.MaxNew
	if maxNew <= 0 {
		maxNew = 20
	}
	now := b.now()
	for _, key := range keys {
		newItems := clusters[key]
		d, exists := open[key]
		if !exists && res.Created >= maxNew {
			res.Capped++
			continue
		}
		all := newItems
		if exists {
			old, err := b.load(ctx, d.ItemIDs)
			if err != nil {
				return res, err
			}
			all = append(old, newItems...)
		} else {
			d = loop.Draft{ID: newID(), ClusterKey: key, Status: loop.DraftPending, CreatedAt: now, ExpiresAt: now.Add(loop.Retention)}
		}
		t := filed[key]
		d.AppendRepo, d.AppendIssue = t.Repo, t.Number
		fill(&d, all, now)
		if reasons := Check(d.Title, d.Body, texts(all)); len(reasons) > 0 {
			// do not store a draft that would fail at approval; the items
			// go to the weekly manual triage
			res.Held++
			for _, it := range newItems {
				it.Status, it.HoldReason = loop.ItemHeld, loop.HoldRejected
				if err := b.updateItem(ctx, it); err != nil {
					return res, err
				}
			}
			continue
		}
		if err := b.Store.PutDraft(ctx, d); err != nil {
			return res, err
		}
		if exists {
			res.Extended++
		} else {
			res.Created++
		}
		if b.OnDraft != nil {
			if err := b.OnDraft(ctx, d, !exists); err != nil {
				return res, err
			}
		}
		for _, it := range newItems {
			it.Status, it.DraftID = loop.ItemDrafted, d.ID
			if err := b.updateItem(ctx, it); err != nil {
				return res, err
			}
			res.Items++
		}
	}
	return res, nil
}

func (b *Builder) updateItem(ctx context.Context, it loop.Item) error {
	if err := b.Store.UpdateItem(ctx, it); err != nil {
		return err
	}
	if b.OnItem != nil {
		return b.OnItem(ctx, it)
	}
	return nil
}

func (b *Builder) load(ctx context.Context, ids []string) ([]loop.Item, error) {
	return LoadItems(ctx, b.Store, ids)
}

// Filed is an issue the loop already posted for a cluster.
type Filed struct {
	Repo   string
	Number int
}

// FiledIssues maps each cluster key to the newest published issue of that
// cluster that is not fixed yet. A fixed cluster that comes back gets a new
// issue (it is a regression, not more of the same).
func FiledIssues(ctx context.Context, s loop.Store) (map[string]Filed, error) {
	ds, err := s.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPublished, Limit: 500})
	if err != nil {
		return nil, err
	}
	out := map[string]Filed{}
	fixed := map[Filed]bool{}
	for _, d := range ds {
		if d.IssueNumber > 0 && !d.FixedAt.IsZero() {
			fixed[Filed{d.IssueRepo, d.IssueNumber}] = true
		}
	}
	for _, d := range ds { // newest first
		f := Filed{d.IssueRepo, d.IssueNumber}
		if _, seen := out[d.ClusterKey]; seen || d.ClusterKey == "" || f.Number <= 0 || fixed[f] {
			continue
		}
		out[d.ClusterKey] = f
	}
	return out, nil
}

// LoadItems reads ids, skipping ones that expired or were erased.
func LoadItems(ctx context.Context, s loop.Store, ids []string) ([]loop.Item, error) {
	out := []loop.Item{}
	for _, id := range ids {
		it, err := s.GetItem(ctx, id)
		if err == loop.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, nil
}

func texts(items []loop.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Text
	}
	return out
}

func newID() string {
	var b [10]byte
	_, _ = rand.Read(b[:])
	return "drf_" + hex.EncodeToString(b[:])
}

// fill computes the draft's fields from all of its items.
func fill(d *loop.Draft, items []loop.Item, now time.Time) {
	sort.SliceStable(items, func(i, j int) bool { return rank(items[i]) > rank(items[j]) })
	rep := items[0]
	d.Category, d.Area = rep.Category, rep.Area
	count, bySource := 0, map[loop.Source]int{}
	versions, oses := []string{}, []string{}
	d.ItemIDs = d.ItemIDs[:0]
	for _, it := range items {
		n := max(it.Count, 1)
		count += n
		bySource[it.Source] += n
		d.ItemIDs = append(d.ItemIDs, it.ID)
		if it.AppVersion != "" && !slices.Contains(versions, it.AppVersion) {
			versions = append(versions, it.AppVersion)
		}
		if it.OSMajor != "" && !slices.Contains(oses, it.OSMajor) {
			oses = append(oses, it.OSMajor)
		}
	}
	sort.Strings(versions)
	sort.Strings(oses)
	d.Count, d.AppVersions = count, versions
	d.Sources = d.Sources[:0]
	for _, s := range []loop.Source{loop.SourceInApp, loop.SourceStoreReview, loop.SourceTestFlight, loop.SourceCrash} {
		if bySource[s] > 0 {
			d.Sources = append(d.Sources, string(s))
		}
	}
	d.Priority = triage.Priority(rep.Category, impactOf(rep.Priority), rep.Area, count)
	d.Title = fmt.Sprintf("[フィードバック] %s: %s", draftable[rep.Category], cut(firstSentence(rep.Summary), 60))
	examples := []string{}
	for _, it := range items[1:] {
		if len(examples) == MaxExamples {
			break
		}
		if it.Summary != "" && it.Summary != rep.Summary && !slices.Contains(examples, it.Summary) {
			examples = append(examples, it.Summary)
		}
	}
	counts := func(b *strings.Builder) {
		fmt.Fprintf(b, "- 件数: %d(%s)\n", count, sourceLine(bySource))
		if len(versions) > 0 {
			b.WriteString("- アプリの版: " + strings.Join(versions, "、") + "\n")
		}
		if len(oses) > 0 {
			b.WriteString("- OS: " + strings.Join(oses, "、") + "\n")
		}
		b.WriteString("- 領域: " + rep.Area + "\n- 優先度の案: " + d.Priority + "\n")
	}

	var b strings.Builder
	if d.AppendIssue > 0 {
		// a comment on the filed issue: only what is new
		b.WriteString("## 同じ束の新しい報告\n\n")
		counts(&b)
		b.WriteString("\n## 新しい報告(要約)\n\n")
		for _, e := range append([]string{rep.Summary}, examples...) {
			b.WriteString("- " + strings.ReplaceAll(e, "\n", " ") + "\n")
		}
		b.WriteString("\n---\nこの Issue を出した後に届いたフィードバックの要約を、人が確かめて追記した。原文と個人情報は載せていない(#367)。\n")
		d.Body = b.String()
		d.UpdatedAt = now
		return
	}
	b.WriteString("## 症状\n\n" + rep.Summary + "\n")
	if len(examples) > 0 {
		b.WriteString("\n## ほかの報告(要約)\n\n")
		for _, e := range examples {
			b.WriteString("- " + strings.ReplaceAll(e, "\n", " ") + "\n")
		}
	}
	b.WriteString("\n## 件数と入口\n\n")
	counts(&b)
	b.WriteString("\n## 再現の手順\n\n(承認する人が書く: `feedback-loop drafts edit`)\n")
	b.WriteString("\n## 完了の条件\n\n- 上の症状が、再現の手順で起きない\n- 同じ束のフィードバックが、直した版から 30 日間来ない\n")
	b.WriteString("\n---\nフィードバックの要約から作った下書きを、人が確かめて出した Issue。原文と個人情報は載せていない(#367)。\n")
	d.Body = b.String()
	d.UpdatedAt = now
}

func rank(it loop.Item) int {
	p := map[string]int{"P1": 3, "P2": 2, "P3": 1}[it.Priority]
	return p*1000 + max(it.Count, 1)
}

// impactOf maps an item's computed priority back to an impact for the
// cluster's priority (the model's impact itself is not stored).
func impactOf(p string) string {
	switch p {
	case "P1":
		return "high"
	case "P2":
		return "medium"
	}
	return "low"
}

func sourceLine(by map[loop.Source]int) string {
	names := []struct {
		s    loop.Source
		name string
	}{{loop.SourceInApp, "アプリ内"}, {loop.SourceStoreReview, "ストアのレビュー"}, {loop.SourceTestFlight, "TestFlight"}, {loop.SourceCrash, "クラッシュ"}}
	parts := []string{}
	for _, n := range names {
		if by[n.s] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", n.name, by[n.s]))
		}
	}
	return strings.Join(parts, " / ")
}

func firstSentence(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if i := strings.IndexAny(s, "。."); i > 0 {
		return s[:i]
	}
	return s
}

func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// BodyHash is the sha256 of what is posted (#366 decision 11: an agent run
// can compare the issue body with what the human approved).
func BodyHash(title, body string) string {
	sum := sha256.Sum256([]byte(title + "\x00" + body))
	return hex.EncodeToString(sum[:])
}
