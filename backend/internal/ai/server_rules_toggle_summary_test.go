package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

const toggleRuleMorning = `{"id":"water_morning","on":{"every":"daily","time":"07:00"}%s,
"do":[{"notify":{"to":{"kind":"owner"},"title":"水分の記録","body":"今日の水分を記録しましょう"}}]}`

const toggleRuleNight = `{"id":"water_night","on":{"every":"daily","time":"21:00"}%s,
"do":[{"notify":{"to":{"kind":"owner"},"title":"水分の記録","body":"%s"}}]}`

func toggleApp(t *testing.T, rules ...string) *dsl.App {
	t.Helper()
	raw := strings.TrimSuffix(strings.TrimSpace(waterApp), "}") + `,"serverRules":[` + strings.Join(rules, ",") + `]}`
	a, err := dsl.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	return a
}

func morning(enabled string) string {
	return strings.Replace(toggleRuleMorning, "%s", enabled, 1)
}

func night(enabled, body string) string {
	s := strings.Replace(toggleRuleNight, "%s", enabled, 1)
	return strings.Replace(s, "%s", body, 1)
}

const off = `,"enabled":false`

// #347: a rule only switched off / on is said as such, not as "added".
func TestDescribeRuleChanges_Toggle(t *testing.T) {
	onApp := toggleApp(t, morning(""), night("", "寝る前に記録"))
	offApp := toggleApp(t, morning(off), night("", "寝る前に記録"))
	desc := onApp.DescribeServerRule(&onApp.ServerRules[0])

	cases := []struct {
		name          string
		before, after *dsl.App
		loc           Locale
		want          string
	}{
		{"off ja", onApp, offApp, LocaleJA, "「" + desc + "」をオフにしました。"},
		{"on ja", offApp, onApp, LocaleJA, "「" + desc + "」をオンにしました。"},
		{"off en", onApp, offApp, LocaleEN, "Turned off 1 automatic action."},
		{"on en", offApp, onApp, LocaleEN, "Turned on 1 automatic action."},
		{"same", onApp, onApp, LocaleJA, ""},
	}
	for _, c := range cases {
		if got := describeRuleChanges(c.before, c.after, c.loc); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestDescribeRuleChanges_Mixed(t *testing.T) {
	before := toggleApp(t, morning(""), night("", "寝る前に記録"))
	empty := toggleApp(t)
	// off + an edited rule + a new one
	after := toggleApp(t, morning(off), night("", "寝る前に水を"),
		`{"id":"water_noon","on":{"every":"daily","time":"12:00"},"do":[{"notify":{"to":{"kind":"owner"},"title":"昼","body":"昼も記録"}}]}`)

	ja := describeRuleChanges(before, after, LocaleJA)
	for _, w := range []string{"自動の動き: ", after.DescribeServerRule(&after.ServerRules[1]), after.DescribeServerRule(&after.ServerRules[2]), "をオフにしました。"} {
		if !strings.Contains(ja, w) {
			t.Errorf("ja %q must contain %q", ja, w)
		}
	}
	if strings.Contains(ja, dsl.DisabledNote) {
		t.Errorf("ja %q: the toggled rule must not carry the (オフ) note", ja)
	}
	if en, want := describeRuleChanges(before, after, LocaleEN), "Added 1 automatic action that runs on the server. Updated 1 automatic action. Turned off 1 automatic action."; en != want {
		t.Errorf("en: got %q want %q", en, want)
	}

	// added from nothing / removed
	if en, want := describeRuleChanges(empty, before, LocaleEN), "Added 2 automatic actions that run on the server."; en != want {
		t.Errorf("en add: got %q want %q", en, want)
	}
	if ja, want := describeRuleChanges(before, empty, LocaleJA), "自動の動きを 2 件消しました。"; ja != want {
		t.Errorf("ja remove: got %q want %q", ja, want)
	}
	if en, want := describeRuleChanges(before, empty, LocaleEN), "Removed 2 automatic actions."; en != want {
		t.Errorf("en remove: got %q want %q", en, want)
	}

	// enabled flipped together with another change = an edit (description keeps the note)
	both := toggleApp(t, morning(""), night(off, "寝る前に水を"))
	if ja := describeRuleChanges(before, both, LocaleJA); !strings.HasPrefix(ja, "自動の動き: ") || !strings.Contains(ja, dsl.DisabledNote) {
		t.Errorf("ja edit+off: %q", ja)
	}
}
