package draft

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
)

// Automatic filing (owner decision 2026-11-14, #365): a pending draft that
// passes the exit check becomes an issue without a person. The rules that
// keep it bounded live here; the posting itself is the same code path as a
// human approval (Reviewer.publish: same body, same append-as-comment
// dedupe, same checks right before the post).

// Caps of the automatic filing: LOOP_ISSUE_AUTO_MAX_PER_RUN and
// LOOP_ISSUE_AUTO_MAX_PER_DAY when unset. The day is a rolling 24 hours.
const (
	DefaultAutoMaxPerRun = 10
	DefaultAutoMaxPerDay = 30
	AutoDayWindow        = 24 * time.Hour
)

// AutoLabel marks every issue the loop filed by itself (a person adds
// agent:* labels later, never the loop).
const AutoLabel = "from-feedback"

// Skip reasons of AutoFiler (audit codes).
const (
	AutoSkipCategory   = "category_not_auto"     // spam / praise / unknown: never filed automatically
	AutoSkipQuestion   = "question_low_priority" // question is filed only at P1 or P2
	AutoSkipCheck      = "check_failed"          // the exit check refused the draft
	AutoSkipRunCap     = "run_cap"
	AutoSkipDayCap     = "day_cap"
	AutoSkipPostFailed = "post_failed"
)

// AutoEligible reports why d may not be filed automatically ("" = it may):
// bug and feature always, question only at P1 or P2, nothing else.
func AutoEligible(d loop.Draft) string {
	switch d.Category {
	case "bug", "feature":
		return ""
	case "question":
		if d.Priority == "P1" || d.Priority == "P2" {
			return ""
		}
		return AutoSkipQuestion
	}
	return AutoSkipCategory
}

