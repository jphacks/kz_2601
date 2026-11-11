package draft

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
)

// fakeGitHub records what the real client sends.
type fakeGitHub struct {
	mu       sync.Mutex
	created  []map[string]any
	comments map[int][]string
}

func newFakeGitHub(t *testing.T) (*fakeGitHub, *github.Client) {
	t.Helper()
	g := &fakeGitHub{comments: map[int][]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		var n int
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/issues":
			g.created = append(g.created, in)
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"number":%d,"html_url":"https://github.com/owner/repo/issues/%d"}`, 40+len(g.created), 40+len(g.created))
		case r.Method == http.MethodPost && sscan(r.URL.Path, "/repos/owner/repo/issues/%d/comments", &n):
			g.comments[n] = append(g.comments[n], in["body"].(string))
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"id":%d,"html_url":"https://github.com/owner/repo/issues/%d#issuecomment-1"}`, 7000+len(g.comments[n]), n)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return g, &github.Client{Token: "tok", BaseURL: srv.URL}
}

func sscan(s, format string, n *int) bool {
	_, err := fmt.Sscanf(s, format, n)
	return err == nil && fmt.Sprintf(format, *n) == s
}

func approve(t *testing.T, r *Reviewer, s loop.Store, cluster string) (*loop.Draft, *github.Issue) {
	t.Helper()
	ctx := context.Background()
	if _, err := (&Builder{Store: s, Now: func() time.Time { return now }}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	ds, _ := s.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPending})
	for _, d := range ds {
		if d.ClusterKey == cluster {
			got, iss, err := r.Approve(ctx, d.ID, "ko-tarou")
			if err != nil {
				t.Fatal(err)
			}
			return got, iss
		}
	}
	t.Fatalf("no pending draft for %s", cluster)
	return nil, nil
}

