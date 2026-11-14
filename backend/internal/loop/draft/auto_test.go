package draft

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
)

func TestAutoEligible(t *testing.T) {
	cases := []struct {
		category, priority, want string
	}{
		{"bug", "P3", ""}, {"feature", "P3", ""}, {"question", "P1", ""}, {"question", "P2", ""},
		{"question", "P3", AutoSkipQuestion}, {"spam", "P1", AutoSkipCategory}, {"praise", "P1", AutoSkipCategory}, {"", "P1", AutoSkipCategory},
	}
	for _, c := range cases {
		if got := AutoEligible(loop.Draft{Category: c.category, Priority: c.priority}); got != c.want {
			t.Errorf("%s %s: %q, want %q", c.category, c.priority, got, c.want)
		}
	}
}

func TestAutoLabels(t *testing.T) {
	got := strings.Join(AutoLabels([]string{"feedback", AutoLabel}, "P2"), ",")
	if got != "feedback,from-feedback,priority:P2" {
		t.Fatalf("labels %s", got)
	}
	if got := strings.Join(AutoLabels(nil, ""), ","); got != AutoLabel {
		t.Fatalf("labels %s", got)
	}
}

func TestAutoBody(t *testing.T) {
	s := loop.NewMemory()
	d := pendingDraft(t, s)
	d.Related = []loop.RelatedIssue{{Repo: "owner/repo", Number: 12, Title: "写真 https://evil.example"}}
	body := AutoBody(d)
	for _, want := range []string{"自動で**出した", "- #12", "自動で出したため未記入"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	for _, leak := range []string{"人が確かめて出した", stepsPlaceholder, "evil.example", "https"} {
		if strings.Contains(body, leak) {
			t.Errorf("body holds %q:\n%s", leak, body)
		}
	}
	if reasons := Check(d.Title, body, nil); len(reasons) > 0 {
		t.Fatalf("auto body fails the exit check: %v", reasons)
	}
	if again := AutoBody(loop.Draft{Body: body, Related: d.Related}); again != body {
		t.Fatalf("not idempotent:\n%s\n---\n%s", body, again)
	}
	// a body a person edited past the footer still gets the automatic line once
	edited := AutoBody(loop.Draft{Body: "## 症状\n\nx\n"})
	if strings.Count(edited, "自動で**出した") != 1 {
		t.Fatalf("edited:\n%s", edited)
	}
	app := AutoBody(loop.Draft{AppendIssue: 3, Body: "## 同じ束の新しい報告\n" + footerAppend})
	if !strings.Contains(app, "自動で**追記") || strings.Contains(app, "人が確かめて") {
		t.Fatalf("append:\n%s", app)
	}
}

func TestAutoFilerFilesCheckedDraftsWithinCaps(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	g, c := newFakeGitHub(t)
	r := &Reviewer{Store: s, Now: func() time.Time { return now }, Repo: "owner/repo", Publisher: c, Labels: []string{"feedback"}}
	triaged(t, s, 2, "photo", "bug", "写真を追加しようとするとアプリが毎回落ちてしまって困っています", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	triaged(t, s, 1, "export", "feature", "CSV で書き出せるようにしてほしいです", "CSV の書き出しが欲しい。", loop.SourceInApp)
	triaged(t, s, 1, "howto", "question", "使い方がわかりません", "使い方の説明が欲しい。", loop.SourceInApp)
	if _, err := (&Builder{Store: s, Now: func() time.Time { return now }}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	ds, _ := s.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPending})
	for i := range ds { // fixed priorities: the order of filing and the P3 question (never filed by itself)
		ds[i].Priority = map[string]string{"bug": "P1", "feature": "P2", "question": "P3"}[ds[i].Category]
		_ = s.PutDraft(ctx, ds[i])
	}
	var skipped, filed []string
	f := &AutoFiler{Reviewer: r, MaxPerRun: 1,
		OnFiled: func(_ context.Context, d *loop.Draft, _ *github.Issue) error { filed = append(filed, d.ID); return nil },
		OnSkipped: func(_ context.Context, d loop.Draft, reason string, _ error) error {
			skipped = append(skipped, reason)
			return nil
		}}

	dry := *f
	dry.DryRun = true
	res, err := dry.Run(ctx)
	if err != nil || len(res.Would) != 2 || len(g.created) != 0 {
		t.Fatalf("dry run: %+v %v created=%d", res, err, len(g.created))
	}

	// run 1: the cap of 1 files the bug (P1 before the feature's P2)
	skipped = nil
	res, err = f.Run(ctx)
	if err != nil || res.Filed != 1 || res.Left != 1 || res.Skipped != 1 || len(filed) != 1 {
		t.Fatalf("run 1: %+v %v filed=%v", res, err, filed)
	}
	if len(g.created) != 1 {
		t.Fatalf("created %d", len(g.created))
	}
	labels := g.created[0]["labels"].([]any)
	if len(labels) != 3 || labels[0] != "feedback" || labels[1] != AutoLabel || labels[2] != "priority:P1" {
		t.Fatalf("labels %v", labels)
	}
	body := g.created[0]["body"].(string)
	if !strings.Contains(body, "自動で**出した") || !strings.Contains(body, "- 件数: 2(アプリ内 2)") || strings.Contains(body, "毎回落ちてしまって") {
		t.Fatalf("body:\n%s", body)
	}
	got, _ := s.GetDraft(ctx, filed[0])
	if got.Status != loop.DraftPublished || got.ReviewedBy != loop.ActorLoop || got.IssueNumber != 41 || got.BodyHash != BodyHash(got.Title, got.Body) {
		t.Fatalf("draft %+v", got)
	}
	if strings.Join(skipped, ",") != AutoSkipQuestion {
		t.Fatalf("skipped %v", skipped)
	}

	// run 2: the day cap counts what the loop filed in the last 24h
	f.MaxPerRun, f.MaxPerDay = 10, 1
	res, err = f.Run(ctx)
	if err != nil || res.FiledToday != 1 || res.Filed != 0 || res.Left != 1 || len(g.created) != 1 {
		t.Fatalf("run 2: %+v %v", res, err)
	}

	// run 3: the feature goes out; the P3 question stays pending
	f.MaxPerDay = 10
	res, err = f.Run(ctx)
	if err != nil || res.Filed != 1 || len(g.created) != 2 {
		t.Fatalf("run 3: %+v %v", res, err)
	}
	if labels := g.created[1]["labels"].([]any); labels[2] != "priority:P2" {
		t.Fatalf("labels %v", labels)
	}
	left, _ := s.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPending})
	if len(left) != 1 || left[0].Category != "question" {
		t.Fatalf("left %+v", left)
	}

	// a later report of the filed cluster is appended as a comment, not a new issue
	triaged(t, s, 1, "photo", "bug", "写真を足すとまた落ちます", "写真を足すとアプリが終了する。", loop.SourceStoreReview)
	later := func() time.Time { return now.Add(time.Hour) }
	if _, err := (&Builder{Store: s, Now: later}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	r.Now = later
	res, err = f.Run(ctx)
	if err != nil || res.Appended != 1 || res.Filed != 0 || len(g.created) != 2 || len(g.comments[41]) != 1 {
		t.Fatalf("append: %+v %v comments=%v", res, err, g.comments)
	}
	if !strings.Contains(g.comments[41][0], "自動で**追記") {
		t.Fatalf("comment:\n%s", g.comments[41][0])
	}
}

