package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// stepCaller answers call i with steps[i] (past the end: 500).
type stepCaller struct {
	mu    sync.Mutex
	steps []func(ctx context.Context) (*Response, error)
	calls []Completion
}

func (c *stepCaller) Complete(ctx context.Context, req Completion) (*Response, error) {
	c.mu.Lock()
	i := len(c.calls)
	c.calls = append(c.calls, req)
	c.mu.Unlock()
	if i >= len(c.steps) {
		return nil, &APIError{Status: 500, Type: "no_more_replies"}
	}
	return c.steps[i](ctx)
}

func (c *stepCaller) prompts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.calls))
	for i, r := range c.calls {
		out[i] = r.Messages[0].Content
	}
	return out
}

func stepReply(text string) func(context.Context) (*Response, error) {
	return func(context.Context) (*Response, error) {
		return &Response{Text: text, StopReason: "end_turn"}, nil
	}
}

func failWith(err error) func(context.Context) (*Response, error) {
	return func(context.Context) (*Response, error) { return nil, err }
}

// newEditStepSvc seeds a1 with kakeibo so the turn is an edit.
func newEditStepSvc(t *testing.T, steps ...func(context.Context) (*Response, error)) (*Service, *stepCaller) {
	t.Helper()
	c := &stepCaller{steps: steps}
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "a1", 0, kakeibo(t), appconfigclient.Source{Kind: "seed"}); err != nil {
		t.Fatal(err)
	}
	return &Service{Claude: c, Store: store, Model: "test-model", TextImage: true}, c
}

func editTurn(ctx context.Context, svc *Service, text string) (*ChatResult, error) {
	return svc.Chat(ctx, ChatRequest{AppID: "a1", Mode: "edit", Messages: user(text)})
}

func wantAIError(t *testing.T, err error, status int, code string) *Error {
	t.Helper()
	var ge *Error
	if !errors.As(err, &ge) || ge.Status != status || ge.Code != code {
		t.Fatalf("err = %#v, want %d %s", err, status, code)
	}
	return ge
}

const goodEditAnswer = `{"kind":"answer","text":"できます。","suggestions":[],"options":[],"transcript":""}`

// A model error is returned as is (no second attempt).
func TestChatFirst_ModelErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"unavailable", &APIError{Status: 503, Type: "overloaded_error"}, http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable},
		{"rate limited", &APIError{Status: 429, Type: "rate_limit_error"}, http.StatusTooManyRequests, httpx.CodeRateLimited},
		{"rejected", &APIError{Status: 400, Type: "invalid_request_error", Message: "bad"}, http.StatusInternalServerError, httpx.CodeInternal},
		{"transport", errors.New("connection reset"), http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, f := newEditStepSvc(t, failWith(c.err), stepReply(goodEditAnswer))
			_, err := editTurn(context.Background(), svc, "メモ欄を消して")
			_ = wantAIError(t, err, c.status, c.code)
			if n := len(f.prompts()); n != 1 {
				t.Fatalf("calls = %d, want 1 (no retry on a model error)", n)
			}
		})
	}
}

func TestChatFirst_Refusal(t *testing.T) {
	svc, _ := newEditStepSvc(t, func(context.Context) (*Response, error) {
		return &Response{Text: "", StopReason: "refusal"}, nil
	})
	ge := wantAIError(t, failTurn(t, context.Background(), svc, "x"), http.StatusBadRequest, httpx.CodeRefused)
	if ge.Attempts != 1 {
		t.Fatalf("attempts = %d", ge.Attempts)
	}
}

// The model call has its own deadline (CallTimeout): 504 upstream_timeout.
func TestChatFirst_ModelTimeout(t *testing.T) {
	svc, _ := newEditStepSvc(t, func(ctx context.Context) (*Response, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	svc.CallTimeout = 20 * time.Millisecond
	_ = wantAIError(t, failTurn(t, context.Background(), svc, "x"), http.StatusGatewayTimeout, httpx.CodeUpstreamTimeout)
}

// The request going away while the model still runs: 504 timeout.
func TestChatFirst_RequestCancelled(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	svc, _ := newEditStepSvc(t, func(ctx context.Context) (*Response, error) {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return &Response{Text: goodEditAnswer, StopReason: "end_turn"}, nil
	})
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := editTurn(ctx, svc, "x")
		errc <- err
	}()
	<-started
	cancel()
	ge := wantAIError(t, <-errc, http.StatusGatewayTimeout, httpx.CodeTimeout)
	if ge.Message != "chat turn cancelled" {
		t.Fatalf("message = %q", ge.Message)
	}
}

// A malformed reply is retried once with the problem in the prompt.
func TestChatFirst_RetryFixesBadReply(t *testing.T) {
	cases := []struct {
		name, first, problem string
	}{
		{"not JSON", `ごめんなさい`, "output is not valid JSON"},
		{"unknown kind", `{"kind":"maybe","text":"x","suggestions":[],"options":[],"transcript":""}`, `kind must be "answer"`},
		{"wrong type", `{"kind":"answer","text":1}`, "output is not valid JSON"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, f := newEditStepSvc(t, stepReply(c.first), stepReply(goodEditAnswer))
			r, err := editTurn(context.Background(), svc, "これは何?")
			if err != nil {
				t.Fatal(err)
			}
			if r.Kind != "answer" || r.Text != "できます。" || r.Meta.Attempts != 2 || r.Meta.Regenerations != 1 || r.Source != "claude" {
				t.Fatalf("result: %+v", r)
			}
			p := f.prompts()
			if len(p) != 2 || strings.Contains(p[0], c.problem) || !strings.Contains(p[1], c.problem) {
				t.Fatalf("prompts:\n%s", strings.Join(p, "\n---\n"))
			}
		})
	}
}

