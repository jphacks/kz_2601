package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/repos/owner/repo/issues" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("%s %s %s", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["title"] != "t" || in["body"] != "b" || in["labels"].([]any)[0] != "feedback" {
			t.Errorf("body %v", in)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":42,"html_url":"https://github.com/owner/repo/issues/42"}`))
	}))
	defer srv.Close()
	c := &Client{Token: "tok", BaseURL: srv.URL}
	iss, err := c.CreateIssue(context.Background(), "owner/repo", "t", "b", []string{"feedback"})
	if err != nil || iss.Number != 42 {
		t.Fatalf("%+v %v", iss, err)
	}
}

func TestCommentIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/repos/owner/repo/issues/42/comments" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("%s %s %s", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if len(in) != 1 || in["body"] != "b" {
			t.Errorf("body %v", in)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":9,"html_url":"https://github.com/owner/repo/issues/42#issuecomment-9"}`))
	}))
	defer srv.Close()
	c := &Client{Token: "tok", BaseURL: srv.URL}
	iss, err := c.CommentIssue(context.Background(), "owner/repo", 42, "b")
	if err != nil || iss.Number != 42 || iss.URL != "https://github.com/owner/repo/issues/42#issuecomment-9" {
		t.Fatalf("%+v %v", iss, err)
	}
	for _, bad := range []struct {
		repo string
		n    int
	}{{"../x/y", 1}, {"o/r", 0}, {"o/r", -1}} {
		if _, err := c.CommentIssue(context.Background(), bad.repo, bad.n, "b"); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestCreateIssueRefuses(t *testing.T) {
	c := &Client{Token: "tok", BaseURL: "http://127.0.0.1:1"}
	for _, repo := range []string{"", "owner", "owner/repo/x", "../x/y"} {
		if _, err := c.CreateIssue(context.Background(), repo, "t", "b", nil); err == nil {
			t.Errorf("repo %q accepted", repo)
		}
	}
	if _, err := (&Client{}).CreateIssue(context.Background(), "o/r", "t", "b", nil); err == nil {
		t.Error("no token accepted")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer srv.Close()
	if _, err := (&Client{Token: "t", BaseURL: srv.URL}).CreateIssue(context.Background(), "o/r", "t", "b", nil); err == nil {
		t.Error("403 accepted")
	}
}
