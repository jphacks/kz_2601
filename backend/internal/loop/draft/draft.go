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
	// Embedder, when set (LOOP_EMBED=on), lists the existing clusters near a
	// new draft's in its body (「同じかもしれない束」). It never merges them.
	Embedder triage.Embedder
	// SimilarThreshold is the cosine for a candidate (0 =
	// triage.DefaultSimilarThreshold).
	SimilarThreshold float64
}

// Result counts one run.
type Result struct {
	Created  int
	Extended int
	Items    int
	Held     int // clusters whose draft failed the exit check
	Attached int // items that predate the filed issue, added to it without a comment
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
	var reps []triage.ClusterRep
	if b.Embedder != nil {
		if reps, err = b.clusterReps(ctx, pending, filed); err != nil {
			return res, err
		}
	}
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
		if t.Number > 0 {
			// a comment carries only what arrived after the issue was
			// published; earlier reports belong to the filed issue
			fresh, earlier := splitAt(all, t.At)
			if len(earlier) > 0 {
				if err := b.attach(ctx, t, earlier, &res); err != nil {
					return res, err
				}
			}
			if len(fresh) == 0 {
				if exists {
					d.Status, d.RejectReason, d.ItemIDs, d.UpdatedAt = loop.DraftRejected, "nothing new since the issue was published", nil, now
					if err := b.Store.PutDraft(ctx, d); err != nil {
						return res, err
					}
				}
				continue
			}
			all = fresh
			newItems, _ = splitAt(newItems, t.At)
		}
		d.AppendRepo, d.AppendIssue = t.Repo, t.Number
		fill(&d, all, now)
		// recomputed on every rebuild, so an extended draft keeps its list
		if b.Embedder != nil && d.AppendIssue == 0 {
			sims, err := triage.SimilarClusters(ctx, b.Embedder, key, all[0].Summary, reps, b.threshold())
			if err != nil {
				return res, fmt.Errorf("similar clusters of %s: %w", key, err)
			}
			addSimilar(&d, sims)
		}
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
			if b.Embedder != nil && d.AppendIssue == 0 {
				reps = append(reps, triage.ClusterRep{Key: key, Count: d.Count, Summary: all[0].Summary})
			}
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

func (b *Builder) threshold() float64 {
	if b.SimilarThreshold > 0 {
		return b.SimilarThreshold
	}
	return triage.DefaultSimilarThreshold
}

// clusterReps are the clusters a new draft is compared with: the pending
// drafts and the filed, still open issues. A draft's first item is its
// representative (fill orders them).
func (b *Builder) clusterReps(ctx context.Context, pending []loop.Draft, filed map[string]Filed) ([]triage.ClusterRep, error) {
	ds := slices.Clone(pending)
	keys := make([]string, 0, len(filed))
	for k := range filed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d, err := b.Store.GetDraft(ctx, filed[k].DraftID)
		if err == loop.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		ds = append(ds, *d)
	}
	out := []triage.ClusterRep{}
	seen := map[string]bool{}
	for _, d := range ds {
		if d.ClusterKey == "" || seen[d.ClusterKey] || len(d.ItemIDs) == 0 {
			continue
		}
		items, err := LoadItems(ctx, b.Store, d.ItemIDs[:1])
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			continue
		}
		seen[d.ClusterKey] = true
		out = append(out, triage.ClusterRep{Key: d.ClusterKey, Count: d.Count, Summary: items[0].Summary})
	}
	return out, nil
}

