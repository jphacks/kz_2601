package ai

import (
	"net/http"
	"strings"
	"testing"
)

// The format hint goes after </instruction>: the instruction stays the
// button prompt (what the creator-paid scope compares).
func TestTextFormatsHintOutsideInstruction(t *testing.T) {
	svc, fc := textSvc(`{"fields":[{"name":"点数","value":"85"},{"name":"日","value":"11/20"},{"name":"気分","value":"良い"},{"name":"講評","value":"よい"}]}`)
	body := `{"instruction":"答えを採点","shape":"fields","items":["点数","日","気分","講評"],` +
		`"formats":[{"item":"点数","kind":"number"},{"item":"日","kind":"date"},{"item":" 気分 ","kind":"enum","options":["良い"," 悪い"]}]}`
	rec := postText(t, NewHandler(svc), "u1", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	user := fc.calls[0].Messages[0].Content
	if !strings.HasPrefix(user, "<instruction>\n答えを採点\n</instruction>\n") {
		t.Fatalf("instruction changed: %q", user)
	}
	want := "書き方: 点数=数字だけ / 日=YYYY/M/D(年が無ければ M/D) / 気分=良い/悪い のどれか(分からなければ -)\n"
	if !strings.Contains(user, want) {
		t.Fatalf("hint missing: %q", user)
	}
	if strings.Contains(user, "講評=") {
		t.Fatal("items without a format get no hint")
	}
}

func TestTextFormatsHintEnglish(t *testing.T) {
	got := textFormatHint(TextRequest{Formats: []TextFormat{{Item: "Start", Kind: "datetime"}}}, LocaleEN)
	if got != "Format: Start=YYYY/M/D H:mm (M/D H:mm if no year) (unknown: -)\n" {
		t.Fatalf("got %q", got)
	}
	if textFormatHint(TextRequest{}, LocaleJA) != "" {
		t.Fatal("no formats, no hint")
	}
}

func TestTextFormatsRejected(t *testing.T) {
	opts := `"` + strings.Repeat("a", 41) + `"`
	for name, body := range map[string]string{
		"not an item":  `{"instruction":"x","shape":"fields","items":["a"],"formats":[{"item":"b","kind":"number"}]}`,
		"duplicate":    `{"instruction":"x","shape":"fields","items":["a"],"formats":[{"item":"a","kind":"number"},{"item":"a","kind":"date"}]}`,
		"bad kind":     `{"instruction":"x","shape":"fields","items":["a"],"formats":[{"item":"a","kind":"text"}]}`,
		"enum no opts": `{"instruction":"x","shape":"fields","items":["a"],"formats":[{"item":"a","kind":"enum"}]}`,
		"long option":  `{"instruction":"x","shape":"fields","items":["a"],"formats":[{"item":"a","kind":"enum","options":[` + opts + `]}]}`,
		"opts on date": `{"instruction":"x","shape":"fields","items":["a"],"formats":[{"item":"a","kind":"date","options":["x"]}]}`,
		"on line":      `{"instruction":"x","formats":[{"item":"a","kind":"number"}]}`,
		"on a task":    `{"task":"summarize","input":"y","formats":[{"item":"a","kind":"number"}]}`,
		"on aiChat":    `{"task":"chat","instruction":"x","messages":[{"role":"user","text":"hi"}],"formats":[{"item":"a","kind":"number"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc, fc := textSvc(`{"text":"ok"}`)
			rec := postText(t, NewHandler(svc), "u1", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			if len(fc.calls) != 0 {
				t.Fatal("Claude must not be called for a rejected request")
			}
		})
	}
}