// AutoLabels is base plus AutoLabel and priority:<P1-P3>, sorted, each once.
func AutoLabels(base []string, priority string) []string {
	out := append(slices.Clone(base), AutoLabel)
	if priority != "" {
		out = append(out, "priority:"+priority)
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// Automatic wording that replaces the human-approval lines of a body.
const (
	autoSteps        = "(自動で出したため未記入。確かめる人が書く)"
	autoFooterIssue  = "\n---\nフィードバックの要約から、ループが**自動で**出した Issue(人の確認を経ていない・#365 の決定)。原文と個人情報は載せていない(#367)。再現の手順と完了の条件は確かめる人が直す。\n"
	autoFooterAppend = "\n---\nこの Issue を出した後に届いたフィードバックの要約を、ループが**自動で**追記した(人の確認を経ていない)。原文と個人情報は載せていない(#367)。\n"
)

// AutoBody is d's body as auto-issue posts it: the human lines are swapped
// for the automatic ones and, for a new issue, the related open issues the
// builder found (#2288) are listed by number so the person who triages it
// sees them (numbers only: titles are someone else's text). Idempotent.
func AutoBody(d loop.Draft) string {
	body := d.Body
	if d.AppendIssue > 0 {
		if strings.Contains(body, footerAppend) {
			return strings.Replace(body, footerAppend, autoFooterAppend, 1)
		}
		if !strings.Contains(body, autoFooterAppend) {
			body += autoFooterAppend
		}
		return body
	}
	body = strings.Replace(body, stepsPlaceholder, autoSteps, 1)
	var related strings.Builder
	if len(d.Related) > 0 && !strings.Contains(body, "## 関係しそうな Issue") {
		related.WriteString("\n## 関係しそうな Issue(候補)\n\n")
		for _, r := range d.Related {
			fmt.Fprintf(&related, "- #%d\n", r.Number)
		}
		related.WriteString("\n題名の近さで選んだ候補。同じ問題なら確かめる人がこの Issue を閉じて結び付ける。\n")
	}
	switch {
	case strings.Contains(body, footerIssue):
		return strings.Replace(body, footerIssue, related.String()+autoFooterIssue, 1)
	case strings.Contains(body, autoFooterIssue):
		return body
	}
	return body + related.String() + autoFooterIssue
}

// AutoApprove files a pending draft without a person: the exit check must
// pass on the automatic body, the approval is recorded as loop.ActorLoop
// and the draft is posted at once with labels. A draft the loop approved
// earlier but could not post (approved, ReviewedBy loop) is posted again.
// There is no dry run: without a repo it refuses (fail closed).
func (r *Reviewer) AutoApprove(ctx context.Context, id string, labels []string) (*loop.Draft, *github.Issue, error) {
	if r.Repo == "" {
		return nil, nil, errors.New("auto-issue needs a target repository (LOOP_ISSUE_REPO)")
	}
	d, err := r.get(ctx, id, loop.DraftPending, loop.DraftApproved)
	if err != nil {
		return nil, nil, err
	}
	if d.Status == loop.DraftApproved {
		if d.ReviewedBy != loop.ActorLoop {
			return nil, nil, fmt.Errorf("draft %s was approved by %s; post it with drafts publish", id, d.ReviewedBy)
		}
		return r.publish(ctx, d, labels)
	}
	d.Body = AutoBody(*d)
	reasons, err := r.CheckDraft(ctx, d)
	if err != nil {
		return nil, nil, err
	}
	if len(reasons) > 0 {
		return nil, nil, fmt.Errorf("%w: %s", ErrCheck, strings.Join(reasons, ", "))
	}
	now := r.now()
	d.Status, d.ReviewedBy, d.ReviewedAt, d.UpdatedAt = loop.DraftApproved, loop.ActorLoop, now, now
	d.BodyHash = BodyHash(d.Title, d.Body)
	if err := r.Store.PutDraft(ctx, *d); err != nil {
		return nil, nil, err
	}
	return r.publish(ctx, d, labels)
}

// AutoFiler runs the automatic filing over the queue.
type AutoFiler struct {
	Reviewer *Reviewer
	// MaxPerRun / MaxPerDay: 0 = the defaults.
	MaxPerRun int
	MaxPerDay int
	// DryRun only counts what would be filed; nothing is approved or posted.
	DryRun bool
	// OnFiled is called after each post (audit). An error stops the run.
	OnFiled func(ctx context.Context, d *loop.Draft, iss *github.Issue) error
	// OnSkipped is called for each draft left in the queue with the reason
	// (and the error for AutoSkipCheck / AutoSkipPostFailed). An error
	// stops the run.
	OnSkipped func(ctx context.Context, d loop.Draft, reason string, err error) error
}

// AutoResult counts one run.
type AutoResult struct {
	Filed    int // new issues
	Appended int // comments on filed issues
	Skipped  int // not eligible or the check refused
	Failed   int // the post failed (the draft stays approved by the loop; the next run retries)
	Left     int // eligible but over a cap
	// FiledToday is how many the loop filed in the last AutoDayWindow before this run.
	FiledToday int
	// Would lists the draft ids a dry run would file.
	Would []string
}

func (f *AutoFiler) caps() (int, int) {
	run, day := f.MaxPerRun, f.MaxPerDay
	if run <= 0 {
		run = DefaultAutoMaxPerRun
	}
	if day <= 0 {
		day = DefaultAutoMaxPerDay
	}
	return run, day
}

// FiledSince counts the drafts the loop itself posted at or after since.
func FiledSince(ctx context.Context, s loop.Store, since time.Time) (int, error) {
	ds, err := s.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPublished, Limit: 500})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range ds {
		if d.ReviewedBy == loop.ActorLoop && !d.PublishedAt.IsZero() && !d.PublishedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

// Run files the eligible drafts, highest priority first (oldest first
// within a priority), until a cap is reached. Drafts the loop approved
// earlier but failed to post come first.
func (f *AutoFiler) Run(ctx context.Context) (AutoResult, error) {
	var res AutoResult
	r := f.Reviewer
	maxRun, maxDay := f.caps()
	now := r.now()
	var err error
	if res.FiledToday, err = FiledSince(ctx, r.Store, now.Add(-AutoDayWindow)); err != nil {
		return res, err
	}
	queue, err := f.queue(ctx)
	if err != nil {
		return res, err
	}
	posted := 0
	for _, d := range queue {
		if reason := AutoEligible(d); reason != "" {
			res.Skipped++
			if err := f.skipped(ctx, d, reason, nil); err != nil {
				return res, err
			}
			continue
		}
		if f.DryRun {
			d.Body = AutoBody(d)
			if reasons, err := r.CheckDraft(ctx, &d); err != nil {
				return res, err
			} else if len(reasons) > 0 {
				res.Skipped++
				continue
			}
			res.Would = append(res.Would, d.ID)
			continue
		}
		if posted >= maxRun || res.FiledToday+posted >= maxDay {
			res.Left++
			continue
		}
		labels := AutoLabels(r.Labels, d.Priority)
		got, iss, perr := r.AutoApprove(ctx, d.ID, labels)
		switch {
		case errors.Is(perr, ErrCheck):
			res.Skipped++
			if err := f.skipped(ctx, d, AutoSkipCheck, perr); err != nil {
				return res, err
			}
			continue
		case perr != nil:
			res.Failed++
			if err := f.skipped(ctx, d, AutoSkipPostFailed, perr); err != nil {
				return res, err
			}
			continue
		}
		posted++
		if got.AppendIssue > 0 {
			res.Appended++
		} else {
			res.Filed++
		}
		if f.OnFiled != nil {
			if err := f.OnFiled(ctx, got, iss); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

func (f *AutoFiler) skipped(ctx context.Context, d loop.Draft, reason string, err error) error {
	if f.OnSkipped == nil {
		return nil
	}
	return f.OnSkipped(ctx, d, reason, err)
}

// queue is the loop's own unposted approvals, then the pending drafts by
// priority (P1 first) and age (oldest first).
func (f *AutoFiler) queue(ctx context.Context) ([]loop.Draft, error) {
	approved, err := f.Reviewer.Store.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftApproved, Limit: 500})
	if err != nil {
		return nil, err
	}
	pending, err := f.Reviewer.Store.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPending, Limit: 500})
	if err != nil {
		return nil, err
	}
	rank := func(d loop.Draft) int { return map[string]int{"P1": 3, "P2": 2, "P3": 1}[d.Priority] }
	sort.SliceStable(pending, func(i, j int) bool {
		if rank(pending[i]) != rank(pending[j]) {
			return rank(pending[i]) > rank(pending[j])
		}
		return pending[i].CreatedAt.Before(pending[j].CreatedAt)
	})
	out := make([]loop.Draft, 0, len(approved)+len(pending))
	for _, d := range approved {
		if d.ReviewedBy == loop.ActorLoop {
			out = append(out, d)
		}
	}
	return append(out, pending...), nil
}
