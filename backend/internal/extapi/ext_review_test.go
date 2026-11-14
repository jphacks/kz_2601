package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// countRevoker counts provider revokes (only Revoke is called).
type countRevoker struct {
	OAuthClient
	calls *atomic.Int64
}

func (c countRevoker) Revoke(context.Context, Token) error {
	c.calls.Add(1)
	return nil
}

// Google Calendar, Tasks and Sheets share one OAuth client, so one grant:
// cutting one of them must not revoke the grant the others still use. The
// revoke endpoint is called only when the last of them is cut.
func TestRevokeSharedGoogleGrant(t *testing.T) {
	ctx := context.Background()
	sealer, err := NewLocalSealer(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &Connections{Store: NewMemory(), Sealer: sealer}
	var calls atomic.Int64
	cl := countRevoker{calls: &calls}
	for _, p := range []string{GoogleCalendar, GoogleTasks, GoogleSheets, Todoist} {
		if err := c.Save(ctx, user, p, Token{AccessToken: "a-" + p, RefreshToken: "r-" + p}); err != nil {
			t.Fatal(err)
		}
	}
	// a provider on its own client is revoked at once
	if err := c.Revoke(ctx, user, Todoist, cl); err != nil || calls.Load() != 1 {
		t.Fatalf("todoist: %v calls=%d", err, calls.Load())
	}
	if err := c.Revoke(ctx, user, GoogleTasks, cl); err != nil || calls.Load() != 1 {
		t.Fatalf("tasks cut while calendar / sheets connected: %v calls=%d", err, calls.Load())
	}
	if _, err := c.Token(ctx, user, GoogleTasks); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("tasks token kept: %v", err)
	}
	if _, err := c.Token(ctx, user, GoogleCalendar); err != nil {
		t.Fatalf("calendar broken by the tasks cut: %v", err)
	}
	if err := c.Revoke(ctx, user, GoogleSheets, cl); err != nil || calls.Load() != 1 {
		t.Fatalf("sheets cut while calendar connected: %v calls=%d", err, calls.Load())
	}
	if err := c.Revoke(ctx, user, GoogleCalendar, cl); err != nil || calls.Load() != 2 {
		t.Fatalf("the last Google cut revokes the grant: %v calls=%d", err, calls.Load())
	}

	// account deletion: the grant is revoked once
	calls.Store(0)
	for _, p := range []string{GoogleCalendar, GoogleTasks, GoogleSheets} {
		if err := c.Save(ctx, user, p, Token{AccessToken: "a-" + p, RefreshToken: "r-" + p}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := c.RevokeAll(ctx, user, func(string) OAuthClient { return cl })
	if err != nil || n != 3 || calls.Load() != 1 {
		t.Fatalf("revoke all: n=%d %v calls=%d", n, err, calls.Load())
	}
}

// todoist.addTask and notion.queryDatabase / addRow check their arguments
// before the call (appconfig counts nothing for a bad one).
func TestCheckArgsTodoistNotion(t *testing.T) {
	const db = "0123456789abcdef0123456789abcdef"
	for _, c := range []struct {
		provider, op string
		args         map[string]string
		ok           bool
	}{
		{Todoist, "addTask", map[string]string{"content": "牛乳"}, true},
		{Todoist, "addTask", map[string]string{"content": "牛乳", "dueDate": "2026-11-20", "priority": "4"}, true},
		{Todoist, "addTask", map[string]string{}, false},
		{Todoist, "addTask", map[string]string{"content": "x", "dueDate": "tomorrow"}, false},
		{Todoist, "addTask", map[string]string{"content": "x", "dueDate": "2026-11-20", "dueString": "明日"}, false},
		{Todoist, "addTask", map[string]string{"content": "x", "priority": "5"}, false},
		{Todoist, "addTask", map[string]string{"content": "x", "projectId": "a/b"}, false},
		{Notion, "queryDatabase", map[string]string{"databaseId": db}, true},
		{Notion, "queryDatabase", map[string]string{"databaseId": db, "limit": "10"}, true},
		{Notion, "queryDatabase", map[string]string{}, false},
		{Notion, "queryDatabase", map[string]string{"databaseId": "nope"}, false},
		{Notion, "queryDatabase", map[string]string{"databaseId": db, "limit": "0"}, false},
		{Notion, "addRow", map[string]string{"databaseId": db, "title": "買い物"}, true},
		{Notion, "addRow", map[string]string{"databaseId": db, "title": "買い物", "properties": `{"数": 1}`}, true},
		{Notion, "addRow", map[string]string{"databaseId": db}, false},
		{Notion, "addRow", map[string]string{"databaseId": "nope", "title": "x"}, false},
		{Notion, "addRow", map[string]string{"databaseId": db, "title": "x", "properties": "[1]"}, false},
	} {
		p, ok := Lookup(c.provider)
		if !ok {
			t.Fatalf("%s not registered", c.provider)
		}
		_, err := p.CheckArgs(c.op, c.args)
		if c.ok != (err == nil) || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Fatalf("%s.%s %v: %v", c.provider, c.op, c.args, err)
		}
	}
}

// A Google quota answered as 403 (YouTube quotaExceeded, ...) is the
// provider's 429: retryable and it pauses the provider. Another 403 stays.
func TestSendJSONGoogleQuota403(t *testing.T) {
	reason := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"m","errors":[{"reason":"` + reason + `","domain":"youtube.quota"}]}}`))
	}))
	defer srv.Close()
	get := func() error {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		var out map[string]any
		return sendJSON(context.Background(), srv.Client(), req, &out)
	}
	for _, r := range []string{"quotaExceeded", "dailyLimitExceeded", "rateLimitExceeded", "userRateLimitExceeded"} {
		reason = r
		err := get()
		var pe *ProviderError
		if !errors.As(err, &pe) || pe.Status != http.StatusTooManyRequests || !pe.Retryable || !upstreamLimited(err) || Reason(err) != ReasonUpstream {
			t.Fatalf("%s: %v", r, err)
		}
	}
	reason = "forbidden"
	err := get()
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Status != http.StatusForbidden || pe.Retryable || upstreamLimited(err) {
		t.Fatalf("forbidden: %v", err)
	}
}

// Asana: only a missing scope asks to connect again; no access to one
// project is bad_request.
func TestAsanaForbiddenNotScope(t *testing.T) {
	for body, want := range map[string]bool{
		`{"errors":[{"message":"The token is missing the required scope(s): tasks:write"}]}`: true,
		`{"errors":[{"message":"You do not have access to this project."}]}`:                 false,
		`not json`: false,
	} {
		if got := asanaScopeMissing([]byte(body)); got != want {
			t.Fatalf("%s: %v", body, got)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"message":"Forbidden"}]}`))
	}))
	defer srv.Close()
	c := &asana{wire: asanaWire{APIBase: srv.URL}, hc: srv.Client()}
	var out map[string]any
	err := c.do(context.Background(), Input{Token: &Token{AccessToken: "x"}}, http.MethodGet, "/projects/1/tasks", nil, &out)
	if errors.Is(err, ErrNotConnected) || Reason(err) != ReasonBadRequest || !strings.Contains(err.Error(), "access") {
		t.Fatalf("403 no access: %v", err)
	}
}
