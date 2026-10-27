package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// gatedStream streams the reply in small deltas, then blocks until release
// is closed before returning (the brief/transcript tail still "generating").
type gatedStream struct {
	reply   string
	release chan struct{}
	calls   []Completion
}

func (g *gatedStream) Complete(ctx context.Context, req Completion) (*Response, error) {
	g.calls = append(g.calls, req)
	rs := []rune(g.reply)
	for i := 0; i < len(rs); i += 7 {
		if req.OnText != nil {
			req.OnText(string(rs[i:min(i+7, len(rs))]))
		}
	}
	select {
	case <-g.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &Response{Text: g.reply, StopReason: "end_turn", Usage: Usage{InputTokens: 10, OutputTokens: 5}}, nil
}

func TestChat_EarlyQuestionThenBackgroundBrief(t *testing.T) {
	g := &gatedStream{reply: questionReply, release: make(chan struct{})}
	svc := &Service{Claude: g, Model: "test-model"}
	ctx := context.Background()

	got := make(chan *ChatResult, 1)
	go func() {
		r, err := svc.Chat(ctx, ChatRequest{AppID: "e1", Messages: user("家計簿")})
		if err != nil {
			t.Error(err)
		}
		got <- r
	}()
	var r1 *ChatResult
	select {
	case r1 = <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("question was not returned before the stream finished")
	}
	if r1.Kind != "question" || r1.Text != "レシートの写真も残しますか?" || len(r1.Suggestions) != 3 || r1.Meta.Attempts != 1 {
		t.Fatalf("early question: %+v", r1)
	}
	if c := g.calls[0]; c.Model != DefaultChatModel || c.Effort != DefaultChatEffort || c.OnText == nil {
		t.Fatalf("chat config not applied: model=%q effort=%q", c.Model, c.Effort)
	}

	// 「もう作って」 waits for the background brief, then answers without Claude.
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(g.release)
	}()
	r2, err := svc.Chat(ctx, ChatRequest{AppID: "e1", Messages: user("もう作って")})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Kind != "ready" || r2.Source != "shortcut" || r2.Brief == nil || r2.Brief.Name != "家計簿" || r2.Transcript == "" {
		t.Fatalf("shortcut after early question: %+v", r2)
	}
	if len(g.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(g.calls))
	}
}

func TestChat_EarlyReadyWaitsForFullReply(t *testing.T) {
	g := &gatedStream{reply: readyReply, release: make(chan struct{})}
	close(g.release)
	svc := &Service{Claude: g, Model: "test-model"}
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "e2", Messages: user("レシート写真つきの家計簿")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "ready" || r.Brief == nil || r.Transcript == "" {
		t.Fatalf("ready: %+v", r)
	}
}

func TestChat_ClearDuringBackgroundDropsUpdate(t *testing.T) {
	g := &gatedStream{reply: questionReply, release: make(chan struct{})}
	svc := &Service{Claude: g, Model: "test-model"}
	if _, err := svc.Chat(context.Background(), ChatRequest{AppID: "e3", Messages: user("家計簿")}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ClearChat("", "e3"); err != nil {
		t.Fatal(err)
	}
	close(g.release)
	svc.conversations().wait(context.Background(), "e3", time.Second)
	if cv := svc.conversations().get("e3"); cv != nil {
		t.Fatalf("cleared conversation came back: %+v", cv)
	}
}

func TestPartialQuestion(t *testing.T) {
	full := `{"kind":"question","text":"写真も残す?","suggestions":["はい","いいえ"],"brief":{"name":"日記"`
	cases := []struct {
		prefix string
		ok     bool
	}{
		{`{"kind":"question","text":"写真`, false},
		{`{"kind":"question","text":"写真も残す?","suggestions":["はい","い`, false},
		{`{"kind":"question","text":"写真も残す?","suggestions":["はい","いいえ"]`, true},
		{full, true},
		{`{"kind":"ready","text":"では作りますね。","suggestions":[]`, false},
		{`{"kind":"question","text":"","suggestions":[]`, false},
	}
	for _, c := range cases {
		q, ok := partialQuestion(c.prefix)
		if ok != c.ok {
			t.Errorf("%s: ok=%v", c.prefix, ok)
		}
		if ok && (q.Text != "写真も残す?" || len(q.Suggestions) != 2) {
			t.Errorf("%s: %+v", c.prefix, q)
		}
	}
}

func TestClient_PerCallModelThinkingAndHaikuFallbacks(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m","content":[{"type":"text","text":"{}"}],"stop_reason":"end_turn","usage":{}}`))
	}))
	defer srv.Close()
	c := NewClient("k", "claude-sonnet-5-5", "low")
	c.BaseURL = srv.URL
	ctx := context.Background()
	if _, err := c.Complete(ctx, Completion{Messages: []Message{{Role: "user", Content: "x"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Complete(ctx, Completion{Messages: []Message{{Role: "user", Content: "x"}},
		Model: "claude-haiku-5-5", Effort: "medium", Thinking: "disabled"}); err != nil {
		t.Fatal(err)
	}
	if b := bodies[0]; b["model"] != "claude-sonnet-5-5" || b["fallbacks"] != "default" || b["thinking"] != nil {
		t.Fatalf("defaults: %v", b)
	}
	b := bodies[1]
	oc, _ := b["output_config"].(map[string]any)
	th, _ := b["thinking"].(map[string]any)
	if b["model"] != "claude-haiku-5-5" || b["fallbacks"] != nil || oc["effort"] != "medium" || th["type"] != "disabled" {
		t.Fatalf("overrides: %v", b)
	}
}
