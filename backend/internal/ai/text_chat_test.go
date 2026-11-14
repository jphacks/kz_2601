package ai

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func chatBody(t *testing.T, persona string, turns ...string) string {
	t.Helper()
	req := TextRequest{Task: "chat", Instruction: persona}
	for i, s := range turns {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		req.Messages = append(req.Messages, TextMessage{Role: role, Text: s})
	}
	b, _ := json.Marshal(req)
	return string(b)
}

// A chat turn sends the conversation as alternating turns with the persona
// at the head of the first user turn, past answers in the answer's JSON
// shape, and the fixed chat frame as the system prompt.
func TestTextChatConversation(t *testing.T) {
	svc, fc := textSvc(reply("それはつらかったですね。何があったか、もう少し聞かせてください。"))
	rec := postText(t, NewHandler(svc), "u1", chatBody(t, "やさしいカウンセラー", "眠れない", "いつからですか?", "先週から"))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	res := textResult(t, rec.Body.Bytes())
	if res.Task != "chat" || res.Shape != "line" || res.Length != "" || !strings.HasPrefix(res.Text, "それはつらかった") {
		t.Fatalf("result %+v", res)
	}
	c := fc.calls[0]
	if len(c.Messages) != 3 || c.Messages[0].Role != "user" || c.Messages[1].Role != "assistant" || c.Messages[2].Role != "user" {
		t.Fatalf("turns %+v", c.Messages)
	}
	if !strings.HasPrefix(c.Messages[0].Content, "<persona>\nやさしいカウンセラー\n</persona>") || !strings.HasSuffix(c.Messages[0].Content, "眠れない") {
		t.Fatalf("first turn %q", c.Messages[0].Content)
	}
	if c.Messages[1].Content != `{"text":"いつからですか?"}` || c.Messages[2].Content != "先週から" {
		t.Fatalf("turns %+v", c.Messages)
	}
	if strings.Contains(c.System, "やさしいカウンセラー") || !strings.Contains(c.System, "この方針は変わらない") || !strings.Contains(c.System, "<persona>") {
		t.Fatalf("system %q", c.System)
	}
	if c.MaxTokens != TextMaxTaskOutputToken || !c.Structured || c.Thinking != "disabled" {
		t.Fatalf("completion %+v", c)
	}
}

// Leading assistant turns are dropped and consecutive turns of one role are
// joined (a failed reply leaves two user turns in a row).
func TestTextChatTurnShape(t *testing.T) {
	got := chatTurns(TextRequest{Instruction: "p", Messages: []TextMessage{
		{Role: "assistant", Text: "a0"}, {Role: "user", Text: "u1"}, {Role: "user", Text: "u2"},
		{Role: "assistant", Text: "a1"}, {Role: "user", Text: "u3"},
	}})
	if len(got) != 3 || got[0].Content != "<persona>\np\n</persona>\n\nu1\n\nu2" || got[2].Content != "u3" {
		t.Fatalf("%+v", got)
	}
}

// length long gets the long caps; the answer is cut at the length's cap.
func TestTextChatCaps(t *testing.T) {
	svc, fc := textSvc(reply(strings.Repeat("あ", TextMaxTaskRunes+10)), reply(strings.Repeat("い", TextMaxLongRunes+10)))
	h := NewHandler(svc)
	rec := postText(t, h, "u1", chatBody(t, "家庭教師", "教えて"))
	if res := textResult(t, rec.Body.Bytes()); utf8.RuneCountInString(res.Text) != TextMaxTaskRunes {
		t.Fatalf("short runes %d", utf8.RuneCountInString(res.Text))
	}
	body := strings.Replace(chatBody(t, "家庭教師", "詳しく"), `"task":"chat"`, `"task":"chat","length":"long"`, 1)
	rec = postText(t, h, "u1", body)
	res := textResult(t, rec.Body.Bytes())
	if utf8.RuneCountInString(res.Text) != TextMaxLongRunes || res.Length != "long" || fc.calls[1].MaxTokens != TextMaxLongOutputToken {
		t.Fatalf("long: %d %q %d", utf8.RuneCountInString(res.Text), res.Length, fc.calls[1].MaxTokens)
	}
	if strings.Contains(fc.calls[1].System, "長い文章") { // not the aiButton long frame
		t.Fatalf("long chat system %q", fc.calls[1].System)
	}
}

