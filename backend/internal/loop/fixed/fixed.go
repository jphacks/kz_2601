// Package fixed tells the people whose feedback led to an issue that it was
// fixed (#374: 「あなたの声で直りました」). When a published draft's issue is
// closed by a merged PR, each in-app sender gets one in-app notice
// (loop_notices); a push is sent as well only when push is on, the sender
// agreed to it, and they were not pushed recently. The text is a fixed
// template plus the human-approved issue title: no LLM writes it.
package fixed

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/draft"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
)

// Notice text (fixed template).
const (
	NoticeTitle = "あなたの声で直りました"
	noticeBody  = "お送りいただいたフィードバックをもとに「%s」を直しました。ありがとうございます。"
)

// DefaultMinPushInterval: at most one push per sender per week (#374: 通知の頻度の上限).
const DefaultMinPushInterval = 7 * 24 * time.Hour

// Issues reads how an issue ended (github.Client).
type Issues interface {
	IssueFix(ctx context.Context, repo string, n int) (github.Fix, error)
}

// Pusher sends one push to uid's devices; sent=false when there was nothing
// to send to.
type Pusher interface {
	Push(ctx context.Context, uid, title, body string) (sent bool, err error)
}

// Consent tells whether uid agreed to this kind of push.
type Consent interface {
	PushAllowed(ctx context.Context, uid string) (bool, error)
}

// Users tells whether an account is deleted.
type Users interface {
	UserDeleted(ctx context.Context, uid string) (bool, error)
}

// Notifier runs one pass.
type Notifier struct {
	Store  loop.Store
	Issues Issues
	Users  Users
	// Pusher and Consent: nil Pusher = in-app notices only.
	Pusher          Pusher
	Consent         Consent
	MinPushInterval time.Duration
	Now             func() time.Time
	// OnEvent, when set, records each step (audit). An error stops the run.
	OnEvent func(ctx context.Context, e loop.Event) error
	// DryRun only reports what would change.
	DryRun bool
}

// Fixed is one draft found fixed in this run.
type Fixed struct {
	DraftID string
	Title   string
	Repo    string
	Issue   int
	PR      int
}

// Result counts one run.
type Result struct {
	Checked int
	Fixed   []Fixed
	Notices int
	Pushes  int
	Skipped int // deleted accounts, notices that already existed
}

func (n *Notifier) now() time.Time {
	if n.Now != nil {
		return n.Now().UTC()
	}
	return time.Now().UTC()
}

func (n *Notifier) event(ctx context.Context, e loop.Event) error {
	if n.OnEvent == nil || n.DryRun {
		return nil
	}
	e.Stage = loop.StageNotify
	return n.OnEvent(ctx, e)
}

// Run checks every published draft that is not fixed yet.
func (n *Notifier) Run(ctx context.Context) (Result, error) {
	var res Result
	ds, err := n.Store.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPublished, Limit: 500})
	if err != nil {
		return res, err
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].ID < ds[j].ID })
	for _, d := range ds {
		if !d.NotifiedAt.IsZero() || d.IssueNumber <= 0 {
			continue
		}
		res.Checked++
		if d.FixedAt.IsZero() {
			fix, err := n.Issues.IssueFix(ctx, d.IssueRepo, d.IssueNumber)
			if err != nil {
				return res, err
			}
			if !fix.Fixed {
				continue
			}
			d.FixedPR, d.FixedAt = fix.PR, fix.At
			if d.FixedAt.IsZero() {
				d.FixedAt = n.now()
			}
			if err := n.event(ctx, loop.Event{Action: "fixed", DraftID: d.ID, IssueRepo: d.IssueRepo, Issue: d.IssueNumber, PR: d.FixedPR}); err != nil {
				return res, err
			}
		}
		res.Fixed = append(res.Fixed, Fixed{DraftID: d.ID, Title: d.Title, Repo: d.IssueRepo, Issue: d.IssueNumber, PR: d.FixedPR})
		if n.DryRun {
			continue
		}
		if err := n.notify(ctx, d, &res); err != nil {
			return res, err
		}
		d.NotifiedAt, d.UpdatedAt = n.now(), n.now()
		if err := n.Store.PutDraft(ctx, d); err != nil {
			return res, err
		}
	}
	return res, nil
}

// notify creates one notice per in-app sender of d.
func (n *Notifier) notify(ctx context.Context, d loop.Draft, res *Result) error {
	items, err := draft.LoadItems(ctx, n.Store, d.ItemIDs)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	now := n.now()
	interval := n.MinPushInterval
	if interval <= 0 {
		interval = DefaultMinPushInterval
	}
	for _, it := range items {
		if it.Source != loop.SourceInApp || it.UID == "" || seen[it.UID] {
			continue
		}
		seen[it.UID] = true
		if n.Users != nil {
			deleted, err := n.Users.UserDeleted(ctx, it.UID)
			if err != nil {
				return err
			}
			if deleted {
				res.Skipped++
				continue
			}
		}
		notice := loop.Notice{ID: loop.NoticeID(d.ID, it.UID), UID: it.UID, DraftID: d.ID, Title: NoticeTitle,
			Body: fmt.Sprintf(noticeBody, publicTitle(d.Title)), CreatedAt: now, ExpiresAt: now.Add(loop.Retention)}
		// decide before storing, push after: a re-run finds the notice and
		// never pushes twice. Pushed means "a push was attempted", which is
		// what the rate limit counts.
		push, err := n.shouldPush(ctx, it.UID, interval)
		if err != nil {
			return err
		}
		notice.Pushed = push
		created, err := n.Store.PutNotice(ctx, notice)
		if err != nil {
			return err
		}
		if !created {
			res.Skipped++
			continue
		}
		res.Notices++
		action := "noticed"
		if push {
			sent, perr := n.Pusher.Push(ctx, it.UID, notice.Title, notice.Body)
			switch {
			case perr != nil:
				action = "noticed_push_failed"
			case sent:
				res.Pushes++
				action = "noticed_pushed"
			default:
				action = "noticed_no_device"
			}
		}
		if err := n.event(ctx, loop.Event{Action: action, ItemID: it.ID, DraftID: d.ID, Issue: d.IssueNumber, IssueRepo: d.IssueRepo, PR: d.FixedPR}); err != nil {
			return err
		}
	}
	return nil
}

func (n *Notifier) shouldPush(ctx context.Context, uid string, interval time.Duration) (bool, error) {
	if n.Pusher == nil || n.Consent == nil {
		return false, nil
	}
	ok, err := n.Consent.PushAllowed(ctx, uid)
	if err != nil || !ok {
		return false, err
	}
	last, err := n.Store.LastPush(ctx, uid)
	if err != nil {
		return false, err
	}
	return last.IsZero() || n.now().Sub(last) >= interval, nil
}

// publicTitle drops the "[フィードバック] 不具合: " prefix of a draft title.
func publicTitle(t string) string {
	for _, p := range []string{"[フィードバック] 不具合: ", "[フィードバック] 要望: ", "[フィードバック] 使い方の質問: "} {
		if len(t) > len(p) && t[:len(p)] == p {
			return t[len(p):]
		}
	}
	return t
}

// ReleaseNotes is a draft of the release notes for the fixed drafts (#374:
// 人が確認して出す). It is printed, never published.
func ReleaseNotes(fs []Fixed) string {
	if len(fs) == 0 {
		return ""
	}
	out := "## 直したこと(フィードバックから)\n\n"
	for _, f := range fs {
		out += fmt.Sprintf("- %s(#%d)\n", publicTitle(f.Title), f.Issue)
	}
	return out
}
