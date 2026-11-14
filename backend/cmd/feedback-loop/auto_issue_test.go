package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/draft"
)

// TestAutoIssueFlow: ingest → triage → draft → auto-issue files the checked
// draft with the fixed labels, records it, and does not file it twice.
func TestAutoIssueFlow(t *testing.T) {
	ctx := context.Background()
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", envIssueRepo: "owner/repo", envIssueLabels: "feedback"})
	ta.model = &fakeModel{category: "bug", cluster: "photo-add-crash", summary: "写真を追加するとアプリが終了する。"}
	pub := &fakePublisher{}
	ta.publisher = pub
	seedFeedback(t, ta, "写真を追加しようとすると毎回アプリが落ちます。電話 090-1234-5678", "写真の追加で落ちます")
	ta.must(t, "ingest")
	ta.must(t, "triage")
	ta.must(t, "draft")

	out := ta.must(t, "auto-issue", "-dry-run")
	if !strings.Contains(out, "would file 1") || len(pub.bodies) != 0 {
		t.Fatalf("dry run: %s", out)
	}
	out = ta.must(t, "auto-issue")
	if !strings.Contains(out, "auto-filed drf_") || !strings.Contains(out, "owner/repo#77") || !strings.Contains(out, "auto-issue: new 1, appended 0") {
		t.Fatalf("out: %s", out)
	}
	if len(pub.bodies) != 1 || strings.Join(pub.labels[0], ",") != "feedback,from-feedback,priority:P2" {
		t.Fatalf("posted %d, labels %v", len(pub.bodies), pub.labels)
	}
	if !strings.Contains(pub.bodies[0], "自動で**出した") || !strings.Contains(pub.bodies[0], "- 件数: ") {
		t.Fatalf("body:\n%s", pub.bodies[0])
	}
	for _, leak := range []string{"090", "毎回アプリが落ちます", "fb_", "lin_", "drafts edit"} {
		if strings.Contains(pub.titles[0]+pub.bodies[0], leak) {
			t.Fatalf("issue holds %q", leak)
		}
	}
	ds, _ := ta.ls.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPublished})
	if len(ds) != 1 || ds[0].ReviewedBy != loop.ActorLoop || ds[0].IssueNumber != 77 {
		t.Fatalf("drafts %+v", ds)
	}
	out = ta.must(t, "audit")
	for _, want := range []string{"issue_auto", "draft_approved", "issue_created", "auto_run"} {
		if !strings.Contains(out, want) {
			t.Errorf("audit lacks %s:\n%s", want, out)
		}
	}
	if out := ta.must(t, "auto-issue"); !strings.Contains(out, "new 0, appended 0") || len(pub.bodies) != 1 {
		t.Fatalf("filed twice: %s", out)
	}
}

func TestAutoIssueStopsByEnvAndSwitches(t *testing.T) {
	ctx := context.Background()
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", envIssueRepo: "owner/repo", "LOOP_ISSUE_AUTO": ""})
	ta.model = &fakeModel{category: "feature", cluster: "csv-export", summary: "CSV の書き出しが欲しい。"}
	pub := &fakePublisher{}
	ta.publisher = pub
	seedFeedback(t, ta, "CSV で書き出せるようにしてほしい")
	ta.must(t, "ingest")
	ta.must(t, "triage")
	ta.must(t, "draft")

	// off by default (fail closed)
	if err := ta.run(ctx, []string{"auto-issue"}); !errors.Is(err, loop.ErrStopped) || len(pub.bodies) != 0 {
		t.Fatalf("ran with LOOP_ISSUE_AUTO unset: %v", err)
	}
	ta.env["LOOP_ISSUE_AUTO"] = "on"
	// the Firestore switch of the stage, of publish, and of all each stop it
	for _, sw := range []string{loop.StageIssueAuto, loop.StagePublish, loop.SwitchAll} {
		ta.must(t, "switch", "set", "-by", "ko-tarou", sw, "off")
		err := ta.run(ctx, []string{"auto-issue"})
		if (sw == loop.StagePublish && err == nil) || (sw != loop.StagePublish && !errors.Is(err, loop.ErrStopped)) || len(pub.bodies) != 0 {
			t.Fatalf("switch %s off ignored: %v (%d posted)", sw, err, len(pub.bodies))
		}
		ta.must(t, "switch", "set", "-by", "ko-tarou", sw, "on")
	}
	// no repo: refuses rather than dry-running by accident
	ta.env[envIssueRepo] = ""
	if err := ta.run(ctx, []string{"auto-issue"}); err == nil || !strings.Contains(err.Error(), envIssueRepo) {
		t.Fatalf("no repo accepted: %v", err)
	}
	ta.env[envIssueRepo] = "owner/repo"
	// bad caps refuse to start
	for _, bad := range []env{{envIssueAutoMaxPerRun: "0"}, {envIssueAutoMaxPerDay: "x"}} {
		for k, v := range bad {
			ta.env[k] = v
			if err := ta.run(ctx, []string{"auto-issue"}); err == nil || !strings.Contains(err.Error(), k) {
				t.Fatalf("%s=%s accepted: %v", k, v, err)
			}
			ta.env[k] = ""
		}
	}
	// agent:* labels are refused for the automatic path too
	ta.env[envIssueLabels] = "agent:ready"
	if err := ta.run(ctx, []string{"auto-issue"}); err == nil || !strings.Contains(err.Error(), "refusing to start") {
		t.Fatalf("agent label accepted: %v", err)
	}
	ta.env[envIssueLabels] = ""
	out := ta.must(t, "auto-issue")
	if !strings.Contains(out, "new 1") || len(pub.bodies) != 1 || strings.Join(pub.labels[0], ",") != draft.AutoLabel+",priority:P2" {
		t.Fatalf("out: %s labels=%v", out, pub.labels)
	}
}
