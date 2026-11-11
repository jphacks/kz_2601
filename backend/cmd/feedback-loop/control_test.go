package main

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func TestStagesStopByDefaultAndBySwitch(t *testing.T) {
	ctx := context.Background()
	ta := newTestApp(t, env{"LOOP_TRIAGE": ""})
	fm := &fakeModel{category: "bug", cluster: "c", summary: "終了する。"}
	ta.model = fm
	err := ta.run(ctx, []string{"triage"})
	if !errors.Is(err, loop.ErrStopped) || fm.calls != 0 {
		t.Fatalf("triage ran with LOOP_TRIAGE unset: %v calls=%d", err, fm.calls)
	}

	ta.env["LOOP_TRIAGE"] = "on"
	ta.must(t, "switch", "set", "-by", "ko-tarou", "triage", "off")
	if err := ta.run(ctx, []string{"triage"}); !errors.Is(err, loop.ErrStopped) || fm.calls != 0 {
		t.Fatalf("firestore switch ignored: %v", err)
	}
	out := ta.must(t, "switch", "show")
	if !regexp.MustCompile(`triage\s+on\s+off\s+no: loop_control/switches.triage is off`).MatchString(out) {
		t.Fatalf("show:\n%s", out)
	}
	ta.must(t, "switch", "set", "-by", "ko-tarou", "all", "off")
	if err := ta.run(ctx, []string{"ingest"}); !errors.Is(err, loop.ErrStopped) {
		t.Fatalf("all switch ignored by ingest: %v", err)
	}
	ta.must(t, "switch", "set", "-by", "ko-tarou", "all", "on")
	ta.must(t, "switch", "set", "-by", "ko-tarou", "triage", "on")
	ta.must(t, "triage")

	for _, bad := range [][]string{{"switch", "set", "triage", "off"}, {"switch", "set", "-by", "x", "merge", "off"}, {"switch", "set", "-by", "x", "triage", "maybe"}} {
		if err := ta.run(ctx, bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	out = ta.must(t, "audit")
	for _, want := range []string{"switch_off", "switch_on", "stopped"} {
		if !strings.Contains(out, want) {
			t.Errorf("audit lacks %s:\n%s", want, out)
		}
	}
}

func TestMonthlyTokenCapStopsTriage(t *testing.T) {
	ctx := context.Background()
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", "LOOP_MONTHLY_TOKEN_CAP": "15"})
	fm := &fakeModel{category: "bug", cluster: "c", summary: "終了する。"}
	ta.model = fm
	seedFeedback(t, ta, "一つ目の不具合", "二つ目の不具合")
	ta.must(t, "ingest")
	out := ta.must(t, "triage", "-batch", "1")
	// the first call uses 15 tokens (the cap); the second is refused before it is made
	if fm.calls != 1 || !strings.Contains(out, "stopped early") || !strings.Contains(out, "token cap") {
		t.Fatalf("calls %d out %s", fm.calls, out)
	}
	if err := ta.run(ctx, []string{"triage"}); !errors.Is(err, loop.ErrStopped) {
		t.Fatalf("cap ignored: %v", err)
	}
}

// #375 完了の条件: one piece of feedback can be followed to its issue.
func TestAuditTracesFeedbackToIssue(t *testing.T) {
	ctx := context.Background()
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", envIssueRepo: "owner/repo"})
	ta.model = &fakeModel{category: "bug", cluster: "photo", summary: "写真を追加すると終了する。"}
	ta.publisher = &fakePublisher{}
	seedFeedback(t, ta, "写真を追加すると落ちる")
	ta.must(t, "ingest")
	ta.must(t, "triage")
	ta.must(t, "draft")
	ds, _ := ta.ls.ListDrafts(ctx, loop.DraftQuery{})
	ta.must(t, "drafts", "approve", "-by", "ko-tarou", ds[0].ID)

	fbs, _ := ta.fs.ListFeedback(ctx, store.FeedbackQuery{})
	fbID := fbs[0].ID
	evs, _ := ta.ctl.ListAudit(ctx, loop.AuditQuery{Limit: 500})
	itemID := ""
	for _, e := range evs {
		if e.FeedbackID == fbID && e.Action == "ingested" {
			itemID = e.ItemID
		}
	}
	if itemID == "" {
		t.Fatal("no ingest event for the feedback")
	}
	draftID := ""
	for _, e := range evs {
		if e.ItemID == itemID && e.Stage == loop.StageDraft {
			draftID = e.DraftID
		}
	}
	steps := map[string]bool{}
	for _, e := range evs {
		if e.ItemID == itemID || (draftID != "" && e.DraftID == draftID) {
			steps[e.Stage+"/"+e.Action] = true
			if e.Action == "issue_created" && (e.Issue != 77 || e.IssueRepo != "owner/repo" || e.Actor != "ko-tarou" || e.BodyHash == "") {
				t.Fatalf("issue event %+v", e)
			}
		}
	}
	for _, want := range []string{"intake/ingested", "triage/triaged", "draft/drafted", "draft/draft_created", "draft/draft_approved", "publish/issue_created"} {
		if !steps[want] {
			t.Errorf("trace lacks %s (have %v)", want, steps)
		}
	}
	// the log holds ids and codes, never the text
	for _, e := range evs {
		if strings.Contains(e.Reason, "落ちる") {
			t.Fatalf("text in audit: %+v", e)
		}
	}
}
