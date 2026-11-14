package ai

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

func textResult(t *testing.T, body []byte) TextResult {
	t.Helper()
	var res TextResult
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestTextTaskAnswers(t *testing.T) {
	long := strings.Repeat("あ", TextMaxTaskRunes+50)
	tests := []struct {
		name, body, reply, want string
	}{
		{"summarize keeps several lines", `{"task":"summarize","input":"今日は朝から雨で…"}`,
			`{"text":"雨の一日。\n午後は読書をした。"}`, "雨の一日。\n午後は読書をした。"},
		{"rewrite is capped", `{"task":"rewrite","input":"x"}`, `{"text":"` + long + `"}`, strings.Repeat("あ", TextMaxTaskRunes)},
		{"classify exact", `{"task":"classify","options":["仕事","趣味"],"input":"会議の議事録"}`, `{"text":"仕事"}`, "仕事"},
		{"classify inside a sentence", `{"task":"classify","options":["仕事","趣味"],"input":"釣り"}`, `{"text":"趣味です"}`, "趣味"},
		{"suggest with instruction", `{"task":"suggest","instruction":"おばあちゃん口調で","input":"- 予定: 夜の山登り"}`, `{"text":"夜の山は危ないよ"}`, "夜の山は危ないよ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, fc := textSvc(tt.reply)
			rec := postText(t, NewHandler(svc), "u1", tt.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			res := textResult(t, rec.Body.Bytes())
			if res.Text != tt.want || res.Shape != "line" || res.Task == "" {
				t.Fatalf("result %+v", res)
			}
			c := fc.calls[0]
			if c.MaxTokens != TextMaxTaskOutputToken || !c.Structured || c.Thinking != "disabled" || !strings.Contains(c.System, "安全") {
				t.Fatalf("completion: max=%d structured=%v", c.MaxTokens, c.Structured)
			}
			user := c.Messages[0].Content
			if !strings.Contains(user, "<task>") || !strings.Contains(user, "<data>") || strings.Contains(c.System, "<data>\n") {
				t.Fatalf("framing: %q", user)
			}
		})
	}
}

// The classify schema narrows the answer to the candidates.
func TestTextTaskClassifySchema(t *testing.T) {
	svc, fc := textSvc(`{"text":"良い"}`)
	if rec := postText(t, NewHandler(svc), "u1", `{"task":"classify","options":["良い","悪い"],"input":"晴れ"}`); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if s := string(fc.calls[0].Schema); !strings.Contains(s, `"enum":["良い","悪い"]`) {
		t.Fatalf("schema %s", s)
	}
}

func TestTextTaskBadOutput(t *testing.T) {
	for name, tc := range map[string]struct{ body, reply string }{
		"not JSON":        {`{"task":"summarize","input":"x"}`, "ごめんなさい"},
		"empty":           {`{"task":"summarize","input":"x"}`, `{"text":" "}`},
		"not an option":   {`{"task":"classify","options":["仕事","趣味"],"input":"x"}`, `{"text":"家事"}`},
		"two options hit": {`{"task":"classify","options":["仕事","趣味"],"input":"x"}`, `{"text":"仕事と趣味"}`},
	} {
		t.Run(name, func(t *testing.T) {
			svc, _ := textSvc(tc.reply)
			rec := postText(t, NewHandler(svc), "u1", tc.body)
			if rec.Code != http.StatusBadGateway || errCode(t, rec) != httpx.CodeBadOutput {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestTextTaskInputCaps(t *testing.T) {
	big := strings.Repeat("a", TextMaxInputBytes+1)
	opts := `["` + strings.Repeat("a", 41) + `","b"]`
	for name, tc := range map[string]struct {
		body   string
		status int
	}{
		"unknown task":         {`{"task":"translate","input":"x"}`, http.StatusBadRequest},
		"no input":             {`{"task":"summarize","input":"  "}`, http.StatusBadRequest},
		"input over 4KB":       {`{"task":"summarize","input":"` + big + `"}`, http.StatusRequestEntityTooLarge},
		"long instruction":     {`{"task":"summarize","instruction":"` + strings.Repeat("あ", 501) + `","input":"x"}`, http.StatusBadRequest},
		"shape on a task":      {`{"task":"summarize","shape":"list","input":"x"}`, http.StatusBadRequest},
		"count on a task":      {`{"task":"suggest","count":2,"input":"x"}`, http.StatusBadRequest},
		"classify one option":  {`{"task":"classify","options":["a"],"input":"x"}`, http.StatusBadRequest},
		"classify dup options": {`{"task":"classify","options":["a","a"],"input":"x"}`, http.StatusBadRequest},
		"classify long option": {`{"task":"classify","options":` + opts + `,"input":"x"}`, http.StatusBadRequest},
		"options on summarize": {`{"task":"summarize","options":["a","b"],"input":"x"}`, http.StatusBadRequest},
		"options on generate":  {`{"instruction":"x","options":["a","b"]}`, http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			svc, fc := textSvc(`{"text":"ok"}`)
			rec := postText(t, NewHandler(svc), "u1", tc.body)
			if rec.Code != tc.status {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			if len(fc.calls) != 0 {
				t.Fatal("Claude must not be called for a rejected request")
			}
		})
	}
}

// The transform tasks are the same Pro feature (onlineAI) as generate.
func TestTextTaskProRequired(t *testing.T) {
	svc, fc := textSvc(`{"text":"要約"}`)
	svc.Pro, svc.ProMode = proSet{"pro-user": true}, paid.ModeEnforce
	h := NewHandler(svc)
	rec := postText(t, h, "free-user", `{"task":"summarize","input":"日記"}`)
	if rec.Code != http.StatusPaymentRequired || errCode(t, rec) != httpx.CodeProRequired || len(fc.calls) != 0 {
		t.Fatalf("free: %d %s (calls %d)", rec.Code, rec.Body.String(), len(fc.calls))
	}
	if !strings.Contains(rec.Body.String(), `"id":"onlineAI"`) {
		t.Fatalf("details: %s", rec.Body.String())
	}
	if rec := postText(t, h, "pro-user", `{"task":"summarize","input":"日記"}`); rec.Code != http.StatusOK {
		t.Fatalf("pro: %d %s", rec.Code, rec.Body.String())
	}
}

// A transform task weighs TextTask units (2 by default), generate weighs 1.
func TestTextTaskRateLimitWeight(t *testing.T) {
	svc, fc := textSvc(`{"text":"1"}`, `{"text":"2"}`, `{"text":"3"}`)
	lim := ratelimit.New(ratelimit.Config{Limits: ratelimit.Limits{SignedIn: 3}, Weights: ratelimit.DefaultWeights},
		ratelimit.NewMemory(), nil)
	h := lim.Middleware(auth.DevVerifier{})(NewHandler(svc))
	if rec := postText(t, h, "u1", `{"task":"summarize","input":"x"}`); rec.Code != http.StatusOK {
		t.Fatalf("task: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postText(t, h, "u1", `{"task":"rewrite","input":"x"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second task (2+2 > 3): %d %s", rec.Code, rec.Body.String())
	}
	if rec := postText(t, h, "u1", `{"instruction":"x"}`); rec.Code != http.StatusOK {
		t.Fatalf("generate (2+1): %d %s", rec.Code, rec.Body.String())
	}
	if len(fc.calls) != 2 {
		t.Fatalf("Claude calls %d, want 2", len(fc.calls))
	}
}

// The usage log (ai.usage, operation text) has counts only: neither the
// record text sent as input nor the answer reaches any log line.
func TestTextTaskLogsNoContent(t *testing.T) {
	const secretInput, secretAnswer = "SECRET-DIARY 通院の記録", "SECRET-ANSWER まとめ"
	body, _ := json.Marshal(map[string]any{
		"model":       "claude-test",
		"content":     []map[string]any{{"type": "text", "text": `{"text":"` + secretAnswer + `"}`}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 300, "output_tokens": 80},
	})
	srv, _ := fakeAnthropic(t, [2]string{"200", string(body)})
	client, buf := testClient(srv, "claude-test")
	svc := &Service{Claude: client, Verifier: auth.DevVerifier{}, Logger: slog.New(slog.NewJSONHandler(buf, nil))}
	rec := postText(t, NewHandler(svc), "u1", `{"task":"summarize","input":"`+secretInput+`"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SECRET-ANSWER") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	lines := usageLines(t, buf)
	found := false
	for _, l := range lines {
		found = found || (l["msg"] == UsageEvent && l["operation"] == OpText)
	}
	if !found {
		t.Fatalf("no ai.usage text line: %s", buf.String())
	}
	if s := buf.String(); strings.Contains(s, "SECRET-DIARY") || strings.Contains(s, "SECRET-ANSWER") {
		t.Fatalf("content leaked into the logs: %s", s)
	}
}
