package ai

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// aiButton length long on POST /v1/text: a longer answer (summaries of long
// input, letters, plans, scripts). Cloud only (the iOS app skips the
// on-device AI), the same Pro gate as every /v1/text call, rate-limit weight
// TextLong (3 by default).

// Caps of a long call.
const (
	TextMaxLongInputBytes  = 16 << 10
	TextMaxLongOutputToken = 1500
	TextMaxLongRunes       = 3000 // the text the result / output field gets
	textLongMinTimeout     = 60 * time.Second
)

// isLong reports whether the request asks for a long answer.
func (r TextRequest) isLong() bool { return r.Length == "long" }

// normalizeLength checks length ("" = short) and what long may combine with.
// Shape is checked after the shape default is applied by the caller.
func normalizeLength(req *TextRequest) error {
	if req.Length == "short" {
		req.Length = ""
	}
	if req.Length != "" && !dsl.AILengths[req.Length] {
		return badText("length must be short or long")
	}
	if req.isLong() && req.Task == "classify" {
		return badText("length long is not for task classify")
	}
	return nil
}

// checkTextInput applies the input cap of the request's length.
func checkTextInput(req *TextRequest) error {
	limit := TextMaxInputBytes
	if req.isLong() {
		limit = TextMaxLongInputBytes
	}
	if len(req.Input) > limit {
		return &Error{Status: http.StatusRequestEntityTooLarge, Code: httpx.CodePayloadTooLarge,
			Message: "input must be at most " + strconv.Itoa(limit) + " bytes"}
	}
	return nil
}

// textTimeout gives a long answer at least 60 s (1500 tokens take longer than
// the short default).
func (s *Service) textTimeout(req TextRequest) time.Duration {
	if req.isLong() {
		return max(s.chatTimeout(), textLongMinTimeout)
	}
	return s.chatTimeout()
}

// applyLong turns a built completion into the long variant: more output, a
// frame that allows paragraphs, and (generate) a plain {text} schema.
func applyLong(c *Completion, req TextRequest, loc Locale) {
	c.MaxTokens = TextMaxLongOutputToken
	if req.Task == "" {
		c.System = longSystemPrompt(loc)
		c.Messages[0].Content = longUserMessage(req)
		c.Schema = textSchema(req) // shape line: {text}
		return
	}
	c.System = taskSystemPrompt(loc) + "\n" + longTaskRule
}

const longTaskRule = `- 長い答えを求められている: 段落に分けてよい(合計 2500 字以内)。summarize は長い材料の要点を落とさずに。`

func longSystemPrompt(loc Locale) string {
	lang := "日本語で書く。"
	if loc == LocaleEN {
		lang = "Write in English."
	}
	return strings.TrimSpace(`
あなたはスマホアプリの中の「AI に長い文章を書かせるボタン」です。<instruction> の指示に従って、手紙・計画・台本・まとめなどのまとまった文章を、指定の JSON の text に 1 つだけ入れて返します。
- ` + lang + ` 前置き・説明・マークダウンの見出しや記号は書かない。段落は改行で分ける。合計 2500 字以内。` + textSafetyRules)
}

func longUserMessage(req TextRequest) string {
	var b strings.Builder
	b.WriteString("<instruction>\n" + req.Instruction + "\n</instruction>\n")
	b.WriteString("形: まとまった文章(text に全文。段落は改行で分ける)\n")
	if strings.TrimSpace(req.Input) != "" {
		b.WriteString("<data>\n" + req.Input + "\n</data>\n")
	}
	return b.String()
}

// parseLongOutput keeps the whole text (paragraphs included), trimmed and
// capped at TextMaxLongRunes.
func parseLongOutput(raw string, req TextRequest) (*TextResult, error) {
	bad := func(msg string) error {
		return &Error{Status: http.StatusBadGateway, Code: httpx.CodeBadOutput, Message: msg}
	}
	if i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); i >= 0 && j > i {
		raw = raw[i : j+1]
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, bad("model output is not JSON: " + err.Error())
	}
	text := strings.TrimSpace(strings.Trim(strings.TrimSpace(out.Text), "\"「」"))
	if text == "" {
		return nil, bad("empty text")
	}
	if r := []rune(text); len(r) > TextMaxLongRunes {
		text = string(r[:TextMaxLongRunes])
	}
	return &TextResult{Shape: "line", Task: req.Task, Text: text, Length: "long"}, nil
}
