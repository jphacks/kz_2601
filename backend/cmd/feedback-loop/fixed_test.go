package main

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/fixed"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
)

type fakeIssues map[int]github.Fix

func (f fakeIssues) IssueFix(_ context.Context, _ string, n int) (github.Fix, error) {
	return f[n], nil
}

type recPusher struct{ uids []string }

func (r *recPusher) Push(_ context.Context, uid, _, _ string) (bool, error) {
	r.uids = append(r.uids, uid)
	return true, nil
}

type allowAll struct{}

func (allowAll) PushAllowed(context.Context, string) (bool, error) { return true, nil }

func TestFixedFlow(t *testing.T) {
	ctx := context.Background()
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", envIssueRepo: "owner/repo"})
	ta.model = &fakeModel{category: "bug", cluster: "photo", summary: "写真を追加すると終了する。"}
	ta.publisher = &fakePublisher{}
	seedFeedback(t, ta, "写真を追加すると落ちる", "写真の追加で落ちます")
	ta.must(t, "ingest")
	ta.must(t, "triage")
	ta.must(t, "draft")
	ds, _ := ta.ls.ListDrafts(ctx, loop.DraftQuery{})
	ta.must(t, "drafts", "approve", "-by", "ko-tarou", ds[0].ID)

	ta.issues = fakeIssues{77: {Fixed: true, PR: 900}}
	out := ta.must(t, "fixed", "-dry-run")
	if !strings.Contains(out, "fixed 1, notices 0") || !strings.Contains(out, "owner/repo#77 by PR #900") {
		t.Fatalf("dry run: %s", out)
	}
	// push is off unless LOOP_NOTIFY_PUSH=on: in-app only
	pusher := &recPusher{}
	ta.pusher, ta.consent = pusher, allowAll{}
	out = ta.must(t, "fixed")
	if !strings.Contains(out, "notices 2, pushes 0") || len(pusher.uids) != 0 || !strings.Contains(out, "release notes draft") {
		t.Fatalf("fixed: %s", out)
	}
	out = ta.must(t, "notices", "-uid", "ua")
	if !strings.Contains(out, fixed.NoticeTitle) || !strings.Contains(out, "1 notice(s)") {
		t.Fatalf("notices: %s", out)
	}
	evs, _ := ta.ctl.ListAudit(ctx, loop.AuditQuery{DraftID: ds[0].ID, Limit: 500})
	got := map[string]bool{}
	for _, e := range evs {
		got[e.Action] = true
	}
	if !got["fixed"] || !got["noticed"] {
		t.Fatalf("audit %v", got)
	}
	// stopped stage
	ta.env["LOOP_NOTIFY"] = ""
	if err := ta.run(ctx, []string{"fixed"}); err == nil {
		t.Fatal("ran while stopped")
	}
}

func TestFixedPushWhenOn(t *testing.T) {
	ctx := context.Background()
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", envIssueRepo: "owner/repo", envNotifyPush: "on"})
	ta.model = &fakeModel{category: "bug", cluster: "photo", summary: "写真を追加すると終了する。"}
	ta.publisher = &fakePublisher{}
	seedFeedback(t, ta, "写真を追加すると落ちる")
	ta.must(t, "ingest")
	ta.must(t, "triage")
	ta.must(t, "draft")
	ds, _ := ta.ls.ListDrafts(ctx, loop.DraftQuery{})
	ta.must(t, "drafts", "approve", "-by", "ko-tarou", ds[0].ID)
	ta.issues = fakeIssues{77: {Fixed: true, PR: 900}}
	pusher := &recPusher{}
	ta.pusher, ta.consent = pusher, allowAll{}
	out := ta.must(t, "fixed")
	if !strings.Contains(out, "pushes 1") || len(pusher.uids) != 1 {
		t.Fatalf("%s %v", out, pusher.uids)
	}
	// with push on but no APNs settings, the real pusher refuses to start
	ta.pusher = nil
	if err := ta.run(ctx, []string{"fixed"}); err == nil {
		t.Fatal("push on without APNs settings accepted")
	}
}
