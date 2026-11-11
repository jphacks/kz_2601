package draft

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
)

var now = time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC)

func triaged(t *testing.T, s loop.Store, n int, cluster, cat, text, summary string, src loop.Source) []loop.Item {
	t.Helper()
	out := []loop.Item{}
	for i := range n {
		sid := fmt.Sprintf("%s-%s-%d", cluster, src, i)
		it := loop.Item{ID: loop.ItemID(src, sid), Source: src, SourceID: sid, UID: "u" + sid, Text: text, AIAllowed: true,
			Status: loop.ItemTriaged, Category: cat, Area: "render", Priority: "P2", Summary: summary, ClusterKey: cluster,
			AppVersion: "1.4.2", OSMajor: "iOS 26", IngestedAt: now, ExpiresAt: now.Add(loop.Retention)}
		if _, err := s.PutItem(context.Background(), it); err != nil {
			t.Fatal(err)
		}
		out = append(out, it)
	}
	return out
}

func TestBuildDrafts(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	triaged(t, s, 2, "photo-add-crash", "bug", "写真を追加しようとするとアプリが毎回落ちてしまって困っています", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	triaged(t, s, 1, "photo-add-crash", "bug", "クラッシュの急増: 種類 SIGABRT / 12 件 / 版 1.4.2", "写真の追加の画面で強制終了が増えている。", loop.SourceCrash)
	triaged(t, s, 1, "dark-mode", "feature", "暗い色にしたい", "暗い配色の切り替えの要望。", loop.SourceStoreReview)
	b := &Builder{Store: s, Now: func() time.Time { return now }}
	res, err := b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 2 || res.Items != 4 {
		t.Fatalf("%+v", res)
	}
	ds, _ := s.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPending})
	var photo loop.Draft
	for _, d := range ds {
		if d.ClusterKey == "photo-add-crash" {
			photo = d
		}
	}
	if photo.Count != 3 || len(photo.ItemIDs) != 3 || photo.Category != "bug" || len(photo.Sources) != 2 {
		t.Fatalf("photo draft %+v", photo)
	}
	if !strings.HasPrefix(photo.Title, "[フィードバック] 不具合: ") || !strings.Contains(photo.Body, "アプリ内 2 / クラッシュ 1") {
		t.Fatalf("title %q body\n%s", photo.Title, photo.Body)
	}
	for _, leak := range []string{"毎回落ちてしまって", "SIGABRT", "fb_", "lin_", "uphoto"} {
		if strings.Contains(photo.Title+photo.Body, leak) {
			t.Fatalf("draft holds %q:\n%s", leak, photo.Body)
		}
	}
	items, _ := s.ListItems(ctx, loop.ItemQuery{Status: loop.ItemDrafted})
	if len(items) != 4 {
		t.Fatalf("drafted items %d", len(items))
	}

	// a later report joins the open draft instead of opening a new one
	triaged(t, s, 1, "photo-add-crash", "bug", "写真で落ちた", "写真を追加すると終了する。", loop.SourceStoreReview)
	res, err = b.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 || res.Extended != 1 {
		t.Fatalf("second run %+v", res)
	}
	d, _ := s.GetDraft(ctx, photo.ID)
	if d.Count != 4 || !strings.Contains(d.Body, "ストアのレビュー 1") {
		t.Fatalf("extended %+v", d)
	}
}

func TestBuildHoldsQuotingDraftsAndCaps(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	text := "一覧の部品で並べ替えを押すと順番がばらばらになってしまう"
	items := triaged(t, s, 1, "0sort", "bug", text, text, loop.SourceInApp) // summary == feedback text
	triaged(t, s, 1, "a", "bug", "x", "別の不具合その一。", loop.SourceInApp)
	triaged(t, s, 1, "b", "bug", "y", "別の不具合その二。", loop.SourceInApp)
	triaged(t, s, 1, "praise", "praise", "z", "褒めている。", loop.SourceInApp)
	res, err := (&Builder{Store: s, Now: func() time.Time { return now }, MaxNew: 1}).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Held != 1 || res.Created != 1 || res.Capped != 1 {
		t.Fatalf("%+v", res)
	}
	it, _ := s.GetItem(ctx, items[0].ID)
	if it.Status != loop.ItemHeld {
		t.Fatalf("quoting item %+v", it)
	}
}

func TestCheck(t *testing.T) {
	src := []string{"写真を追加しようとするとアプリが毎回落ちてしまって困っています"}
	ok := Check("[フィードバック] 不具合: 写真の追加で終了する", "## 症状\n\n写真を追加すると終了する。\n- 件数: 3 #123", src)
	if len(ok) != 0 {
		t.Fatalf("clean draft rejected: %v", ok)
	}
	cases := map[string][2]string{
		"url":             {"t", "see https://example.com"},
		"mention":         {"t", "cc @maintainer"},
		"html":            {"t", "<details>x</details>"},
		"image":           {"t", "![x](y)"},
		"label_like":      {"t", "agent:ready"},
		"pii":             {"t", "連絡 090-1234-5678"},
		"invisible_chars": {"t", "a\u202eb"},
		"internal_id":     {"t", "fb_01HXXXXXXXX"},
		"quotes_feedback": {"t", "アプリが毎回落ちてしまって困っています"},
		"title_shape":     {"a\nb", "x"},
		"empty_title":     {" ", "x"},
		"too_long":        {"t", strings.Repeat("あ", MaxBodyRunes+1)},
	}
	for want, c := range cases {
		got := Check(c[0], c[1], src)
		found := false
		for _, g := range got {
			found = found || g == want
		}
		if !found {
			t.Errorf("%s: got %v", want, got)
		}
	}
}

