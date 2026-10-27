package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

const (
	questionReply = `{"kind":"question","text":"レシートの写真も残しますか?","suggestions":["はい","いいえ","あとで"],
	  "brief":{"name":"家計簿","purpose":"毎日の出費を記録する","entities":["支出"],"features":["合計","一覧"],"native":[]},
	  "transcript":"家計簿アプリを作って。金額とカテゴリと日付を記録して、今月の合計と一覧を見たい。"}`
	readyReply = `{"kind":"ready","text":"では作りますね。","suggestions":[],
	  "brief":{"name":"家計簿","purpose":"毎日の出費を記録する","entities":["支出"],"features":["合計","一覧"],"native":["Camera"]},
	  "transcript":"家計簿アプリを作って。金額とカテゴリと日付を記録して、今月の合計と一覧を見たい。レシートの写真も残したい。"}`
)

func newChatSvc(replies ...string) (*Service, *fakeCaller) {
	f := &fakeCaller{replies: replies}
	svc, _ := newSvc(f)
	return svc, f
}

func user(texts ...string) []ChatMessage {
	ms := make([]ChatMessage, 0, len(texts))
	for i, t := range texts {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		ms = append(ms, ChatMessage{Role: role, Text: t})
	}
	return ms
}

func TestChat_QuestionThenReady(t *testing.T) {
	svc, f := newChatSvc(questionReply, readyReply)
	ctx := context.Background()

	r1, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("家計簿")})
	if err != nil {
		t.Fatal(err)
	}
	if r1.Kind != "question" || r1.Turn != 1 || r1.Locale != LocaleJA || r1.Source != "claude" {
		t.Fatalf("turn 1: %+v", r1)
	}
	if len(r1.Suggestions) != 3 || r1.Suggestions[0] != "はい" {
		t.Fatalf("suggestions: %v", r1.Suggestions)
	}
	if r1.Transcript != "" || r1.Brief != nil {
		t.Fatalf("question must not expose brief/transcript: %+v", r1)
	}
	if !f.calls[0].Structured || string(f.calls[0].Schema) != string(ChatSchema()) || f.calls[0].MaxTokens != chatMaxTokens {
		t.Fatalf("structured output not requested: %+v", f.calls[0])
	}
	if p := f.calls[0].Messages[0].Content; !strings.Contains(p, "ユーザー: 家計簿") || !strings.Contains(p, "ja(日本語)") {
		t.Fatalf("prompt: %s", p)
	}

	// Only the newest user message: the server appends it to the stored history.
	r2, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("はい")})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Kind != "ready" || r2.Turn != 2 || r2.Text != "では作りますね。" {
		t.Fatalf("turn 2: %+v", r2)
	}
	if r2.Brief == nil || r2.Brief.Native[0] != "camera" || !strings.Contains(r2.Transcript, "レシート") {
		t.Fatalf("brief/transcript: %+v %q", r2.Brief, r2.Transcript)
	}
	p := f.calls[1].Messages[0].Content
	if !strings.Contains(p, "あなた: レシートの写真も残しますか?") || !strings.Contains(p, "ユーザー: はい") {
		t.Fatalf("history not carried: %s", p)
	}
}

func TestChat_FullHistoryReplacesStored(t *testing.T) {
	svc, f := newChatSvc(questionReply, questionReply)
	ctx := context.Background()
	if _, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("家計簿")}); err != nil {
		t.Fatal(err)
	}
	// iOS resends the whole conversation (3 messages) -> turn 2, not 3.
	r, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("家計簿", "レシートの写真も残しますか?", "いいえ")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Turn != 2 {
		t.Fatalf("turn: %d", r.Turn)
	}
	if n := strings.Count(f.calls[1].Messages[0].Content, "ユーザー:"); n != 2 {
		t.Fatalf("user turns in prompt: %d", n)
	}
}

func TestChat_BuildNowShortcutSkipsClaude(t *testing.T) {
	svc, f := newChatSvc(questionReply)
	ctx := context.Background()
	if _, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("家計簿")}); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("もう作って")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "ready" || r.Source != "shortcut" || len(f.calls) != 1 {
		t.Fatalf("shortcut: %+v calls=%d", r, len(f.calls))
	}
	if !strings.HasPrefix(r.Transcript, "家計簿アプリを作って") || r.Brief == nil || r.Brief.Name != "家計簿" {
		t.Fatalf("should reuse the previous brief: %+v %q", r.Brief, r.Transcript)
	}
	if r.Text != "では作りますね。" || r.LatencyMs.Claude != 0 {
		t.Fatalf("text/latency: %+v", r)
	}
}

