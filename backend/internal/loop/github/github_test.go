package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestIssueOpen(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    bool
		wantErr string
	}{
		{"open", http.StatusOK, `{"state":"open"}`, true, ""},
		{"closed", http.StatusOK, `{"state":"closed","state_reason":"not_planned"}`, false, ""},
		{"unknown state", http.StatusOK, `{"state":"locked"}`, false, "unknown state"},
		{"no state", http.StatusOK, `{}`, false, "unknown state"},
		{"404", http.StatusNotFound, `{"message":"Not Found"}`, false, "status 404"},
		{"5xx", http.StatusBadGateway, `bad gateway`, false, "status 502"},
		{"broken json", http.StatusOK, `{"state":`, false, "EOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/repos/owner/repo/issues/42" || r.Header.Get("Authorization") != "Bearer tok" {
					t.Errorf("%s %s %s", r.Method, r.URL.Path, r.Header.Get("Authorization"))
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			got, err := (&Client{Token: "tok", BaseURL: srv.URL}).IssueOpen(context.Background(), "owner/repo", 42)
			if tt.wantErr == "" {
				if err != nil || got != tt.want {
					t.Fatalf("IssueOpen = %v, %v; want %v, nil", got, err, tt.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || got {
				t.Fatalf("IssueOpen = %v, %v; want false and an error with %q", got, err, tt.wantErr)
			}
		})
	}
}

func TestIssueOpenRefusesWithoutRequest(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"state":"open"}`))
	}))
	defer srv.Close()
	tests := []struct {
		name  string
		token string
		repo  string
		n     int
	}{
		{"path in repo", "tok", "../x/y", 1},
		{"no owner", "tok", "repo", 1},
		{"query in repo", "tok", "o/r?x=1", 1},
		{"zero", "tok", "o/r", 0},
		{"negative", "tok", "o/r", -1},
		{"no token", "", "o/r", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&Client{Token: tt.token, BaseURL: srv.URL}).IssueOpen(context.Background(), tt.repo, tt.n)
			if err == nil || got {
				t.Fatalf("IssueOpen(%q, %d) = %v, %v; want an error", tt.repo, tt.n, got, err)
			}
		})
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("%d requests sent for bad arguments", n)
	}
}