type fakePublisher struct {
	calls []string
	err   error
}

func (f *fakePublisher) CreateIssue(_ context.Context, repo, title, body string, labels []string) (*github.Issue, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.calls = append(f.calls, repo+"|"+title+"|"+strings.Join(labels, ","))
	return &github.Issue{Number: 900 + len(f.calls), URL: "u"}, nil
}

func pendingDraft(t *testing.T, s loop.Store) loop.Draft {
	t.Helper()
	triaged(t, s, 2, "photo", "bug", "写真を追加しようとするとアプリが毎回落ちてしまって困っています", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	if _, err := (&Builder{Store: s, Now: func() time.Time { return now }}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	ds, _ := s.ListDrafts(context.Background(), loop.DraftQuery{Status: loop.DraftPending})
	if len(ds) != 1 {
		t.Fatalf("drafts %d", len(ds))
	}
	return ds[0]
}

func TestApproveDryRunThenPublish(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	d := pendingDraft(t, s)
	r := &Reviewer{Store: s, Now: func() time.Time { return now }}
	if _, _, err := r.Approve(ctx, d.ID, ""); err == nil {
		t.Fatal("approved without a name")
	}
	got, iss, err := r.Approve(ctx, d.ID, "ko-tarou")
	if err != nil || iss != nil || got.Status != loop.DraftApproved || got.BodyHash == "" || got.ReviewedBy != "ko-tarou" {
		t.Fatalf("dry run: %+v %v %v", got, iss, err)
	}
	if _, _, err := r.Publish(ctx, d.ID); err == nil {
		t.Fatal("published with no repo")
	}
	pub := &fakePublisher{}
	r.Repo, r.Publisher, r.Labels = "owner/repo", pub, []string{"feedback"}
	got, iss, err = r.Publish(ctx, d.ID)
	if err != nil || iss.Number != 901 || got.Status != loop.DraftPublished || got.IssueNumber != 901 || got.IssueRepo != "owner/repo" {
		t.Fatalf("publish: %+v %v %v", got, iss, err)
	}
	if len(pub.calls) != 1 || !strings.HasPrefix(pub.calls[0], "owner/repo|[フィードバック]") || !strings.HasSuffix(pub.calls[0], "|feedback") {
		t.Fatalf("calls %v", pub.calls)
	}
	if _, _, err := r.Approve(ctx, d.ID, "x"); err == nil {
		t.Fatal("approved a published draft")
	}
}

func TestApproveRefusesTamperingAndStop(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	d := pendingDraft(t, s)
	pub := &fakePublisher{}
	r := &Reviewer{Store: s, Now: func() time.Time { return now }}
	if _, _, err := r.Approve(ctx, d.ID, "ko-tarou"); err != nil {
		t.Fatal(err)
	}
	// someone changes the stored text after approval
	cur, _ := s.GetDraft(ctx, d.ID)
	cur.Body += "\n追記"
	_ = s.PutDraft(ctx, *cur)
	r.Repo, r.Publisher = "owner/repo", pub
	if _, _, err := r.Publish(ctx, d.ID); err == nil || !strings.Contains(err.Error(), "changed after approval") {
		t.Fatalf("tampered draft published: %v", err)
	}
	cur.Body = d.Body
	_ = s.PutDraft(ctx, *cur)
	r.BeforePublish = func(context.Context) error { return errors.New("publish is stopped") }
	if _, _, err := r.Publish(ctx, d.ID); err == nil || len(pub.calls) != 0 {
		t.Fatalf("stop switch ignored: %v", err)
	}
}

func TestEditAndReject(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	d := pendingDraft(t, s)
	r := &Reviewer{Store: s, Now: func() time.Time { return now }}
	if _, err := r.Edit(ctx, d.ID, "ko-tarou", "", d.Body+"\n連絡は a@example.com"); !errors.Is(err, ErrCheck) {
		t.Fatalf("PII edit accepted: %v", err)
	}
	if _, err := r.Edit(ctx, d.ID, "ko-tarou", "", d.Body+"\n写真を追加しようとするとアプリが毎回落ちてしまって"); !errors.Is(err, ErrCheck) {
		t.Fatalf("quoting edit accepted: %v", err)
	}
	got, err := r.Edit(ctx, d.ID, "ko-tarou", "写真の追加で終了する", strings.Replace(d.Body, "(承認する人が書く: `feedback-loop drafts edit`)", "1. 一覧の画面を開く\n2. 写真を追加する", 1))
	if err != nil || got.Title != "写真の追加で終了する" || !strings.Contains(got.Body, "2. 写真を追加する") {
		t.Fatalf("edit: %+v %v", got, err)
	}
	got, err = r.Reject(ctx, d.ID, "ko-tarou", "duplicate of #12")
	if err != nil || got.Status != loop.DraftRejected {
		t.Fatalf("reject %+v %v", got, err)
	}
	if _, _, err := r.Approve(ctx, d.ID, "ko-tarou"); err == nil {
		t.Fatal("approved a rejected draft")
	}
}
