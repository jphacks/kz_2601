package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
)

type fakePublisher struct{ titles, bodies []string }

func (f *fakePublisher) CreateIssue(_ context.Context, repo, title, body string, _ []string) (*github.Issue, error) {
	f.titles, f.bodies = append(f.titles, title), append(f.bodies, body)
	return &github.Issue{Number: 77, URL: "https://github.com/" + repo + "/issues/77"}, nil
}

var reDraftID = regexp.MustCompile(`drf_[0-9a-f]+`)

func TestDraftsFlow(t *testing.T) {
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01"})
	ta.model = &fakeModel{category: "bug", cluster: "photo-add-crash", summary: "写真を追加するとアプリが終了する。"}
	secretish := "写真を追加しようとすると毎回アプリが落ちます。私の電話は 090-1234-5678 です"
	seedFeedback(t, ta, secretish, "写真の追加で落ちます")
	ta.must(t, "ingest")
	ta.must(t, "triage")
	out := ta.must(t, "draft")
	if !strings.Contains(out, "draft: new 1") {
		t.Fatalf("draft: %s", out)
	}
	out = ta.must(t, "drafts", "list")
	id := reDraftID.FindString(out)
	if id == "" || !strings.Contains(out, "1 draft(s)") {
		t.Fatalf("list: %s", out)
	}
	out = ta.must(t, "drafts", "show", id)
	if !strings.Contains(out, "exit check: ok") {
		t.Fatalf("show: %s", out)
	}

	// the human writes the reproduction steps
	body := filepath.Join(t.TempDir(), "body.md")
	cur, _ := ta.ls.GetDraft(context.Background(), id)
	_ = os.WriteFile(body, []byte(strings.Replace(cur.Body, "(承認する人が書く: `feedback-loop drafts edit`)", "1. 写真の部品のある画面を開く\n2. 追加を押す", 1)), 0o600)
	ta.must(t, "drafts", "edit", "-by", "ko-tarou", "-body-file", body, id)

	// dry run: no repo
	out = ta.must(t, "drafts", "approve", "-by", "ko-tarou", id)
	if !strings.Contains(out, "dry run") || !strings.Contains(out, "2. 追加を押す") {
		t.Fatalf("approve dry run: %s", out)
	}
	// configured: posted once, with no feedback text or personal data
	pub := &fakePublisher{}
	ta.publisher = pub
	ta.env[envIssueRepo] = "owner/repo"
	out = ta.must(t, "drafts", "publish", id)
	if !strings.Contains(out, "owner/repo#77") || len(pub.bodies) != 1 {
		t.Fatalf("publish: %s", out)
	}
	for _, leak := range []string{"090", "毎回アプリが落ちます", "fb_", "lin_"} {
		if strings.Contains(pub.titles[0]+pub.bodies[0], leak) {
			t.Fatalf("issue holds %q", leak)
		}
	}
	d, _ := ta.ls.GetDraft(context.Background(), id)
	if d.Status != loop.DraftPublished || d.IssueNumber != 77 {
		t.Fatalf("draft %+v", d)
	}
}

func TestDraftsArgs(t *testing.T) {
	ta := newTestApp(t, env{envIssueRepo: "not a repo"})
	if err := ta.run(context.Background(), []string{"drafts"}); err == nil {
		t.Fatal("no subcommand accepted")
	}
	if err := ta.run(context.Background(), []string{"drafts", "approve", "drf_x"}); err == nil {
		t.Fatal("bad repo accepted")
	}
	ta.env[envIssueRepo] = ""
	if err := ta.run(context.Background(), []string{"drafts", "approve", "-by", "x"}); err == nil {
		t.Fatal("missing id accepted")
	}
}
