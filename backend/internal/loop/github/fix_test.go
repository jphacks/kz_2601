package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIssueFix(t *testing.T) {
	answers := map[int]string{
		1: `{"data":{"repository":{"issue":{"state":"CLOSED","stateReason":"COMPLETED","closedAt":"2026-11-30T00:00:00Z",
			"timelineItems":{"nodes":[{"closer":{"__typename":"PullRequest","number":55,"merged":true}}]},
			"closedByPullRequestsReferences":{"nodes":[]}}}}}`,
		2: `{"data":{"repository":{"issue":{"state":"CLOSED","stateReason":"COMPLETED","closedAt":"2026-11-30T00:00:00Z",
			"timelineItems":{"nodes":[{"closer":null}]},
			"closedByPullRequestsReferences":{"nodes":[{"number":60,"merged":false},{"number":61,"merged":true}]}}}}}`,
		3: `{"data":{"repository":{"issue":{"state":"CLOSED","stateReason":"NOT_PLANNED","closedAt":"2026-11-30T00:00:00Z",
			"timelineItems":{"nodes":[{"closer":{"__typename":"PullRequest","number":70,"merged":true}}]},
			"closedByPullRequestsReferences":{"nodes":[]}}}}}`,
		4: `{"data":{"repository":{"issue":{"state":"OPEN","stateReason":null,"closedAt":null,
			"timelineItems":{"nodes":[]},"closedByPullRequestsReferences":{"nodes":[{"number":80,"merged":true}]}}}}}`,
		5: `{"data":{"repository":{"issue":{"state":"CLOSED","stateReason":"COMPLETED","closedAt":"2026-11-30T00:00:00Z",
			"timelineItems":{"nodes":[{"closer":{"__typename":"PullRequest","number":90,"merged":false}}]},
			"closedByPullRequestsReferences":{"nodes":[]}}}}}`,
		6: `{"errors":[{"message":"nope"}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" || r.Method != http.MethodPost {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var in struct {
			Variables struct {
				Owner, Name string
				N           int
			}
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Variables.Owner != "owner" || in.Variables.Name != "repo" {
			t.Errorf("vars %+v", in.Variables)
		}
		_, _ = w.Write([]byte(answers[in.Variables.N]))
	}))
	defer srv.Close()
	c := &Client{Token: "tok", BaseURL: srv.URL}
	want := map[int]Fix{1: {Fixed: true, PR: 55}, 2: {Fixed: true, PR: 61}, 3: {}, 4: {}, 5: {}}
	for n, w := range want {
		got, err := c.IssueFix(context.Background(), "owner/repo", n)
		if err != nil || got.Fixed != w.Fixed || got.PR != w.PR {
			t.Errorf("#%d: %+v %v", n, got, err)
		}
	}
	if _, err := c.IssueFix(context.Background(), "owner/repo", 6); err == nil {
		t.Error("graphql error accepted")
	}
	if _, err := c.IssueFix(context.Background(), "bad", 1); err == nil {
		t.Error("bad repo accepted")
	}
}
