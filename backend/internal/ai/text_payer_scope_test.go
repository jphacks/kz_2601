package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// The device fills an aiButton prompt in: {{input}} becomes a marker, the
// other {{field}} the field's value; the result is trimmed and cut at 500.
func TestMatchTemplate(t *testing.T) {
	long := strings.Repeat("あ", dsl.MaxAIInstructionRunes)
	cases := []struct {
		tmpl, got string
		want      bool
	}{
		{"励ましの一言", "励ましの一言", true},
		{"励ましの一言", "励ましの一言を長く", false},
		{"励ましの一言", "別の指示", false},
		{"{{input}}を要約", "下の材料を要約", true},
		{"Summarize {{input}}", "Summarize the data below", true},
		{"{{input}}を要約", "なんでもを要約", false},
		{"{{name}}さんへの手紙", "山田 太郎さんへの手紙", true},
		{"{{name}}さんへの手紙", "さんへの手紙", true},
		{"{{name}}さんへの手紙", "さんへの手紙と別の話", false},
		{"{{a}}と{{b}}の比較", "りんごとみかんの比較", true},
		{"{{a}}と{{b}}の比較", "りんごとみかんの違い", false},
		{"  前後の空白  ", "前後の空白", true},
		{"{{x}}  末尾", "末尾", true},
		{"", "", true},
		{"", "何か", false},
		{"あ{{x}}い", long, false}, // cut inside the value, but the value adds over 400
		{strings.Repeat("あ", 300) + "{{x}}" + strings.Repeat("い", 300), long, true},
		{strings.Repeat("あ", 200) + "{{x}}い", long, true}, // 300 of value
		{"い{{x}}", long, false},
		{strings.Repeat("う", 450) + "{{x}}" + strings.Repeat("え", 100), strings.Repeat("う", 450) + strings.Repeat("え", 50), true}, // cut in the literal
		{strings.Repeat("う", 450) + "{{x}}" + strings.Repeat("え", 100), strings.Repeat("う", 450) + strings.Repeat("お", 50), true}, // cut in the value
		{strings.Repeat("う", 450) + "え", strings.Repeat("う", 449) + "お" + "x" + strings.Repeat("う", 49), false},
		// iOS replaces "{{input}}" only as it is: "{{ input }}" stays literal text
		{"{{ input }}を要約", "下の材料を要約", false},
		{"{{ input }}を要約", "{{ input }}を要約", true},
		{"{{ name }}さん", "山田さん", false},
		{"{{Name}}さん", "{{Name}}さん", true},
		// the values add at most 400 runes
		{"{{a}}について", strings.Repeat("x", 400) + "について", true},
		{"{{a}}について", strings.Repeat("x", 401) + "について", false},
		{"{{a}}と{{b}}", strings.Repeat("x", 200) + "と" + strings.Repeat("y", 201), false},
		// special characters of the regexp are literal text
		{"a.b*{{x}}(c)", "a.b*値(c)", true},
		{"a.b*{{x}}(c)", "aXbb値(c)", false},
	}
	for _, c := range cases {
		cut := len([]rune(c.got)) >= dsl.MaxAIInstructionRunes
		tm, err := compileTemplate(c.tmpl)
		if err != nil {
			t.Fatalf("%q: %v", c.tmpl, err)
		}
		if got := tm.match(c.got, cut); got != c.want {
			t.Errorf("%q vs %q = %v, want %v", c.tmpl, c.got, got, c.want)
		}
	}
}