// Two bad replies: 502 generation_failed with the second attempt's problems.
func TestChatFirst_RetryStillBad(t *testing.T) {
	svc, _ := newEditStepSvc(t, stepReply(`nope`), stepReply(`{"kind":"later","text":"x"}`))
	ge := wantAIError(t, failTurn(t, context.Background(), svc, "x"), http.StatusBadGateway, httpx.CodeGenerationFailed)
	if ge.Attempts != 2 || len(ge.Problems) != 1 || !strings.Contains(ge.Problems[0], "kind must be") {
		t.Fatalf("error: %+v", ge)
	}
}

// The retry itself failing returns that error.
func TestChatFirst_RetryModelError(t *testing.T) {
	svc, _ := newEditStepSvc(t, stepReply(`nope`), failWith(&APIError{Status: 529, Type: "overloaded_error"}))
	_ = wantAIError(t, failTurn(t, context.Background(), svc, "x"), http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable)
}

// A failed turn keeps nothing: the next turn starts from the user's message.
func TestChatFirst_FailureLeavesNoHistory(t *testing.T) {
	svc, f := newEditStepSvc(t, failWith(&APIError{Status: 503}), stepReply(goodEditAnswer))
	ctx := context.Background()
	_ = failTurn(t, ctx, svc, "一つ目")
	if _, err := editTurn(ctx, svc, "二つ目"); err != nil {
		t.Fatal(err)
	}
	if p := f.prompts()[1]; strings.Contains(p, "一つ目") || !strings.Contains(p, "ユーザー: 二つ目") {
		t.Fatalf("second prompt:\n%s", p)
	}
}

func failTurn(t *testing.T, ctx context.Context, svc *Service, text string) error {
	t.Helper()
	r, err := editTurn(ctx, svc, text)
	if err == nil {
		t.Fatalf("want an error, got %+v", r)
	}
	return err
}

// getStore fails or returns a fixed envelope on Get.
type getStore struct {
	appconfigclient.Store
	env *appconfigclient.Envelope
	err error
}

func (g getStore) Get(context.Context, string) (*appconfigclient.Envelope, error) {
	return g.env, g.err
}

func TestChatMode(t *testing.T) {
	kb := appconfigclient.NewMemory()
	if _, err := kb.Put(context.Background(), "a1", 0, kakeibo(t), appconfigclient.Source{Kind: "seed"}); err != nil {
		t.Fatal(err)
	}
	storeErr := &appconfigclient.Error{Status: http.StatusNotFound, Code: "not_found", Message: "no app"}
	cases := []struct {
		name     string
		store    appconfigclient.Store
		mode     string
		want     string
		wantApp  bool
		errState int
		errCode  string
	}{
		{"unknown mode", kb, "rewrite", "", false, http.StatusBadRequest, httpx.CodeBadRequest},
		{"unknown mode before the store", getStore{err: storeErr}, "delete", "", false, http.StatusBadRequest, httpx.CodeBadRequest},
		{"create skips the store", getStore{err: storeErr}, "create", chatModeCreate, false, 0, ""},
		{"no store is create", nil, "edit", chatModeCreate, false, 0, ""},
		{"edit, case and spaces", kb, " EDIT ", chatModeEdit, true, 0, ""},
		{"auto picks edit", kb, "auto", chatModeEdit, true, 0, ""},
		{"empty picks edit", kb, "", chatModeEdit, true, 0, ""},
		{"auto on a store error is create", getStore{err: errors.New("down")}, "auto", chatModeCreate, false, 0, ""},
		{"edit on a store error", getStore{err: storeErr}, "edit", "", false, http.StatusNotFound, "not_found"},
		{"edit on a plain store error", getStore{err: errors.New("down")}, "edit", "", false, http.StatusBadGateway, httpx.CodeAppconfigError},
		{"broken DSL", getStore{env: &appconfigclient.Envelope{AppID: "a1", DSL: json.RawMessage(`{"schemaVersion":1}`)}}, "edit", "", false, http.StatusInternalServerError, httpx.CodeInternal},
		{"initial app is create", appconfigclient.NewMemory(), "edit", chatModeCreate, false, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := &Service{Store: c.store}
			mode, app, err := svc.chatMode(context.Background(), ChatRequest{AppID: "a1", Mode: c.mode})
			if c.errState != 0 {
				_ = wantAIError(t, err, c.errState, c.errCode)
				return
			}
			if err != nil || mode != c.want || (app != nil) != c.wantApp {
				t.Fatalf("chatMode = %q, app=%v, %v; want %q, app=%v", mode, app != nil, err, c.want, c.wantApp)
			}
		})
	}
}
