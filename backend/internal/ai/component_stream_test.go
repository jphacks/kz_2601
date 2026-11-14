package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
)

type gotComponent struct {
	screen string
	c      ComponentEvent
	at     int // bytes fed when emitted
}

func watchComponents(src string, chunk int) []gotComponent {
	var got []gotComponent
	fed := 0
	w := &componentWatcher{emit: func(s string, c ComponentEvent) { got = append(got, gotComponent{s, c, fed}) }}
	for i := 0; i < len(src); i += chunk {
		end := min(i+chunk, len(src))
		fed = end
		w.Feed(src[i:end])
	}
	return got
}

func componentIDs(got []gotComponent) string {
	var out []string
	for _, g := range got {
		out = append(out, g.screen+"/"+g.c.ID)
	}
	return strings.Join(out, ",")
}

func TestComponentWatcher_InitialTabInOrderOnce(t *testing.T) {
	compact := abbreviateExample(fewShotKakeibo)
	for name, src := range map[string]string{"compact": compact, "pretty": fewShotKakeibo, "fenced": "```json\n" + compact + "\n```\n"} {
		for _, n := range []int{1, 7, 64, len(src)} {
			got := watchComponents(src, n)
			if componentIDs(got) != "home/summary,home/add,home/recent" {
				t.Fatalf("%s/%d: %s", name, n, componentIDs(got))
			}
			for i, g := range got {
				if g.c.Index != []int{0, 1, 2}[i] {
					t.Errorf("%s/%d: index %d = %d", name, n, i, g.c.Index)
				}
			}
			if c := got[1].c; c.Type != "button" || c.Area != "D11:F12" || c.Label != "支出を追加" {
				t.Errorf("%s/%d: add = %+v", name, n, c)
			}
			if c := got[2].c; c.Type != "section" || c.Label != "最近の支出" {
				t.Errorf("%s/%d: recent = %+v", name, n, c)
			}
			// streamed: the first part is out long before the document ends
			if n < len(src) && got[0].at > strings.Index(src, `"expense_form"`)+n {
				t.Errorf("%s/%d: first component only at byte %d", name, n, got[0].at)
			}
		}
	}
}

func TestComponentWatcher_PicksFirstVisibleScreen(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"no tabs: initialScreen",
			`{"app":{"name":"a"},"initialScreen":"b","screens":[{"id":"a","components":[{"type":"text","id":"x"}]},{"id":"b","components":[{"type":"text","id":"y"},{"type":"button","id":"z","label":"Go"}]}]}`,
			"b/y,b/z"},
		{"initialScreen not a tab: first tab",
			`{"app":{"name":"a","tabs":[{"screen":"t1","title":"1"},{"screen":"t2","title":"2"}]},"initialScreen":"other","screens":[{"id":"t2","components":[{"type":"text","id":"no"}]},{"id":"t1","components":[{"type":"text","id":"yes"}]}]}`,
			"t1/yes"},
		{"initialScreen is the second tab",
			`{"app":{"name":"a","tabs":[{"screen":"t1","title":"1"},{"screen":"t2","title":"2"}]},"initialScreen":"t2","screens":[{"id":"t1","components":[{"type":"text","id":"no"}]},{"id":"t2","components":[{"type":"text","id":"yes"}]}]}`,
			"t2/yes"},
		{"initialScreen after the screens: buffered, then sent",
			`{"app":{"name":"a"},"screens":[{"id":"h","components":[{"type":"text","id":"p"},{"type":"text","id":"q"}]}],"initialScreen":"h"}`,
			"h/p,h/q"},
		{"screen id after its components",
			`{"app":{"name":"a"},"initialScreen":"h","screens":[{"components":[{"type":"text","id":"p"}],"id":"h"}]}`,
			"h/p"},
		{"no app block: nothing",
			`{"initialScreen":"h","screens":[{"id":"h","components":[{"type":"text","id":"p"}]}]}`,
			""},
	}
	for _, tc := range cases {
		for _, n := range []int{1, 5, len(tc.src)} {
			if got := componentIDs(watchComponents(tc.src, n)); got != tc.want {
				t.Errorf("%s/%d: %q, want %q", tc.name, n, got, tc.want)
			}
		}
	}
}

func TestComponentWatcher_EscapesAndNesting(t *testing.T) {
	// brackets / quotes inside strings, nested children, and a "components"
	// key at the wrong depth must not produce events.
	src := `{"app":{"name":"a\"}]{"},"initialScreen":"h","screens":[{"id":"h","title":"x]}\\","components":[` +
		`{"type":"text","id":"t1","text":"ok \"}]\" \\\\ あ"},` +
		`{"type":"section","id":"s1","title":"Sec","children":[{"type":"text","id":"inner","text":"{["}],"extra":{"components":[{"type":"text","id":"bogus"}]}},` +
		`{"type":"voiceInput","id":"v","prompt":"話して"}` +
		`]}]}`
	for _, n := range []int{1, 2, 3, len(src)} {
		got := watchComponents(src, n)
		if componentIDs(got) != "h/t1,h/s1,h/v" {
			t.Fatalf("%d: %s", n, componentIDs(got))
		}
		if got[0].c.Label != `ok "}]" \\ あ` || got[1].c.Label != "Sec" || got[2].c.Label != "話して" {
			t.Fatalf("%d: labels %+v", n, got)
		}
	}
}