func TestRequestInApp(t *testing.T) {
	var app dsl.App
	if err := json.Unmarshal([]byte(sharedAppDSL), &app); err != nil {
		t.Fatal(err)
	}
	img := &TextImage{MediaType: "image/jpeg", Data: "x"}
	cases := []struct {
		name string
		req  TextRequest
		want bool
	}{
		{"generate", TextRequest{Instruction: "励ましの一言"}, true},
		{"generate other prompt", TextRequest{Instruction: "別の文章"}, false},
		{"task in a stack", TextRequest{Task: "extract", Instruction: "薬", Image: img}, true},
		{"task differs", TextRequest{Task: "summarize", Instruction: "薬"}, false},
		{"photo without inputPhoto", TextRequest{Instruction: "励ましの一言", Image: img}, false},
		{"long button", TextRequest{Instruction: "手紙", Length: "long"}, true},
		{"long on a short button", TextRequest{Instruction: "励ましの一言", Length: "long"}, false},
		{"chat persona", TextRequest{Task: "chat", Instruction: "p"}, true},
		{"chat other persona", TextRequest{Task: "chat", Instruction: "別の役"}, false},
		{"chat with a button prompt", TextRequest{Task: "chat", Instruction: "励ましの一言"}, false},
	}
	for _, c := range cases {
		if got := requestInApp(&app, c.req); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// A member's request the creator would pay for must match a component of the
// shared app; otherwise the caller's own Pro decides (402 without a reason).
func TestTextCreatorPaysOnlyForTheAppsComponents(t *testing.T) {
	m := store.NewMemory()
	proApp, _ := sharedAppFixture(t, m, "")
	if _, err := m.ClaimApp(context.Background(), "pro-owner", "U"); err != nil { // unpublished
		t.Fatal(err)
	}
	if _, err := m.AddMember(context.Background(), "U", store.Member{UID: "member", Via: store.MemberViaShare, InvitedBy: "pro-owner"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, body string
		status     int
	}{
		{"matching button", `{"instruction":"励ましの一言","appId":"` + proApp + `"}`, http.StatusOK},
		{"other instruction", `{"instruction":"関係のない長い文章を書いて","appId":"` + proApp + `"}`, http.StatusPaymentRequired},
		{"other task", `{"task":"rewrite","input":"a","instruction":"励ましの一言","appId":"` + proApp + `"}`, http.StatusPaymentRequired},
		{"other persona", chatWithApp(t, "別の役", proApp), http.StatusPaymentRequired},
		{"matching persona", chatWithApp(t, "p", proApp), http.StatusOK},
		{"unpublished app", `{"instruction":"励ましの一言","appId":"U"}`, http.StatusPaymentRequired},
	} {
		t.Run(c.name, func(t *testing.T) {
			svc, fc := textSvc(`{"text":"ok"}`)
			svc.Pro, svc.ProMode, svc.Members = proSet{"pro-owner": true}, paid.ModeEnforce, m
			rec := postText(t, NewHandler(svc), "member", c.body)
			if rec.Code != c.status {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			if c.status != http.StatusOK {
				if len(fc.calls) != 0 || textReason(t, rec.Body.Bytes()) != "" {
					t.Fatalf("calls %d body %s", len(fc.calls), rec.Body.String())
				}
			}
		})
	}
}

func chatWithApp(t *testing.T, persona, app string) string {
	return strings.Replace(chatBody(t, persona, "hi"), `"task":"chat"`, `"task":"chat","appId":"`+app+`"`, 1)
}

// The creator-paid calls of one app are capped per day: past the cap 429
// rate_limited scope app (no Claude call); another app and a Pro caller's
// own calls are not affected, and the next day counts again.
func TestTextCreatorPaidDailyCap(t *testing.T) {
	m := store.NewMemory()
	proApp, _ := sharedAppFixture(t, m, "")
	other, _ := sharedAppFixture(t, m, "2")
	now := time.Date(2026, 11, 8, 14, 0, 0, 0, time.UTC) // 23:00 JST
	ok := `{"text":"ok"}`
	svc, fc := textSvc(ok, ok, ok, ok, ok)
	svc.Pro, svc.ProMode, svc.Members = proSet{"pro-owner": true}, paid.ModeEnforce, m
	svc.CreatorPaid = &CreatorPaidCap{Daily: 2, Env: "test", Now: func() time.Time { return now }}
	h := NewHandler(svc)
	body := func(app string) string { return `{"instruction":"励ましの一言","appId":"` + app + `"}` }
	for i := range 2 {
		if rec := postText(t, h, "member", body(proApp)); rec.Code != http.StatusOK {
			t.Fatalf("#%d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec := postText(t, h, "member", body(proApp))
	if rec.Code != http.StatusTooManyRequests || errCode(t, rec) != httpx.CodeRateLimited || !strings.Contains(rec.Body.String(), `"scope":"app"`) {
		t.Fatalf("over the cap: %d %s", rec.Code, rec.Body.String())
	}
	if len(fc.calls) != 2 {
		t.Fatalf("calls %d", len(fc.calls))
	}
	if rec := postText(t, h, "member", body(other)); rec.Code != http.StatusOK {
		t.Fatalf("other app: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postText(t, h, "pro-owner", body(proApp)); rec.Code != http.StatusOK {
		t.Fatalf("Pro caller pays for itself: %d %s", rec.Code, rec.Body.String())
	}
	now = now.Add(2 * time.Hour) // 0:00 JST passed
	if rec := postText(t, h, "member", body(proApp)); rec.Code != http.StatusOK {
		t.Fatalf("next day: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreatorPaidDailyFromEnv(t *testing.T) {
	for raw, want := range map[string]int{"": DefaultCreatorPaidDaily, "0": 0, "50": 50} {
		got, err := CreatorPaidDailyFromEnv(func(string) string { return raw })
		if err != nil || got != want {
			t.Errorf("%q: %d %v, want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"-1", "x"} {
		if _, err := CreatorPaidDailyFromEnv(func(string) string { return raw }); err == nil {
			t.Errorf("%q: no error", raw)
		}
	}
}
