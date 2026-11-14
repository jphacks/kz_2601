package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

// syncBuf is a log buffer that an async ai.chat line may write to.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// chatLines collects ai.chat lines, waiting (briefly) until want are written.
func chatLines(t *testing.T, buf *syncBuf, want int) ([]map[string]any, string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		raw := buf.String()
		var lines []map[string]any
		for _, l := range strings.Split(strings.TrimSpace(raw), "\n") {
			if l == "" {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(l), &m); err != nil {
				t.Fatalf("not JSON: %q", l)
			}
			if m["event"] == ChatEvent {
				lines = append(lines, m)
			}
		}
		if len(lines) >= want || time.Now().After(deadline) {
			if len(lines) != want {
				t.Fatalf("want %d ai.chat lines, got %d: %s", want, len(lines), raw)
			}
			return lines, raw
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func withChatLog(svc *Service) *syncBuf {
	buf := &syncBuf{}
	svc.Logger = slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return buf
}

// forbidden: what the user said or the AI replied, the appId and the raw uid.
func assertNoChatLeak(t *testing.T, raw string, extra ...string) {
	t.Helper()
	leaks := append([]string{"SECRET-PROMPT", "app-RAW-1", rawUID, "家計簿", "レシート", "transcript", "brief", "\"text\""}, extra...)
	for _, leak := range leaks {
		if strings.Contains(raw, leak) {
			t.Errorf("ai.chat leaks %q: %s", leak, raw)
		}
	}
}

func TestChatLog_QuestionThenReady(t *testing.T) {
	svc, _ := newChatSvc(questionReply, readyReply)
	buf := withChatLog(svc)
	ctx := auth.ContextWithUID(context.Background(), rawUID)
	if _, err := svc.Chat(ctx, ChatRequest{AppID: "app-RAW-1", Messages: user(secretPrompt)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Chat(ctx, ChatRequest{AppID: "app-RAW-1", Messages: user("はい")}); err != nil {
		t.Fatal(err)
	}
	lines, raw := chatLines(t, buf, 2)
	model := svc.ChatConfig.resolved().Model
	common := map[string]any{
		"msg": ChatEvent, "level": "INFO", "mode": "clarify", "outcome": "replied", "source": "claude",
		"model": model, "uid_hash": UIDHash(rawUID), "app_hash": metrics.AppHash("app-RAW-1"),
	}
	byTurn := map[float64]map[string]any{}
	for _, m := range lines {
		for k, v := range common {
			if m[k] != v {
				t.Errorf("%s = %v, want %v", k, m[k], v)
			}
		}
		for _, k := range []string{"duration_ms", "claude_ms", "first_token_ms"} {
			if _, ok := m[k].(float64); !ok {
				t.Errorf("%s missing: %v", k, m)
			}
		}
		if _, ok := m["early_reply"].(bool); !ok {
			t.Errorf("early_reply missing: %v", m)
		}
		if _, ok := m["error_code"]; ok {
			t.Errorf("error_code on success: %v", m)
		}
		byTurn[m["turn"].(float64)] = m
	}
	if m := byTurn[1]; m["reply_kind"] != ChatKindQuestion || m["clarify_ended_in_build"] != false || m["suggestions"] != float64(3) {
		t.Errorf("turn 1: %v", m)
	}
	if m := byTurn[2]; m["reply_kind"] != ChatKindReady || m["clarify_ended_in_build"] != true {
		t.Errorf("turn 2: %v", m)
	}
	assertNoChatLeak(t, raw, "はい")
}

func TestChatLog_FirstTurnReadyIsNotAClarifyChain(t *testing.T) {
	svc, _ := newChatSvc(readyReply)
	buf := withChatLog(svc)
	if _, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Messages: user("お任せで家計簿を作って")}); err != nil {
		t.Fatal(err)
	}
	lines, _ := chatLines(t, buf, 1)
	if m := lines[0]; m["reply_kind"] != ChatKindReady || m["turn"] != float64(1) || m["clarify_ended_in_build"] != false {
		t.Fatalf("first-turn ready: %v", m)
	}
}

func TestChatLog_ChatFirstAnswer(t *testing.T) {
	svc, _ := newEditChatSvc(t, editAnswerReply)
	buf := withChatLog(svc)
	if _, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Messages: user("これ WebView?"),
		Capabilities: []string{"answer", "proposal"}}); err != nil {
		t.Fatal(err)
	}
	lines, raw := chatLines(t, buf, 1)
	if m := lines[0]; m["reply_kind"] != ChatKindAnswer || m["mode"] != "edit" || m["turn"] != float64(1) {
		t.Fatalf("answer: %v", m)
	}
	assertNoChatLeak(t, raw, "WebView", "ネイティブ")
}

func TestChatLog_BadRequest(t *testing.T) {
	svc, _ := newChatSvc()
	buf := withChatLog(svc)
	if _, err := svc.Chat(context.Background(), ChatRequest{AppID: "app-RAW-1"}); err == nil {
		t.Fatal("want error")
	}
	lines, raw := chatLines(t, buf, 1)
	m := lines[0]
	if m["outcome"] != "bad_request" || m["error_code"] != "bad_request" || m["level"] != "INFO" {
		t.Fatalf("bad request: %v", m)
	}
	for _, k := range []string{"reply_kind", "turn", "uid_hash"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s on a failed turn without uid: %v", k, m)
		}
	}
	assertNoChatLeak(t, raw)
}

// Tokens come from the request's tally; an early reply waits for its call.
func TestChatLog_TokensAfterEarlyReply(t *testing.T) {
	svc, _ := newChatSvc()
	buf := withChatLog(svc)
	ctx := withCostTally(context.Background())
	tally := tallyOf(ctx)
	done := tally.track() // the Claude call still streams after the reply
	res := &ChatResult{Kind: "question", Turn: 1, Source: "claude", Meta: Meta{Model: "claude-x", Attempts: 1}}
	svc.logChat(ctx, ChatRequest{AppID: "a1"}, res, nil, 5*time.Millisecond)
	if strings.Contains(buf.String(), ChatEvent) {
		t.Fatal("logged before the call finished")
	}
	tally.add(Usage{InputTokens: 900, OutputTokens: 120}, 0.01, true)
	done()
	lines, _ := chatLines(t, buf, 1)
	m := lines[0]
	if m["early_reply"] != true || m["input_tokens"] != float64(900) || m["output_tokens"] != float64(120) ||
		m["claude_calls"] != float64(1) || m["model"] != "claude-x" {
		t.Fatalf("tokens: %v", m)
	}
}

func TestChatReplyKind(t *testing.T) {
	for in, want := range map[string]string{"question": "question", "ready": "ready", "answer": "answer",
		"proposal": "proposal", "": "other", "refuse": "other"} {
		if got := chatReplyKind(in); got != want {
			t.Errorf("chatReplyKind(%q) = %q, want %q", in, got, want)
		}
	}
}
