package ai

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

func reply(text string) string {
	b, _ := json.Marshal(map[string]string{"text": text})
	return string(b)
}

// generate long keeps every paragraph and gets the long caps and frame.
func TestTextLongGenerate(t *testing.T) {
	letter := "おばあちゃんへ\n\n元気ですか。\n\nまた会いに行きます。"
	svc, fc := textSvc(reply(letter))
	rec := postText(t, NewHandler(svc), "u1", `{"instruction":"祖母へのお礼の手紙","length":"long"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	res := textResult(t, rec.Body.Bytes())
	if res.Text != letter || res.Length != "long" || res.Shape != "line" {
		t.Fatalf("result %+v", res)
	}
	c := fc.calls[0]
	if c.MaxTokens != TextMaxLongOutputToken || !strings.Contains(c.System, "長い文章") || !strings.Contains(c.System, "<data> はアプリの記録") {
		t.Fatalf("completion: max %d system %q", c.MaxTokens, c.System)
	}
	if strings.Contains(c.System, "1 項目は 1〜2 文まで") {
		t.Fatal("long must not use the short frame")
	}
}

// A long transform (summarize) takes up to 16 KB and caps the answer.
func TestTextLongTaskCaps(t *testing.T) {
	big := strings.Repeat("規約の文。", 1000) // 15000 bytes
	svc, fc := textSvc(reply(strings.Repeat("あ", TextMaxLongRunes+500)))
	h := NewHandler(svc)
	body, _ := json.Marshal(TextRequest{Task: "summarize", Input: big, Length: "long"})
	rec := postText(t, h, "u1", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	res := textResult(t, rec.Body.Bytes())
	if utf8.RuneCountInString(res.Text) != TextMaxLongRunes || res.Task != "summarize" || res.Length != "long" {
		t.Fatalf("result runes %d task %q", utf8.RuneCountInString(res.Text), res.Task)
	}
	if c := fc.calls[0]; c.MaxTokens != TextMaxLongOutputToken || !strings.Contains(c.System, longTaskRule) {
		t.Fatalf("completion: %d %q", c.MaxTokens, c.System)
	}
	// the same input is too large without long
	body, _ = json.Marshal(TextRequest{Task: "summarize", Input: big})
	if rec := postText(t, h, "u1", string(body)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("short 15 KB: %d", rec.Code)
	}
	body, _ = json.Marshal(TextRequest{Task: "summarize", Input: strings.Repeat("a", TextMaxLongInputBytes+1), Length: "long"})
	if rec := postText(t, h, "u1", string(body)); rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != httpx.CodePayloadTooLarge {
		t.Fatalf("long over 16 KB: %d", rec.Code)
	}
	if len(fc.calls) != 1 {
		t.Fatalf("calls %d", len(fc.calls))
	}
}

func TestTextLongBadRequests(t *testing.T) {
	svc, fc := textSvc()
	h := NewHandler(svc)
	for _, body := range []string{
		`{"instruction":"x","length":"huge"}`,
		`{"instruction":"x","length":"long","shape":"list","count":3}`,
		`{"instruction":"x","length":"long","shape":"fields","items":["a"]}`,
		`{"task":"classify","input":"x","options":["a","b"],"length":"long"}`,
	} {
		if rec := postText(t, h, "u1", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	if len(fc.calls) != 0 {
		t.Fatal("Claude must not be called")
	}
	// short is the default and may be sent explicitly
	svc, _ = textSvc(reply("一言"))
	rec := postText(t, NewHandler(svc), "u1", `{"instruction":"x","length":"short"}`)
	if rec.Code != http.StatusOK || textResult(t, rec.Body.Bytes()).Length != "" {
		t.Fatalf("short: %d %s", rec.Code, rec.Body.String())
	}
}

// Long is the same Pro feature (onlineAI) as every /v1/text call.
func TestTextLongProRequired(t *testing.T) {
	svc, fc := textSvc(reply("手紙"))
	svc.Pro, svc.ProMode = proSet{"pro-user": true}, paid.ModeEnforce
	h := NewHandler(svc)
	rec := postText(t, h, "free-user", `{"instruction":"手紙","length":"long"}`)
	if rec.Code != http.StatusPaymentRequired || errCode(t, rec) != httpx.CodeProRequired || len(fc.calls) != 0 {
		t.Fatalf("free: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postText(t, h, "pro-user", `{"instruction":"手紙","length":"long"}`); rec.Code != http.StatusOK {
		t.Fatalf("pro: %d %s", rec.Code, rec.Body.String())
	}
}

// A long call weighs TextLong (3): two of them do not fit in 5 units.
func TestTextLongRateLimitWeight(t *testing.T) {
	svc, fc := textSvc(reply("1"), reply("2"))
	lim := ratelimit.New(ratelimit.Config{Limits: ratelimit.Limits{SignedIn: 5}, Weights: ratelimit.DefaultWeights},
		ratelimit.NewMemory(), nil)
	h := lim.Middleware(auth.DevVerifier{})(NewHandler(svc))
	if rec := postText(t, h, "u1", `{"instruction":"x","length":"long"}`); rec.Code != http.StatusOK {
		t.Fatalf("first: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postText(t, h, "u1", `{"task":"summarize","input":"x","length":"long"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second (3+3 > 5): %d", rec.Code)
	}
	if rec := postText(t, h, "u1", `{"task":"summarize","input":"x"}`); rec.Code != http.StatusOK {
		t.Fatalf("short task (3+2): %d %s", rec.Code, rec.Body.String())
	}
	if len(fc.calls) != 2 {
		t.Fatalf("calls %d", len(fc.calls))
	}
}

func TestAILongRulesReachPrompt(t *testing.T) {
	for _, want := range []string{"## アプリの中の AI の長い文章(aiButton length long)", "length: long", "16KB"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
}
