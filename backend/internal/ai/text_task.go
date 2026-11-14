package ai

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// The transform tasks of aiButton (summarize / classify / rewrite / extract /
// suggest) on POST /v1/text: the online fallback when the device has no
// on-device AI. Same Pro gate, caps and safety frame as generate; the input
// (the button's field or recent records, ≤ 4 KB) is required and the answer
// is one text that the app writes into the output field.

// Caps of a transform task.
const (
	TextMaxTaskOutputToken = 1000
	TextMaxTaskRunes       = 1200 // the text the output field gets
	TextMaxOptions         = 20
	TextMaxOptionRunes     = 40
)

// textTasks are the transform tasks (generate is the default of /v1/text).
var textTasks = map[string]string{
	"summarize": "<data> の要点を短くまとめる(元の半分以下・箇条書きにしない)。",
	"classify":  "<data> に最も当てはまる候補を 1 つだけ選ぶ(text に候補の文字をそのまま入れる)。",
	"rewrite":   "<data> の意味を変えずに、読みやすく自然な文章に書き直す(誤字・言い回しを直す。内容を足さない)。",
	"extract":   "<data> から重要な点や値を短い箇条書き(行頭は「・」)で抜き出す。",
	"suggest":   "<data> を踏まえて、短い一言や提案を返す(2〜3 文まで)。",
}

// IsTextTask reports whether task is a transform task of /v1/text.
func IsTextTask(task string) bool { _, ok := textTasks[task]; return ok }

func normalizeTaskRequest(req *TextRequest) error {
	if !IsTextTask(req.Task) {
		return badText("task must be generate, summarize, classify, rewrite, extract, suggest or chat")
	}
	req.Instruction = strings.TrimSpace(req.Instruction)
	if utf8.RuneCountInString(req.Instruction) > dsl.MaxAIInstructionRunes {
		return badText("instruction must be at most 500 characters")
	}
	if req.Shape != "" && req.Shape != "line" {
		return badText("shape is only for task generate")
	}
	req.Shape = "line"
	if req.Count != 0 || len(req.Items) > 0 {
		return badText("count / items are only for task generate")
	}
	if req.Task == "classify" {
		if len(req.Options) < 2 || len(req.Options) > TextMaxOptions {
			return badText("classify needs 2..20 options")
		}
		seen := map[string]bool{}
		for i, o := range req.Options {
			o = strings.TrimSpace(o)
			if o == "" || utf8.RuneCountInString(o) > TextMaxOptionRunes || seen[o] {
				return badText("options must be unique non-empty strings of at most 40 characters")
			}
			seen[o] = true
			req.Options[i] = o
		}
	} else if len(req.Options) > 0 {
		return badText("options are only for task classify")
	}
	if err := checkTextInput(req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Input) == "" && req.Image == nil { // a photo alone is material too (text_image.go)
		return badText("input is required for task " + req.Task)
	}
	return nil
}

// taskSystemPrompt is the fixed frame of a transform task (longer answers
// than generate; same safety rules).
func taskSystemPrompt(loc Locale) string {
	lang := "<data> と同じ言語で書く(迷ったら日本語)。"
	if loc == LocaleEN {
		lang = "Write in English."
	}
	return strings.TrimSpace(`
あなたはスマホアプリの中の「AI で処理するボタン」です。<task> の処理を <data> に行い、結果だけを指定の JSON の形で返します。
- ` + lang + ` 前置き・説明・マークダウンの見出しは書かない。` + textSafetyRules)
}

func taskUserMessage(req TextRequest) string {
	var b strings.Builder
	b.WriteString("<task>\n" + textTasks[req.Task] + "\n</task>\n")
	if req.Instruction != "" {
		b.WriteString("<instruction>\n" + req.Instruction + "\n</instruction>\n")
	}
	if req.Task == "classify" {
		b.WriteString("候補: " + strings.Join(req.Options, " / ") + "\n")
	}
	b.WriteString("<data>\n" + req.Input + "\n</data>\n")
	return b.String()
}

// taskSchema is {text}; classify narrows text to the candidates.
func taskSchema(req TextRequest) json.RawMessage {
	text := map[string]any{"type": "string"}
	if req.Task == "classify" {
		text["enum"] = req.Options
	}
	raw, _ := json.Marshal(map[string]any{
		"type": "object", "properties": map[string]any{"text": text}, "required": []string{"text"}, "additionalProperties": false,
	})
	return raw
}

// parseTaskOutput keeps the whole text (trimmed, capped); classify must name
// one of the candidates (an answer that contains exactly one is accepted).
func parseTaskOutput(raw string, req TextRequest) (*TextResult, error) {
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
	if req.Task == "classify" {
		if !slices.Contains(req.Options, text) {
			var hit []string
			for _, o := range req.Options {
				if strings.Contains(text, o) {
					hit = append(hit, o)
				}
			}
			if len(hit) != 1 {
				return nil, bad("answer is not one of the options")
			}
			text = hit[0]
		}
	} else if r := []rune(text); len(r) > TextMaxTaskRunes {
		text = string(r[:TextMaxTaskRunes])
	}
	return &TextResult{Shape: "line", Task: req.Task, Text: text}, nil
}