func TestAppendToFiledIssue(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	g, c := newFakeGitHub(t)
	r := &Reviewer{Store: s, Now: func() time.Time { return now }, Repo: "owner/repo", Publisher: c, Labels: []string{"feedback"}}

	// no match: the first draft of a cluster files a new issue
	triaged(t, s, 2, "photo", "bug", "写真を追加しようとするとアプリが毎回落ちてしまって困っています", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	first, iss := approve(t, r, s, "photo")
	if iss == nil || iss.Number != 41 || len(g.created) != 1 || first.AppendIssue != 0 {
		t.Fatalf("create: %+v %+v %d", first, iss, len(g.created))
	}

	// match: later reports of the same cluster go to #41 as a comment
	triaged(t, s, 3, "photo2", "bug", "写真の追加で強制終了します", "写真を足すと終了する。", loop.SourceStoreReview)
	for _, it := range mustItems(t, s, loop.ItemTriaged) {
		it.ClusterKey = "photo"
		_ = s.UpdateItem(ctx, it)
	}
	second, iss := approve(t, r, s, "photo")
	if iss == nil || iss.Number != 41 || !strings.Contains(iss.URL, "#issuecomment-") {
		t.Fatalf("append: %+v", iss)
	}
	if len(g.created) != 1 || len(g.comments[41]) != 1 {
		t.Fatalf("issues %d, comments %v", len(g.created), g.comments)
	}
	if second.Status != loop.DraftPublished || second.IssueNumber != 41 || second.AppendIssue != 41 || second.AppendRepo != "owner/repo" {
		t.Fatalf("second draft %+v", second)
	}
	comment := g.comments[41][0]
	if comment != second.Body || !strings.Contains(comment, "- 件数: 3(ストアのレビュー 3)") || !strings.Contains(comment, "写真を足すと終了する。") {
		t.Fatalf("comment:\n%s", comment)
	}
	if strings.Contains(comment, "## 再現の手順") || strings.Contains(comment, "アプリ内") {
		t.Fatalf("comment repeats the issue or old counts:\n%s", comment)
	}

	// a fixed issue is not added to: the cluster comes back as a new issue
	first.FixedAt, first.FixedPR = now, 5
	second.FixedAt, second.FixedPR = now, 5
	_ = s.PutDraft(ctx, *first)
	_ = s.PutDraft(ctx, *second)
	triaged(t, s, 1, "photo", "bug", "また落ちる", "写真の追加でまた終了する。", loop.SourceCrash)
	third, iss := approve(t, r, s, "photo")
	if third.AppendIssue != 0 || iss.Number != 42 || len(g.created) != 2 {
		t.Fatalf("after fix: %+v %+v", third, iss)
	}
}

func mustItems(t *testing.T, s loop.Store, status string) []loop.Item {
	t.Helper()
	items, err := s.ListItems(context.Background(), loop.ItemQuery{Status: status})
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestAppendKeepsPIIOut(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	g, c := newFakeGitHub(t)
	r := &Reviewer{Store: s, Now: func() time.Time { return now }, Repo: "owner/repo", Publisher: c}
	triaged(t, s, 1, "photo", "bug", "写真で落ちる", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	approve(t, r, s, "photo")

	// the feedback text holds a phone number and a mail address (masked at
	// ingest in real runs); neither it nor the uid or item ids may reach
	// the comment
	secret := "写真を追加しようとすると毎回アプリが落ちます。連絡は 090-1234-5678 か a@example.com へ"
	items := triaged(t, s, 2, "photo", "bug", secret, "写真の追加の画面で終了する。", loop.SourceStoreReview)
	_, iss := approve(t, r, s, "photo")
	if iss == nil || iss.Number != 41 || len(g.comments[41]) != 1 {
		t.Fatalf("no comment: %+v %v", iss, g.comments)
	}
	comment := g.comments[41][0]
	leaks := []string{"090", "example.com", "毎回アプリが落ちます", "fb_", "lin_", "uphoto"}
	for _, it := range items {
		leaks = append(leaks, it.UID, it.ID, it.SourceID)
	}
	for _, leak := range leaks {
		if strings.Contains(comment, leak) {
			t.Fatalf("comment holds %q:\n%s", leak, comment)
		}
	}

	// a summary that carries personal data is held, never appended
	held := triaged(t, s, 1, "photo", "bug", "x", "090-1234-5678 の人の端末で終了する。", loop.SourceTestFlight)
	res, err := (&Builder{Store: s, Now: func() time.Time { return now }}).Run(ctx)
	if err != nil || res.Held != 1 || res.Created != 0 {
		t.Fatalf("held summary: %+v %v", res, err)
	}
	if got, _ := s.GetItem(ctx, held[0].ID); got.Status != loop.ItemHeld {
		t.Fatalf("pii item %+v", got)
	}
	if len(g.comments[41]) != 1 || len(g.created) != 1 {
		t.Fatalf("posted after hold: %v %d", g.comments, len(g.created))
	}
}

func TestAppendRefusesStaleTarget(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	g, c := newFakeGitHub(t)
	r := &Reviewer{Store: s, Now: func() time.Time { return now }, Publisher: c}
	pub := &Reviewer{Store: s, Now: func() time.Time { return now }, Repo: "owner/repo", Publisher: c}
	triaged(t, s, 1, "photo", "bug", "写真で落ちる", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	first, _ := approve(t, pub, s, "photo")

	// dry run: approval records the append draft; nothing is posted
	triaged(t, s, 1, "photo", "bug", "写真で落ちた", "写真の追加で終了する。", loop.SourceStoreReview)
	d, iss := approve(t, r, s, "photo")
	if iss != nil || d.Status != loop.DraftApproved || d.AppendIssue != first.IssueNumber || len(g.comments) != 0 {
		t.Fatalf("dry run: %+v %+v %v", d, iss, g.comments)
	}

	// another target repo: refused
	other := &Reviewer{Store: s, Now: func() time.Time { return now }, Repo: "owner/other", Publisher: c}
	if _, _, err := other.Publish(ctx, d.ID); err == nil {
		t.Fatal("appended to an issue of another repo")
	}
	// the issue was fixed meanwhile: refused (the approved text no longer fits)
	first.FixedAt = now
	_ = s.PutDraft(ctx, *first)
	if _, _, err := pub.Publish(ctx, d.ID); err == nil || !strings.Contains(err.Error(), "no longer") {
		t.Fatalf("appended to a fixed issue: %v", err)
	}
	if len(g.comments) != 0 || len(g.created) != 1 {
		t.Fatalf("posted: %v %d", g.comments, len(g.created))
	}
}
