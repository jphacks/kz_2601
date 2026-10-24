package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every shipped example (docs + few-shot copies) must keep parsing and have
// a clean screen graph.
func TestNavExamplesClean(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(docsDir, "examples", "*.json"))
	more, _ := filepath.Glob("../../internal/ai/examples/*.json")
	files = append(files, more...)
	if len(files) < 5 {
		t.Fatalf("examples not found: %v", files)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		app, err := Parse(raw)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if u := app.Unreachable(); len(u) > 0 {
			t.Errorf("%s: unreachable screens %v", f, u)
		}
		for _, w := range app.Lint() {
			if strings.Contains(w, "not reachable from the initial screen") || strings.Contains(w, "no way back") {
				t.Errorf("%s: %s", f, w)
			}
		}
	}
}

// navDoc: home (tab) -> form (sheet) and a stats tab; %s adds screens.
const navDoc = `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"%s},"initialScreen":"home",
"entities":[{"id":"log","name":"記録","fields":[{"id":"title","name":"内容","type":"string"}]}],
"screens":[
 {"id":"home","title":"ホーム","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"log","item":{"title":"title"},"onItemTap":{"type":"navigate","screen":"form","presentation":"push"}}%s]},
 {"id":"form","title":"追加","entity":"log","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"log"}}],
  "components":[{"type":"textField","field":"title"}]},
 {"id":"stats","title":"集計","components":[{"type":"stat","label":"件数","aggregate":"count","entity":"log"}]}%s]}`

const twoTabs = `,"tabs":[{"screen":"home","title":"ホーム"},{"screen":"stats","title":"集計"}]`

func navRaw(tabs, homeExtra, screens string) string {
	parts := strings.SplitN(navDoc, "%s", 4)
	return parts[0] + tabs + parts[1] + homeExtra + parts[2] + screens + parts[3]
}

func navWarnings(t *testing.T, raw string) []string {
	t.Helper()
	var out []string
	for _, w := range lintApp(t, raw) {
		if strings.Contains(w, "not reachable from the initial screen") || strings.Contains(w, "no way back") {
			out = append(out, w)
		}
	}
	return out
}

func TestNavReachable(t *testing.T) {
	if w := navWarnings(t, navRaw(twoTabs, "", "")); len(w) > 0 {
		t.Fatalf("tabs app: unexpected %v", w)
	}
	// no tabs: stats is an orphan
	w := navWarnings(t, navRaw("", "", ""))
	if len(w) != 1 || !strings.Contains(w[0], `screen "stats" is not reachable`) || !strings.Contains(w[0], `"screen":"stats"`) {
		t.Fatalf("orphan stats: %v", w)
	}
	// a button from home fixes it
	btn := `,{"type":"button","label":"集計","action":{"type":"navigate","screen":"stats"}}`
	if w := navWarnings(t, navRaw("", btn, "")); len(w) > 0 {
		t.Fatalf("linked stats: unexpected %v", w)
	}
	// reached only through another unreachable screen: both reported
	detail := `,{"id":"detail","title":"詳細","components":[{"type":"text","text":"x"}]}`
	w = navWarnings(t, navRaw("", "", detail))
	if len(w) != 2 {
		t.Fatalf("want stats+detail, got %v", w)
	}
	// saveRecord then-screen counts as a way in
	app, err := Parse([]byte(navRaw(twoTabs, "", detail)))
	if err != nil {
		t.Fatal(err)
	}
	if got := app.Unreachable(); len(got) != 1 || got[0] != "detail" {
		t.Fatalf("Unreachable = %v", got)
	}
	app.Screens[1].Toolbar[0].Action.Screen = "detail"
	if got := app.Unreachable(); len(got) != 0 {
		t.Fatalf("then-screen: Unreachable = %v", got)
	}
	if !app.HasNav("form", "detail") || !app.HasNav("home", "form") || app.HasNav("home", "stats") {
		t.Fatal("HasNav")
	}
}

// An unreachable form is reported once (entityReach), not twice.
func TestNavUnreachableFormOnce(t *testing.T) {
	raw := strings.Replace(navRaw(twoTabs, "", ""), `"toolbar":[{"label":"追加","action":{"type":"navigate","screen":"form","presentation":"sheet"}}],`, "", 1)
	raw = strings.Replace(raw, `,"onItemTap":{"type":"navigate","screen":"form","presentation":"push"}`, "", 1)
	n := 0
	for _, w := range lintApp(t, raw) {
		if strings.Contains(w, `"form"`) && strings.Contains(w, "reachable") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want 1 warning for the form, got %d: %v", n, lintApp(t, raw))
	}
}

func TestNavFullScreenNeedsWayBack(t *testing.T) {
	btn := `,{"type":"button","label":"詳細","action":{"type":"navigate","screen":"detail","presentation":"fullScreen"}}`
	stuck := `,{"id":"detail","title":"詳細","components":[{"type":"text","text":"x"}]}`
	w := navWarnings(t, navRaw(twoTabs, btn, stuck))
	if len(w) != 1 || !strings.Contains(w[0], `screen "detail" is opened fullScreen`) || !strings.Contains(w[0], `"type":"back"`) {
		t.Fatalf("stuck fullScreen: %v", w)
	}
	back := `,{"id":"detail","title":"詳細","toolbar":[{"label":"閉じる","action":{"type":"back"}}],"components":[{"type":"text","text":"x"}]}`
	if w := navWarnings(t, navRaw(twoTabs, btn, back)); len(w) > 0 {
		t.Fatalf("with back: unexpected %v", w)
	}
	// a fullScreen form closes itself on save
	full := strings.Replace(navRaw(twoTabs, "", ""), `"screen":"form","presentation":"sheet"`, `"screen":"form","presentation":"fullScreen"`, 1)
	if w := navWarnings(t, full); len(w) > 0 {
		t.Fatalf("fullScreen form: unexpected %v", w)
	}
}

// Graph errors say where and how to fix (they are fed back to the AI).
func TestNavErrorsHint(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"navigate", navRaw(twoTabs, `,{"type":"button","label":"x","action":{"type":"navigate","screen":"settings"}}`, ""),
			`screen "home" components[1]: navigate target screen "settings" does not exist; use one of the screen ids (home, form, stats)`},
		{"then-screen", strings.Replace(navRaw(twoTabs, "", ""), `"type":"saveRecord","entity":"log"`, `"type":"saveRecord","entity":"log","screen":"done"`, 1),
			`saveRecord then-screen "done" does not exist`},
		{"tab target", navRaw(`,"tabs":[{"screen":"home","title":"a"},{"screen":"setting","title":"b"}]`, "", ""),
			`app.tabs[1]: screen "setting" does not exist; point the tab at one of the screen ids (home, form, stats)`},
		{"one tab", navRaw(`,"tabs":[{"screen":"home","title":"a"}]`, "", ""),
			`app.tabs must have 2-5 tabs (got 1)`},
		{"six tabs", navRaw(`,"tabs":[{"screen":"home","title":"a"},{"screen":"stats","title":"b"},{"screen":"s3","title":"c"},{"screen":"s4","title":"d"},{"screen":"s5","title":"e"},{"screen":"s6","title":"f"}]`, "",
			`,{"id":"s3","title":"3","components":[{"type":"text","text":"x"}]},{"id":"s4","title":"4","components":[{"type":"text","text":"x"}]},{"id":"s5","title":"5","components":[{"type":"text","text":"x"}]},{"id":"s6","title":"6","components":[{"type":"text","text":"x"}]}`),
			`app.tabs must have 2-5 tabs (got 6)`},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.raw))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q in %v", c.name, c.want, err)
		}
	}
}
