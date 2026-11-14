package draft

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
)

// Publisher creates the issue, or comments on the filed one for an append
// draft after checking that it is still open (github.Client).
type Publisher interface {
	CreateIssue(ctx context.Context, repo, title, body string, labels []string) (*github.Issue, error)
	CommentIssue(ctx context.Context, repo string, n int, body string) (*github.Issue, error)
	IssueOpen(ctx context.Context, repo string, n int) (bool, error)
}

// Reviewer is the human side of the queue: every method is run by a person
// from the CLI (`feedback-loop drafts ...`), named by By.
type Reviewer struct {
	Store loop.Store
	// Publisher and Repo: where an approved draft is posted. Repo "" = dry
	// run (approve records the decision; nothing is posted).
	Publisher Publisher
	Repo      string
	Labels    []string
	Now       func() time.Time
	// BeforePublish, when set, may refuse posting (stop switch, #375).
	BeforePublish func(ctx context.Context) error
}

// ErrCheck is a draft that fails the exit check.
var ErrCheck = errors.New("draft fails the exit check")

// ErrIssueClosed: an append draft's issue was closed on GitHub before the
// comment was posted. The draft was drafted again as a new issue and waits
// for approval.
var ErrIssueClosed = errors.New("the filed issue is closed")

func (r *Reviewer) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func (r *Reviewer) get(ctx context.Context, id string, states ...string) (*loop.Draft, error) {
	d, err := r.Store.GetDraft(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, s := range states {
		if d.Status == s {
			return d, nil
		}
	}
	return nil, fmt.Errorf("draft %s is %s (needs %s)", id, d.Status, strings.Join(states, " or "))
}

// CheckDraft runs the exit check against the draft's current items.
func (r *Reviewer) CheckDraft(ctx context.Context, d *loop.Draft) ([]string, error) {
	items, err := LoadItems(ctx, r.Store, d.ItemIDs)
	if err != nil {
		return nil, err
	}
	return Check(d.Title, d.Body, texts(items)), nil
}

// Edit replaces the title and/or body of a pending draft (the human writes
// the reproduction steps here). The result must pass the exit check.
func (r *Reviewer) Edit(ctx context.Context, id, by, title, body string) (*loop.Draft, error) {
	if strings.TrimSpace(by) == "" {
		return nil, errors.New("who is editing? (-by)")
	}
	d, err := r.get(ctx, id, loop.DraftPending)
	if err != nil {
		return nil, err
	}
	if title != "" {
		d.Title = strings.TrimSpace(title)
	}
	if body != "" {
		d.Body = body
	}
	reasons, err := r.CheckDraft(ctx, d)
	if err != nil {
		return nil, err
	}
	if len(reasons) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrCheck, strings.Join(reasons, ", "))
	}
	d.UpdatedAt = r.now()
	return d, r.Store.PutDraft(ctx, *d)
}

// Reject closes a pending draft. reason stays in the private queue only.
func (r *Reviewer) Reject(ctx context.Context, id, by, reason string) (*loop.Draft, error) {
	if strings.TrimSpace(by) == "" {
		return nil, errors.New("who is rejecting? (-by)")
	}
	d, err := r.get(ctx, id, loop.DraftPending)
	if err != nil {
		return nil, err
	}
	now := r.now()
	d.Status, d.ReviewedBy, d.ReviewedAt, d.RejectReason, d.UpdatedAt = loop.DraftRejected, by, now, reason, now
	return d, r.Store.PutDraft(ctx, *d)
}

// Approve is 承認 1 (#365): a human says this draft may become an issue. It
// re-runs the exit check, records who approved and the hash of the text,
// then posts it when a repo is configured. With no repo it stops at
// "approved" (dry run); Publish posts it later.
func (r *Reviewer) Approve(ctx context.Context, id, by string) (*loop.Draft, *github.Issue, error) {
	if strings.TrimSpace(by) == "" {
		return nil, nil, errors.New("who is approving? (-by)")
	}
	d, err := r.get(ctx, id, loop.DraftPending)
	if err != nil {
		return nil, nil, err
	}
	reasons, err := r.CheckDraft(ctx, d)
	if err != nil {
		return nil, nil, err
	}
	if len(reasons) > 0 {
		return nil, nil, fmt.Errorf("%w: %s", ErrCheck, strings.Join(reasons, ", "))
	}
	now := r.now()
	d.Status, d.ReviewedBy, d.ReviewedAt, d.UpdatedAt = loop.DraftApproved, by, now, now
	d.BodyHash = BodyHash(d.Title, d.Body)
	if err := r.Store.PutDraft(ctx, *d); err != nil {
		return nil, nil, err
	}
	if r.Repo == "" {
		return d, nil, nil
	}
	return r.publish(ctx, d)
}