func TestChat_BuildNowOnFirstTurnForcesClaudeReady(t *testing.T) {
	// No brief yet: Claude is still called once, with the force flag, and a
	// question reply is coerced to ready.
	svc, f := newChatSvc(questionReply)
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Messages: user("お任せで家計簿")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "ready" || r.Source != "forced" || len(f.calls) != 1 || !strings.Contains(f.calls[0].Messages[0].Content, "## 強制 ready") {
		t.Fatalf("%+v calls=%d", r, len(f.calls))
	}
	if r.Brief == nil || r.Brief.Entities == nil || r.Brief.Native == nil || r.Transcript == "" {
		t.Fatalf("brief arrays must be non-nil: %+v", r.Brief)
	}
}

func TestChat_EnglishShortcut(t *testing.T) {
	svc, f := newChatSvc(`{"kind":"question","text":"What do you want to write?","suggestions":["Mood","Free text"],
	  "brief":{"name":"Diary","purpose":"Daily notes","entities":["Entry"],"features":["List"],"native":[]},"transcript":"Make a diary app with a daily entry and a list."}`)
	ctx := context.Background()
	if _, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("a diary")}); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("Just build it!")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "ready" || r.Source != "shortcut" || len(f.calls) != 1 || r.Locale != LocaleEN {
		t.Fatalf("%+v calls=%d", r, len(f.calls))
	}
	if r.Text != "Got it, I'll build it now." || !strings.HasPrefix(r.Transcript, "Make a diary app") {
		t.Fatalf("%+v", r)
	}
}

func TestChat_FourthTurnIsForcedReady(t *testing.T) {
	// Claude keeps asking; turn 4 must come back as ready anyway.
	svc, f := newChatSvc(questionReply, questionReply, questionReply, questionReply)
	ctx := context.Background()
	var r *ChatResult
	var err error
	for i, say := range []string{"メモ", "なんでも", "うーん", "はい"} {
		r, err = svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user(say)})
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 && r.Kind != "question" {
			t.Fatalf("turn %d: %+v", i+1, r)
		}
	}
	if r.Kind != "ready" || r.Turn != 4 || r.Source != "forced" || r.Transcript == "" || r.Brief == nil {
		t.Fatalf("turn 4: %+v", r)
	}
	if r.Text != "では作りますね。" {
		t.Fatalf("forced text: %q", r.Text)
	}
	if p := f.calls[3].Messages[0].Content; !strings.Contains(p, "## 強制 ready") {
		t.Fatalf("force flag missing: %s", p)
	}
	if p := f.calls[2].Messages[0].Content; strings.Contains(p, "## 強制 ready") {
		t.Fatalf("turn 3 must not be forced")
	}
}

func TestChat_TTLExpiresConversation(t *testing.T) {
	svc, f := newChatSvc(questionReply, questionReply)
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	svc.conv = newConvStore()
	svc.conv.now = func() time.Time { return now }
	ctx := context.Background()
	if _, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("家計簿")}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(29 * time.Minute)
	if svc.conv.get("a1") == nil {
		t.Fatal("conversation should still be alive at 29 min")
	}
	now = now.Add(2 * time.Minute)
	r, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("はい")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Turn != 1 {
		t.Fatalf("expired conversation should restart at turn 1: %+v", r)
	}
	if strings.Contains(f.calls[1].Messages[0].Content, "家計簿") {
		t.Fatalf("expired history leaked into the prompt")
	}
}

func TestChat_KeepsLastTenMessages(t *testing.T) {
	st := newConvStore()
	ms := make([]ChatMessage, 0, 14)
	for i := 0; i < 14; i++ {
		ms = append(ms, ChatMessage{Role: "user", Text: strings.Repeat("x", i+1)})
	}
	st.put("a1", &conversation{messages: ms})
	got := st.get("a1")
	if len(got.messages) != 10 || got.messages[0].Text != strings.Repeat("x", 5) {
		t.Fatalf("kept %d, first=%q", len(got.messages), got.messages[0].Text)
	}
}

