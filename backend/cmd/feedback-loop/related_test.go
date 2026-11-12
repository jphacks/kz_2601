package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop/github"
)

// fakeGitHubIssues answers the open-issues query with fixed nodes (status
// 200), or with status when it is not 200.
func fakeGitHubIssues(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Errorf("path %s", r.URL.Path)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		nodes := []map[string]any{
			{"number": 31, "title": "写真を追加するとアプリが落ちる\x1b[2J", "url": "u31", "labels": map[string]any{"nodes": []any{}}},
			{"number": 30, "title": "写真を追加するとアプリが終了する", "url": "u30", "labels": map[string]any{"nodes": []any{map[string]string{"name": "feedback"}}}},
			{"number": 29, "title": "通知が来ない", "url": "u29", "labels": map[string]any{"nodes": []any{}}},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"repository": map[string]any{"issues": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false}, "nodes": nodes}}}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func draftWithRelated(t *testing.T, ta *testApp) string {
	t.Helper()
	ta.model = &fakeModel{category: "bug", cluster: "photo-add-crash", summary: "写真を追加するとアプリが終了する。"}
	seedFeedback(t, ta, "写真の追加で落ちます")
	ta.must(t, "ingest")
	ta.must(t, "triage")
	return ta.must(t, "draft")
}

func TestDraftListsRelatedIssues(t *testing.T) {
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", envRelated: "on", envIssueRepo: "owner/repo", envIssueLabels: "feedback"})
	ta.issueReader = &github.Client{Token: "tok", BaseURL: fakeGitHubIssues(t, http.StatusOK).URL}
	out := draftWithRelated(t, ta)
	if !strings.Contains(out, "draft: new 1") || strings.Contains(out, "related issues") {
		t.Fatalf("draft: %s", out)
	}
	id := reDraftID.FindString(ta.must(t, "drafts", "list"))
	out = ta.must(t, "drafts", "show", id)
	if !strings.Contains(out, "関係しそうな Issue") || !strings.Contains(out, "owner/repo#31  写真を追加するとアプリが落ちる") {
		t.Fatalf("show: %s", out)
	}
	if strings.Contains(out, "#30") || strings.Contains(out, "#29") || strings.Contains(out, "\x1b") {
		t.Fatalf("show lists a loop issue, an unrelated one or a control character: %q", out)
	}
}

func TestDraftWhenIssuesCannotBeRead(t *testing.T) {
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", envRelated: "on", envIssueRepo: "owner/repo"})
	ta.issueReader = &github.Client{Token: "tok", BaseURL: fakeGitHubIssues(t, http.StatusForbidden).URL}
	out := draftWithRelated(t, ta)
	if !strings.Contains(out, "draft: new 1") || strings.Count(out, "draft: related issues not read: ") != 1 {
		t.Fatalf("draft: %s", out)
	}
	id := reDraftID.FindString(ta.must(t, "drafts", "list"))
	if out := ta.must(t, "drafts", "show", id); strings.Contains(out, "関係しそうな Issue") {
		t.Fatalf("show: %s", out)
	}
}

func TestRelatedIssuesOffByDefault(t *testing.T) {
	ta := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", envIssueRepo: "owner/repo"})
	ta.issueReader = &github.Client{Token: "tok", BaseURL: "http://127.0.0.1:1"} // must not be called
	if out := draftWithRelated(t, ta); strings.Contains(out, "related") {
		t.Fatalf("draft: %s", out)
	}
	ta2 := newTestApp(t, env{"LOOP_CONSENT_VERSIONS": "feedback-2027-01", envRelated: "on", envIssueRepo: "owner/repo"})
	if out := draftWithRelated(t, ta2); !strings.Contains(out, "related issues off ("+envRelated+"=on but "+envGitHubToken+" is not set)") {
		t.Fatalf("draft without a token: %s", out)
	}
}