// Publish posts an approved draft (after a dry run, or a failed post).
func (r *Reviewer) Publish(ctx context.Context, id string) (*loop.Draft, *github.Issue, error) {
	d, err := r.get(ctx, id, loop.DraftApproved)
	if err != nil {
		return nil, nil, err
	}
	if r.Repo == "" {
		return d, nil, errors.New("no target repository (LOOP_ISSUE_REPO): dry run")
	}
	return r.publish(ctx, d)
}

func (r *Reviewer) publish(ctx context.Context, d *loop.Draft) (*loop.Draft, *github.Issue, error) {
	if r.BeforePublish != nil {
		if err := r.BeforePublish(ctx); err != nil {
			return d, nil, err
		}
	}
	if r.Publisher == nil {
		return d, nil, errors.New("no publisher configured (LOOP_GITHUB_TOKEN)")
	}
	// the text must still be exactly what the human approved
	if BodyHash(d.Title, d.Body) != d.BodyHash {
		return d, nil, errors.New("draft changed after approval; approve it again")
	}
	reasons, err := r.CheckDraft(ctx, d)
	if err != nil {
		return d, nil, err
	}
	if len(reasons) > 0 {
		return d, nil, fmt.Errorf("%w: %s", ErrCheck, strings.Join(reasons, ", "))
	}
	var iss *github.Issue
	if d.AppendIssue > 0 {
		iss, err = r.appendTo(ctx, d)
	} else {
		iss, err = r.Publisher.CreateIssue(ctx, r.Repo, d.Title, d.Body, r.Labels)
	}
	if err != nil {
		return d, nil, err
	}
	now := r.now()
	d.Status, d.IssueRepo, d.IssueNumber, d.PublishedAt, d.UpdatedAt = loop.DraftPublished, r.Repo, iss.Number, now, now
	return d, iss, r.Store.PutDraft(ctx, *d)
}

// appendTo posts an append draft as a comment on its filed issue. The issue
// must still be the cluster's open one in the target repo; otherwise the
// approved text no longer fits, and the draft is rejected and drafted again.
func (r *Reviewer) appendTo(ctx context.Context, d *loop.Draft) (*github.Issue, error) {
	if d.AppendRepo != r.Repo {
		return nil, fmt.Errorf("draft adds to %s#%d but the target is %s; reject it and run draft again", d.AppendRepo, d.AppendIssue, r.Repo)
	}
	filed, err := FiledIssues(ctx, r.Store)
	if err != nil {
		return nil, err
	}
	if f := filed[d.ClusterKey]; f.Repo != d.AppendRepo || f.Number != d.AppendIssue {
		return nil, fmt.Errorf("%s#%d is no longer the open issue of this cluster (fixed?); reject the draft and run draft again", d.AppendRepo, d.AppendIssue)
	}
	// FixedAt is set only for issues closed by a merged PR, and only when
	// the notifier has run: ask GitHub (closed as not planned or duplicate
	// counts too)
	open, err := r.Publisher.IssueOpen(ctx, d.AppendRepo, d.AppendIssue)
	if err != nil {
		return nil, err
	}
	if !open {
		if err := r.redraftAsNew(ctx, d); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s#%d; drafted again as a new issue (%s), approve it again", ErrIssueClosed, d.AppendRepo, d.AppendIssue, d.ID)
	}
	return r.Publisher.CommentIssue(ctx, d.AppendRepo, d.AppendIssue, d.Body)
}

// redraftAsNew marks the closed issue so later runs stop adding to it, and
// turns d back into a pending draft for a new issue.
func (r *Reviewer) redraftAsNew(ctx context.Context, d *loop.Draft) error {
	now := r.now()
	ds, err := r.Store.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPublished, Limit: 500})
	if err != nil {
		return err
	}
	for _, p := range ds {
		if p.IssueRepo == d.AppendRepo && p.IssueNumber == d.AppendIssue && p.ClosedAt.IsZero() {
			p.ClosedAt, p.UpdatedAt = now, now
			if err := r.Store.PutDraft(ctx, p); err != nil {
				return err
			}
		}
	}
	items, err := LoadItems(ctx, r.Store, d.ItemIDs)
	if err != nil {
		return err
	}
	d.AppendRepo, d.AppendIssue = "", 0
	if len(items) > 0 {
		fill(d, items, now)
	}
	d.Status, d.ReviewedBy, d.ReviewedAt, d.BodyHash, d.UpdatedAt = loop.DraftPending, "", time.Time{}, "", now
	return r.Store.PutDraft(ctx, *d)
}