func TestAutoApproveRefusesWithoutRepoAndRetriesFailedPost(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	d := pendingDraft(t, s)
	r := &Reviewer{Store: s, Now: func() time.Time { return now }}
	if _, _, err := r.AutoApprove(ctx, d.ID, nil); err == nil || !strings.Contains(err.Error(), "LOOP_ISSUE_REPO") {
		t.Fatalf("no repo accepted: %v", err)
	}
	pub := &fakePublisher{err: errors.New("github: status 502")}
	r.Repo, r.Publisher = "owner/repo", pub
	f := &AutoFiler{Reviewer: r}
	res, err := f.Run(ctx)
	if err != nil || res.Failed != 1 || res.Filed != 0 {
		t.Fatalf("failed post: %+v %v", res, err)
	}
	got, _ := s.GetDraft(ctx, d.ID)
	if got.Status != loop.DraftApproved || got.ReviewedBy != loop.ActorLoop {
		t.Fatalf("after failure %+v", got)
	}
	// the next run posts the loop's own approval; a person's stays for drafts publish
	pub.err = nil
	res, err = f.Run(ctx)
	if err != nil || res.Filed != 1 || len(pub.calls) != 1 || !strings.HasSuffix(pub.calls[0], "|"+AutoLabel+",priority:P2") {
		t.Fatalf("retry: %+v %v calls=%v", res, err, pub.calls)
	}
	human := pendingDraft(t, loop.NewMemory())
	human.Status, human.ReviewedBy = loop.DraftApproved, "ko-tarou"
	_ = s.PutDraft(ctx, human)
	res, err = f.Run(ctx)
	if err != nil || res.Filed != 0 || len(pub.calls) != 1 {
		t.Fatalf("human approval posted by the loop: %+v %v", res, err)
	}
}

func TestAutoApproveChecksTheAutomaticBody(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	d := pendingDraft(t, s)
	d.Body = "## 症状\n\n連絡先 090-1234-5678 に電話\n" + footerIssue
	_ = s.PutDraft(ctx, d)
	r := &Reviewer{Store: s, Now: func() time.Time { return now }, Repo: "owner/repo", Publisher: &fakePublisher{}}
	var reason string
	f := &AutoFiler{Reviewer: r, OnSkipped: func(_ context.Context, _ loop.Draft, rs string, err error) error {
		reason = rs
		if !errors.Is(err, ErrCheck) {
			t.Errorf("err %v", err)
		}
		return nil
	}}
	res, err := f.Run(ctx)
	if err != nil || res.Skipped != 1 || reason != AutoSkipCheck {
		t.Fatalf("%+v %v %s", res, err, reason)
	}
	if got, _ := s.GetDraft(ctx, d.ID); got.Status != loop.DraftPending {
		t.Fatalf("refused draft changed: %+v", got)
	}
}
