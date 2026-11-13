package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeIssues serves total open issues (numbers total..1, newest first) over
// the GraphQL issues connection, with a body on every node.
func fakeIssues(t *testing.T, total int, labels map[int][]string) (*httptest.Server, *atomic.Int32, *[]int) {
	t.Helper()
	var calls atomic.Int32
	firsts := []int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var in struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if strings.Contains(in.Query, "body") {
			t.Errorf("query asks for the body: %s", in.Query)
		}
		if in.Variables["owner"] != "o" || in.Variables["name"] != "r" {
			t.Errorf("vars %v", in.Variables)
		}
		first := int(in.Variables["first"].(float64))
		firsts = append(firsts, first)
		start := 0
		if a, ok := in.Variables["after"].(string); ok {
			_, _ = fmt.Sscanf(a, "c%d", &start)
		}
		nodes := []map[string]any{}
		for i := start; i < total && i < start+first; i++ {
			n := total - i
			ls := []map[string]string{}
			for _, l := range labels[n] {
				ls = append(ls, map[string]string{"name": l})
			}
			nodes = append(nodes, map[string]any{"number": n, "title": fmt.Sprintf("題名 %d", n), "url": fmt.Sprintf("https://github.com/o/r/issues/%d", n),
				"body": "SECRET BODY", "labels": map[string]any{"nodes": ls}})
		}
		end := start + len(nodes)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"repository": map[string]any{"issues": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": end < total, "endCursor": fmt.Sprintf("c%d", end)},
			"nodes":    nodes,
		}}}})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &firsts
}

func TestOpenIssuesPagesUpToLimit(t *testing.T) {
	srv, calls, firsts := fakeIssues(t, 1000, nil)
	c := &Client{Token: "tok", BaseURL: srv.URL}
	got, err := c.OpenIssues(context.Background(), "o/r", OpenIssuesOptions{Limit: 250})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 250 || got[0].Number != 1000 || got[249].Number != 751 {
		t.Fatalf("got %d issues, first %+v", len(got), got[0])
	}
	if fmt.Sprint(*firsts) != "[100 100 50]" {
		t.Errorf("pages %v", *firsts)
	}
	if calls.Load() != 3 {
		t.Errorf("calls %d", calls.Load())
	}
}

func TestOpenIssuesDefaultLimitAndLastPage(t *testing.T) {
	srv, _, _ := fakeIssues(t, 700, nil)
	got, err := (&Client{Token: "tok", BaseURL: srv.URL}).OpenIssues(context.Background(), "o/r", OpenIssuesOptions{})
	if err != nil || len(got) != DefaultOpenIssuesLimit {
		t.Fatalf("%d %v", len(got), err)
	}
	srv2, calls, _ := fakeIssues(t, 130, nil)
	got, err = (&Client{Token: "tok", BaseURL: srv2.URL}).OpenIssues(context.Background(), "o/r", OpenIssuesOptions{})
	if err != nil || len(got) != 130 || calls.Load() != 2 {
		t.Fatalf("%d %v calls %d", len(got), err, calls.Load())
	}
}

func TestOpenIssuesExcludesLoopIssues(t *testing.T) {
	srv, _, _ := fakeIssues(t, 5, map[int][]string{5: {"bug"}, 4: {"Feedback-Loop"}, 3: {"bug", "feedback-loop"}})
	c := &Client{Token: "tok", BaseURL: srv.URL}
	got, err := c.OpenIssues(context.Background(), "o/r", OpenIssuesOptions{ExcludeLabels: []string{" feedback-loop "}, ExcludeNumbers: []int{2}})
	if err != nil {
		t.Fatal(err)
	}
	want := []OpenIssue{
		{Number: 5, Title: "題名 5", URL: "https://github.com/o/r/issues/5", Labels: []string{"bug"}},
		{Number: 1, Title: "題名 1", URL: "https://github.com/o/r/issues/1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestOpenIssuesCachedPerClient(t *testing.T) {
	srv, calls, _ := fakeIssues(t, 3, nil)
	c := &Client{Token: "tok", BaseURL: srv.URL}
	for range 3 {
		got, err := c.OpenIssues(context.Background(), "o/r", OpenIssuesOptions{})
		if err != nil || len(got) != 3 {
			t.Fatal(len(got), err)
		}
		got[0].Title = "changed by the caller"
	}
	if calls.Load() != 1 {
		t.Errorf("calls %d, want 1 (cached)", calls.Load())
	}
	if got, _ := c.OpenIssues(context.Background(), "o/r", OpenIssuesOptions{}); got[0].Title != "題名 3" {
		t.Errorf("cache shared with the caller: %q", got[0].Title)
	}
	if _, _ = c.OpenIssues(context.Background(), "o/r", OpenIssuesOptions{ExcludeNumbers: []int{1}}); calls.Load() != 2 {
		t.Errorf("other options must read again: calls %d", calls.Load())
	}
}

// The body is neither asked for (fakeIssues checks the query) nor kept.
func TestOpenIssueHasNoBody(t *testing.T) {
	for i := range reflect.TypeFor[OpenIssue]().NumField() {
		if f := reflect.TypeFor[OpenIssue]().Field(i); strings.Contains(strings.ToLower(f.Name), "body") {
			t.Errorf("OpenIssue keeps %s", f.Name)
		}
	}
	srv, _, _ := fakeIssues(t, 2, nil)
	got, err := (&Client{Token: "tok", BaseURL: srv.URL}).OpenIssues(context.Background(), "o/r", OpenIssuesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(got); strings.Contains(string(b), "SECRET") {
		t.Errorf("body leaked: %s", b)
	}
}

func TestOpenIssuesErrors(t *testing.T) {
	c := &Client{Token: "tok", BaseURL: "http://127.0.0.1:1"}
	for _, repo := range []string{"", "o", "../x/y"} {
		if _, err := c.OpenIssues(context.Background(), repo, OpenIssuesOptions{}); err == nil {
			t.Errorf("repo %q accepted", repo)
		}
	}
	if _, err := (&Client{}).OpenIssues(context.Background(), "o/r", OpenIssuesOptions{}); err == nil {
		t.Error("no token accepted")
	}
	for _, body := range []string{`{"errors":[{"message":"x"}]}`, `{"data":{"repository":null}}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		if _, err := (&Client{Token: "t", BaseURL: srv.URL}).OpenIssues(context.Background(), "o/r", OpenIssuesOptions{}); err == nil {
			t.Errorf("%s accepted", body)
		}
		srv.Close()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer srv.Close()
	c = &Client{Token: "t", BaseURL: srv.URL}
	if _, err := c.OpenIssues(context.Background(), "o/r", OpenIssuesOptions{}); err == nil {
		t.Error("403 accepted")
	}
	if c.cache.byID != nil {
		t.Error("an error was cached")
	}
}

func TestClipTitle(t *testing.T) {
	long := strings.Repeat("あ", 300)
	if got := clip("  " + long); len([]rune(got)) != maxTitleRunes {
		t.Errorf("%d runes", len([]rune(got)))
	}
}