func TestComponentWatcher_BrokenInputDoesNotPanic(t *testing.T) {
	good := `{"app":{"name":"a"},"initialScreen":"h","screens":[{"id":"h","components":[{"type":"text","id":"p"},`
	for name, tc := range map[string]struct{ src, want string }{
		"truncated mid-component": {good + `{"type":"text","id":"q`, "h/p"},
		"unbalanced close":        {good + `]]]]}{"type":"text","id":"q"}]}]}`, "h/p"},
		"mismatched bracket":      {good + `{"type":"text","id":"q"]}`, "h/p"},
		"not JSON in a component": {good + `{"type":1,"id":"q"},{"id":"r"},{"type":"text","id":"s"}]}]}`, "h/p,h/s"},
		"garbage only":            {"ごめんなさい、作れません。", ""},
		"empty":                   {"", ""},
		"trailing text":           {good + `{"type":"text","id":"q"}]}]}` + "\n説明 {\"x\":[1]}", "h/p,h/q"},
	} {
		for _, n := range []int{1, 4, 1 << 20} {
			if got := componentIDs(watchComponents(tc.src, max(n, 1))); got != tc.want {
				t.Errorf("%s/%d: %q, want %q", name, n, got, tc.want)
			}
		}
	}
	// skipped index stays: s is at position 3
	got := watchComponents(good+`{"type":1},{"id":"r"},{"type":"text","id":"s"}]}]}`, 3)
	if len(got) != 2 || got[1].c.Index != 3 {
		t.Fatalf("index after skipped parts: %+v", got)
	}
}

func FuzzComponentWatcher(f *testing.F) {
	f.Add(abbreviateExample(fewShotKakeibo), 7)
	f.Add(`{"app":{"name":"a"},"initialScreen":"h","screens":[{"id":"h","components":[{"type":"t"}]}]}`, 1)
	f.Add(`{"a":"\`, 2)
	f.Fuzz(func(t *testing.T, src string, n int) {
		if n <= 0 || n > 64 {
			n = 1
		}
		seen := map[int]bool{}
		for _, g := range watchComponents(src, n) {
			if seen[g.c.Index] {
				t.Fatalf("index %d sent twice", g.c.Index)
			}
			seen[g.c.Index] = true
		}
	})
}

// kakeiboWithBadPart is kakeibo plus an unknown component type on home:
// it streams components, then fails validation.
func kakeiboWithBadPart(t *testing.T) string {
	t.Helper()
	src := abbreviateExample(fewShotKakeibo)
	const anchor = `"components":[`
	i := strings.Index(src, anchor)
	if i < 0 {
		t.Fatal("no components in kakeibo")
	}
	i += len(anchor)
	return src[:i] + `{"type":"nosuchpart","id":"bad","area":"A1:F1"},` + src[i:]
}

func eventTrace(events []Event) string {
	var out []string
	for _, ev := range events {
		s := ev.Type + "@" + string(rune('0'+ev.Attempt))
		if ev.Component != nil {
			s += ":" + ev.Component.ID
		}
		out = append(out, s)
	}
	return strings.Join(out, ",")
}

func TestGenerateStream_RegenerationSendsRestart(t *testing.T) {
	f := &streamingFake{fakeCaller{replies: []string{kakeiboWithBadPart(t), abbreviateExample(fewShotKakeibo)}}}
	svc := &Service{Claude: f, Store: appconfigclient.NewMemory(), Model: "test-model"}
	var events []Event
	res, err := svc.GenerateStream(context.Background(), Request{AppID: "r1", Transcript: "家計簿アプリを作って"}, func(ev Event) { events = append(events, ev) })
	if err != nil {
		t.Fatal(err)
	}
	want := "app@1,component@1:bad,component@1:summary,component@1:add,component@1:recent," +
		"restart@2,app@2,component@2:summary,component@2:add,component@2:recent"
	if got := eventTrace(events); got != want {
		t.Fatalf("events\n got %s\nwant %s", got, want)
	}
	if res.Meta.Attempts != 2 || strings.Contains(string(res.DSL), "nosuchpart") {
		t.Fatalf("result must be the regenerated DSL: attempts=%d", res.Meta.Attempts)
	}
}

func TestGenerateStream_NoRestartWithoutComponents(t *testing.T) {
	bad := `{"app":{"name":"x"},"initialScreen":"nope","entities":[],"screens":[]}`
	f := &streamingFake{fakeCaller{replies: []string{bad, abbreviateExample(fewShotKakeibo)}}}
	svc := &Service{Claude: f, Store: appconfigclient.NewMemory(), Model: "test-model"}
	var events []Event
	if _, err := svc.GenerateStream(context.Background(), Request{AppID: "r2", Transcript: "家計簿アプリを作って"}, func(ev Event) { events = append(events, ev) }); err != nil {
		t.Fatal(err)
	}
	if got := eventTrace(events); got != "app@1,app@2,component@2:summary,component@2:add,component@2:recent" {
		t.Fatalf("events %s", got)
	}
}

func TestGenerate_NonStreamingUnchanged(t *testing.T) {
	f := &streamingFake{fakeCaller{replies: []string{kakeiboWithBadPart(t), abbreviateExample(fewShotKakeibo)}}}
	svc := &Service{Claude: f, Store: appconfigclient.NewMemory(), Model: "test-model"}
	res, err := svc.Generate(context.Background(), Request{AppID: "n1", Transcript: "家計簿アプリを作って"})
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range f.calls {
		if c.OnText != nil {
			t.Fatalf("call %d: JSON mode must not stream", i)
		}
	}
	if res.Meta.Attempts != 2 || res.Mode != "full" {
		t.Fatalf("result %+v", res.Meta)
	}
}