func TestChat_LocaleEnglishInPrompt(t *testing.T) {
	svc, f := newChatSvc(`{"kind":"question","text":"What will you write each day?","suggestions":["Mood","Free text"],
	  "brief":{"name":"Diary","purpose":"","entities":[],"features":[],"native":[]},"transcript":"Make a diary app."}`)
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Messages: user("a diary")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Locale != LocaleEN || r.Kind != "question" || len(r.Suggestions) != 2 {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(f.calls[0].Messages[0].Content, "en (English)") {
		t.Fatalf("prompt locale: %s", f.calls[0].Messages[0].Content)
	}
	if _, err := svc.Chat(context.Background(), ChatRequest{AppID: "a2", Locale: "fr", Messages: user("x")}); err == nil || err.(*Error).Status != http.StatusBadRequest {
		t.Fatalf("unknown locale should be 400: %v", err)
	}
}

func TestChat_RetriesOnceOnBadJSON(t *testing.T) {
	svc, f := newChatSvc("not json", readyReply)
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Messages: user("家計簿。金額と一覧")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "ready" || r.Meta.Attempts != 2 || !strings.Contains(f.calls[1].Messages[0].Content, "検証に失敗") {
		t.Fatalf("%+v", r)
	}
	svc2, _ := newChatSvc("nope", "still nope")
	_, err = svc2.Chat(context.Background(), ChatRequest{AppID: "a1", Messages: user("家計簿")})
	if err == nil || err.(*Error).Code != "generation_failed" || err.(*Error).Attempts != 2 {
		t.Fatalf("expected generation_failed after 2 attempts: %v", err)
	}
}

func TestChat_BadRequests(t *testing.T) {
	svc, _ := newChatSvc()
	cases := []ChatRequest{
		{Messages: user("x")},
		{AppID: "a1"},
		{AppID: "a1", Messages: []ChatMessage{{Role: "system", Text: "x"}}},
		{AppID: "a1", Messages: []ChatMessage{{Role: "assistant", Text: "x"}}},
		{AppID: "a1", Messages: []ChatMessage{{Role: "user", Text: "  "}}},
	}
	for i, c := range cases {
		_, err := svc.Chat(context.Background(), c)
		if err == nil || err.(*Error).Status != http.StatusBadRequest {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestWantsBuildNow(t *testing.T) {
	yes := []string{"もう作って", "それでいいよ", "お任せします", "はい、作って！", "とりあえずこのまま作って", "Just build it", "OK, go ahead.", "that's fine"}
	no := []string{"家計簿アプリを作って", "筋トレの記録アプリ。腕立てとスクワット、朝8時に通知して", "はい", "make a diary app", "I want a list and a total"}
	for _, s := range yes {
		if !wantsBuildNow(s) {
			t.Errorf("should be go-ahead: %q", s)
		}
	}
	for _, s := range no {
		if wantsBuildNow(s) {
			t.Errorf("should not be go-ahead: %q", s)
		}
	}
}

func TestChatHandler_RoundTripAndDelete(t *testing.T) {
	svc, _ := newChatSvc(questionReply)
	svc.Verifier = auth.DevVerifier{}
	h := NewHandler(svc)
	body := `{"appId":"a1","messages":[{"role":"user","text":"家計簿"}]}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, bearer(httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(body)), "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["kind"] != "question" || out["turn"] != float64(1) || out["suggestions"].([]any)[0] != "はい" {
		t.Fatalf("body: %s", rec.Body.String())
	}
	if _, has := out["brief"]; has {
		t.Fatalf("question body must omit brief: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, bearer(httptest.NewRequest(http.MethodDelete, "/v1/chat/a1", nil), "u1"))
	if rec.Code != http.StatusNoContent || svc.conv.get(convKey("u1", "a1")) != nil {
		t.Fatalf("delete: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"appId":"a1"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad request: %d %s", rec.Code, rec.Body.String())
	}
}

func TestChatSchema_IsClosedAndRequired(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal(ChatSchema(), &s); err != nil {
		t.Fatal(err)
	}
	if s["additionalProperties"] != false || len(s["required"].([]any)) != 5 {
		t.Fatalf("root: %v", s)
	}
	brief := s["properties"].(map[string]any)["brief"].(map[string]any)
	if brief["additionalProperties"] != false || len(brief["required"].([]any)) != 5 {
		t.Fatalf("brief: %v", brief)
	}
}