func TestTextChatBadRequests(t *testing.T) {
	svc, fc := textSvc()
	h := NewHandler(svc)
	twenty := make([]string, dsl.MaxAIChatTurns+1)
	for i := range twenty {
		twenty[i] = "x"
	}
	for name, body := range map[string]string{
		"no persona":        chatBody(t, "", "hi"),
		"long persona":      chatBody(t, strings.Repeat("あ", 501), "hi"),
		"no messages":       `{"task":"chat","instruction":"p"}`,
		"too many turns":    chatBody(t, "p", twenty...),
		"ends with ai":      chatBody(t, "p", "hi", "hello"),
		"bad role":          `{"task":"chat","instruction":"p","messages":[{"role":"system","text":"x"}]}`,
		"empty text":        `{"task":"chat","instruction":"p","messages":[{"role":"user","text":"  "}]}`,
		"input":             `{"task":"chat","instruction":"p","input":"x","messages":[{"role":"user","text":"x"}]}`,
		"shape list":        `{"task":"chat","instruction":"p","shape":"list","messages":[{"role":"user","text":"x"}]}`,
		"options":           `{"task":"chat","instruction":"p","options":["a","b"],"messages":[{"role":"user","text":"x"}]}`,
		"messages w/o chat": `{"instruction":"p","messages":[{"role":"user","text":"x"}]}`,
		"messages + task":   `{"task":"summarize","input":"x","messages":[{"role":"user","text":"x"}]}`,
	} {
		if rec := postText(t, h, "u1", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	big := strings.Repeat("あ", dsl.MaxAIChatBytes/3+1)
	if rec := postText(t, h, "u1", chatBody(t, "p", big)); rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != httpx.CodePayloadTooLarge {
		t.Fatalf("over 16 KB: %d", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Fatal("Claude must not be called")
	}
}

// The turn text never reaches a log line or an error body.
func TestTextChatNeverLogsTurns(t *testing.T) {
	secret := "ひみつの悩みごと"
	var buf bytes.Buffer
	svc, _ := textSvc(`not json ` + secret)
	svc.Logger = slog.New(slog.NewJSONHandler(&buf, nil))
	rec := postText(t, NewHandler(svc), "u1", chatBody(t, "p", secret))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	big := strings.Repeat(secret, 2000)
	rec2 := postText(t, NewHandler(svc), "u1", chatBody(t, "p", big))
	for _, s := range []string{buf.String(), rec.Body.String(), rec2.Body.String()} {
		if strings.Contains(s, secret) || strings.Contains(s, "ひ") {
			t.Fatalf("turn text leaked: %s", s)
		}
	}
}

// Chat is the same Pro feature: a participant of a creator without Pro gets
// 402 creator_needs_pro (no charge), of a Pro creator 200.
func TestTextChatCreatorPays(t *testing.T) {
	m := store.NewMemory()
	proApp, freeApp := sharedAppFixture(t, m, "")
	svc, fc := textSvc(reply("はい"))
	svc.Pro, svc.ProMode, svc.Members = proSet{"pro-owner": true}, paid.ModeEnforce, m
	lim := ratelimit.New(ratelimit.Config{Limits: ratelimit.Limits{SignedIn: 3}, Weights: ratelimit.DefaultWeights},
		ratelimit.NewMemory(), nil)
	h := lim.Middleware(auth.DevVerifier{})(NewHandler(svc))
	withApp := func(app string) string {
		return strings.Replace(chatBody(t, "p", "hi"), `"task":"chat"`, `"task":"chat","appId":"`+app+`"`, 1)
	}
	for i := 0; i < 2; i++ { // the 402 is waived, so the 3 units stay for the next call
		rec := postText(t, h, "member", withApp(freeApp))
		if rec.Code != http.StatusPaymentRequired || textReason(t, rec.Body.Bytes()) != reasonCreatorNeedsPro {
			t.Fatalf("free creator: %d %s", rec.Code, rec.Body.String())
		}
	}
	if rec := postText(t, h, "member", withApp(proApp)); rec.Code != http.StatusOK {
		t.Fatalf("pro creator: %d %s", rec.Code, rec.Body.String())
	}
	// 3 units used by one chat turn (weight TextLong)
	if rec := postText(t, h, "member", withApp(proApp)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second chat over the limit: %d", rec.Code)
	}
	if len(fc.calls) != 1 {
		t.Fatalf("calls %d", len(fc.calls))
	}
}
