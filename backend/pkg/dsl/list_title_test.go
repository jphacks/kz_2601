package dsl

import (
	"strings"
	"testing"
)

const optionalTitleMsg = "is an optional field, so a record saved without it shows a blank title"

func noLint(t *testing.T, got []string, substr string) {
	t.Helper()
	for _, w := range got {
		if strings.Contains(w, substr) {
			t.Errorf("unexpected warning containing %q: %s", substr, w)
		}
	}
}

const requiredCategory = `,{"id":"category","name":"カテゴリ","type":"enum","required":true,"options":["食費","交通"]}`

func TestLintOptionalTitle(t *testing.T) {
	// kakeibo: title = optional memo while category is required
	got := lintWith(t, requiredCategory, "")
	wantLint(t, got, `list item.title "title" `+optionalTitleMsg)
	wantLint(t, got, `set the title to the required field "category" and move "title" to the subtitle`)

	// a required string is an alternative too
	wantLint(t, lintWith(t, `,{"id":"shop","name":"店","type":"string","required":true}`, ""), `required field "shop"`)

	// timeline titleField
	timeline := strings.Replace(strings.Replace(lintBase, "%s", requiredCategory, 1), "%s",
		`{"type":"timeline","entity":"log","dateField":"date","titleField":"title"},`, 1)
	wantLint(t, lintApp(t, timeline), `timeline titleField "title" `+optionalTitleMsg)
}

func TestLintOptionalTitle_Clean(t *testing.T) {
	cases := map[string]string{
		// no required alternative: nothing better to show
		"no alternative": lintBase,
		// the title itself is required
		"required title": strings.Replace(lintBase, `{"id":"title","name":"内容","type":"string"}`, `{"id":"title","name":"内容","type":"string","required":true}`, 1),
		// a default value is never empty
		"default value": strings.Replace(lintBase, `{"id":"title","name":"内容","type":"string"}`, `{"id":"title","name":"内容","type":"string","defaultValue":"記録"}`, 1),
		// already titled by the required field
		"title is required enum": strings.Replace(lintBase, `"item":{"title":"title"}`, `"item":{"title":"category","subtitle":"title"}`, 1),
		// the only required alternative is a number (not a label)
		"number alternative": strings.Replace(lintBase, `{"id":"amount","name":"金額","type":"currency"}`, `{"id":"amount","name":"金額","type":"currency","required":true}`, 1),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if name != "no alternative" && name != "number alternative" {
				raw = strings.Replace(raw, "%s", requiredCategory, 1)
			}
			raw = strings.Replace(strings.Replace(raw, "%s", "", 1), "%s", "", 1)
			noLint(t, lintApp(t, raw), optionalTitleMsg)
		})
	}
}
