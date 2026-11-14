package dsl

import (
	"strings"
	"testing"
)

// openAppDoc: a 割り勘 app; comp goes on the form screen (screen.entity bill).
func openAppDoc(comp string) string {
	return `{"schemaVersion":1,"app":{"name":"割り勘","voiceEntry":"none"},"initialScreen":"home",
"entities":[{"id":"bill","name":"支払い","titleField":"title","fields":[
 {"id":"title","name":"内容","type":"string","required":true},{"id":"amount","name":"金額","type":"currency"},
 {"id":"phone","name":"電話","type":"string"},{"id":"address","name":"住所","type":"string"},
 {"id":"memo","name":"秘密","type":"string","hidden":true},{"id":"pic","name":"写真","type":"photo"}]}],
"screens":[
 {"id":"home","title":"一覧","components":[
  {"type":"list","entity":"bill","item":{"title":"title"},"onItemTap":{"type":"navigate","screen":"form"}},
  {"type":"button","label":"追加","action":{"type":"navigate","screen":"form"}}]},
 {"id":"form","title":"支払い","entity":"bill","components":[{"type":"textField","field":"title"},` + comp + `],
  "toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"bill"}}]}]}`
}

func TestOpenAppValid(t *testing.T) {
	for _, comp := range []string{
		`{"type":"openUrlButton","app":"paypay"}`,
		`{"type":"openUrlButton","app":"paypay","label":"PayPay で払う","to":"https://qr.paypay.ne.jp/p2p01_abc"}`,
		`{"type":"openUrlButton","app":"line","to":"https://line.me/R/ti/p/%40shop"}`,
		`{"type":"openUrlButton","app":"slack","to":"https://myteam.slack.com/archives/C123"}`,
		`{"type":"openUrlButton","app":"discord","to":"https://discord.gg/abc"}`,
		`{"type":"openUrlButton","app":"x","to":"@jphacks"}`,
		`{"type":"openUrlButton","app":"instagram","to":"some.user_1"}`,
		`{"type":"openUrlButton","app":"maps","to":"金沢駅"}`,
		`{"type":"openUrlButton","app":"maps","to":"{{address}}"}`,
		`{"type":"openUrlButton","app":"mail","to":"a@example.com"}`,
		`{"type":"openUrlButton","app":"phone","to":"076-123-4567"}`,
		`{"type":"openUrlButton","app":"phone","to":"{{phone}}"}`,
		`{"type":"openUrlButton","label":"公式","url":"https://example.com"}`,
		`{"type":"shareButton","text":"{{title}} {{amount}}円","shareTo":"x"}`,
		`{"type":"shareButton","text":"{{title}}","shareTo":"mail"}`,
	} {
		if _, err := Parse([]byte(openAppDoc(comp))); err != nil {
			t.Errorf("%s: %v", comp, err)
		}
	}
}

func TestOpenAppInvalid(t *testing.T) {
	for comp, want := range map[string]string{
		`{"type":"openUrlButton","app":"telegram"}`:                                        "must be one of",
		`{"type":"openUrlButton","app":"paypay","url":"https://paypay.ne.jp/"}`:            "not both",
		`{"type":"openUrlButton","app":"paypay","to":"https://evil.example/paypay.ne.jp"}`: "https link on paypay.ne.jp",
		`{"type":"openUrlButton","app":"paypay","to":"http://qr.paypay.ne.jp/x"}`:          "https link",
		`{"type":"openUrlButton","app":"slack","to":"https://slack.com.evil.example/"}`:    "https link",
		`{"type":"openUrlButton","app":"x","to":"not a user!"}`:                            "username",
		`{"type":"openUrlButton","app":"mail","to":"nope"}`:                                "email",
		`{"type":"openUrlButton","app":"phone"}`:                                           "needs to",
		`{"type":"openUrlButton","app":"phone","to":"call me"}`:                            "phone number",
		`{"type":"openUrlButton","app":"phone","to":"{{nope}}"}`:                           "not a field",
		`{"type":"openUrlButton","app":"phone","to":"{{memo}}"}`:                           "visible text-like",
		`{"type":"openUrlButton","app":"maps","to":"{{pic}}"}`:                             "visible text-like",
		`{"type":"openUrlButton","app":"maps","to":"` + strings.Repeat("あ", 301) + `"}`:    "too long",
		`{"type":"openUrlButton","label":"x","to":"a"}`:                                    "to needs app",
		`{"type":"openUrlButton","label":"x"}`:                                             "label and url (or app)",
		`{"type":"button","label":"x","app":"line","action":{"type":"back"}}`:              "app",
		`{"type":"shareButton","text":"a","shareTo":"telegram"}`:                           "shareTo",
	} {
		_, err := Parse([]byte(openAppDoc(comp)))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", comp, err, want)
		}
	}
}

// {{x}} on a screen without screen.entity has no record to read.
func TestOpenAppPlaceholderNeedsEntity(t *testing.T) {
	var probs []string
	checkOpenURLButton("c", &Component{Type: CompOpenURLButton, App: "maps", To: "{{address}}"}, nil, func(f string, a ...any) { probs = append(probs, f) })
	if len(probs) != 1 || !strings.Contains(probs[0], "requires screen.entity") {
		t.Fatalf("problems = %v", probs)
	}
}

func TestOpenAppLinkHost(t *testing.T) {
	for _, c := range []struct {
		app, s string
		ok     bool
	}{
		{"paypay", "https://qr.paypay.ne.jp/x", true},
		{"paypay", "https://paypay.ne.jp/", true},
		{"paypay", "https://paypay.ne.jp.evil.example/", false},
		{"paypay", "https://user@paypay.ne.jp/", false},
		{"line", "https://lin.ee/abc", true},
		{"discord", "https://discord.com/invite/abc", true},
		{"x", "https://twitter.com/jphacks", true},
		{"maps", "https://maps.apple.com/", false}, // maps takes a place, not a link
	} {
		if got := OpenAppLinkHost(c.app, c.s); got != c.ok {
			t.Errorf("%s %s = %v", c.app, c.s, got)
		}
	}
}

func TestShareTextLint(t *testing.T) {
	app, err := Parse([]byte(openAppDoc(`{"type":"shareButton","text":"{{title}} {{nope}} ` + strings.Repeat("あ", ShareTextMax) + `","shareTo":"line"}`)))
	if err != nil {
		t.Fatal(err)
	}
	var long, unknown bool
	for _, p := range app.Lint() {
		long = long || strings.Contains(p, "longer than")
		unknown = unknown || strings.Contains(p, "{{nope}} is not a field")
		if strings.Contains(p, "{{title}}") {
			t.Errorf("unexpected lint: %s", p)
		}
	}
	if !long || !unknown {
		t.Fatalf("lint = %v", app.Lint())
	}
}