// addSimilar writes the candidates (cluster key and count only) above the
// footer. No candidates, no section.
func addSimilar(d *loop.Draft, sims []triage.Similar) {
	if len(sims) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString("\n## 同じかもしれない束\n\n")
	for _, s := range sims {
		fmt.Fprintf(&b, "- %s(%d 件)\n", s.Key, s.Count)
	}
	b.WriteString("\n要約の近さで選んだ候補。束ねるかは承認する人が決める。\n")
	if i := strings.LastIndex(d.Body, "\n---\n"); i >= 0 {
		d.Body = d.Body[:i] + b.String() + d.Body[i:]
		return
	}
	d.Body += b.String()
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

// attach adds items that arrived before the filed issue was published to
// that issue's draft, so their senders are told when it is fixed, without
// posting them as new reports.
func (b *Builder) attach(ctx context.Context, t Filed, items []loop.Item, res *Result) error {
	fd, err := b.Store.GetDraft(ctx, t.DraftID)
	if err != nil {
		return err
	}
	for _, it := range items {
		if !slices.Contains(fd.ItemIDs, it.ID) {
			fd.ItemIDs = append(fd.ItemIDs, it.ID)
		}
	}
	if err := b.Store.PutDraft(ctx, *fd); err != nil {
		return err
	}
	for _, it := range items {
		it.Status, it.DraftID = loop.ItemDrafted, fd.ID
		if err := b.updateItem(ctx, it); err != nil {
			return err
		}
		res.Attached++
	}
	return nil
}

// splitAt splits items into those ingested at or after at and the rest.
func splitAt(items []loop.Item, at time.Time) (after, before []loop.Item) {
	for _, it := range items {
		when := it.IngestedAt
		if when.IsZero() {
			when = it.CreatedAt
		}
		if !when.Before(at) {
			after = append(after, it)
		} else {
			before = append(before, it)
		}
	}
	return after, before
}

func (b *Builder) load(ctx context.Context, ids []string) ([]loop.Item, error) {
	return LoadItems(ctx, b.Store, ids)
}

// Filed is an issue the loop already posted for a cluster.
type Filed struct {
	Repo   string
	Number int
	// DraftID is the newest published draft of the issue; At is when the
	// issue itself was published.
	DraftID string
	At      time.Time
}

type issueKey struct {
	repo string
	n    int
}

// publishedAt is when d was posted (older drafts have no PublishedAt).
func publishedAt(d loop.Draft) time.Time {
	switch {
	case !d.PublishedAt.IsZero():
		return d.PublishedAt
	case !d.ReviewedAt.IsZero():
		return d.ReviewedAt
	}
	return d.CreatedAt
}

// FiledIssues maps each cluster key to the newest published issue of that
// cluster that is still open as far as the loop knows (not fixed, not seen
// closed). A fixed or closed cluster that comes back gets a new issue (it is
// a regression or was set aside, not more of the same).
func FiledIssues(ctx context.Context, s loop.Store) (map[string]Filed, error) {
	ds, err := s.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPublished, Limit: 500})
	if err != nil {
		return nil, err
	}
	out := map[string]Filed{}
	done := map[issueKey]bool{}
	first := map[issueKey]time.Time{}
	for _, d := range ds {
		k := issueKey{d.IssueRepo, d.IssueNumber}
		if d.IssueNumber > 0 && (!d.FixedAt.IsZero() || !d.ClosedAt.IsZero()) {
			done[k] = true
		}
		if at := publishedAt(d); first[k].IsZero() || at.Before(first[k]) {
			first[k] = at
		}
	}
	for _, d := range ds { // newest first
		k := issueKey{d.IssueRepo, d.IssueNumber}
		if _, seen := out[d.ClusterKey]; seen || d.ClusterKey == "" || k.n <= 0 || done[k] {
			continue
		}
		out[d.ClusterKey] = Filed{Repo: k.repo, Number: k.n, DraftID: d.ID, At: first[k]}
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
	for _, s := range []loop.Source{loop.SourceInApp, loop.SourceStoreReview, loop.SourceTestFlight, loop.SourceCrash, loop.SourceGenFailure} {
		if bySource[s] > 0 {
			d.Sources = append(d.Sources, string(s))
		}
	}
	d.Priority = BundlePriority(rep.Category, rep.Area, items, count)
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

// ManyReports is how many reports raise a bundle's priority one more level.
const ManyReports = 10

// BundlePriority is a bundle's P1–P3, from the triage verdicts of all its
// items and its number of reports (count):
//
//  1. triage.Priority with the bundle's category and area, the highest
//     impact any item got, and count
//  2. count >= ManyReports raises it one level, capped at P1
//  3. spam and praise are always P3 and never raised
func BundlePriority(category, area string, items []loop.Item, count int) string {
	if _, ok := draftable[category]; !ok {
		return "P3"
	}
	impact := "low"
	for _, it := range items {
		if impactRank(impactOf(it)) > impactRank(impact) {
			impact = impactOf(it)
		}
	}
	p := triage.Priority(category, impact, area, count)
	if count >= ManyReports {
		p = map[string]string{"P3": "P2", "P2": "P1", "P1": "P1"}[p]
	}
	return p
}

func impactRank(s string) int {
	return map[string]int{"low": 1, "medium": 2, "high": 3}[s]
}

// impactOf is the model's impact for it. Items triaged before the impact was
// stored fall back to one worked back from their priority.
func impactOf(it loop.Item) string {
	if impactRank(it.Impact) > 0 {
		return it.Impact
	}
	switch it.Priority {
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
	}{{loop.SourceInApp, "アプリ内"}, {loop.SourceStoreReview, "ストアのレビュー"}, {loop.SourceTestFlight, "TestFlight"}, {loop.SourceCrash, "クラッシュ"}, {loop.SourceGenFailure, "生成の失敗"}}
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
