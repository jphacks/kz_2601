package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestResolveLocale(t *testing.T) {
	cases := []struct {
		explicit   Locale
		transcript string
		want       Locale
	}{
		{"", "家計簿アプリを作って", LocaleJA},
		{"", "Make an expense tracker", LocaleEN},
		{"", "カフェを追加 please", LocaleJA}, // any kana/kanji wins
		{"", "   ", LocaleJA},
		{LocaleEN, "家計簿", LocaleEN},
		{LocaleJA, "expense tracker", LocaleJA},
	}
	for _, c := range cases {
		got, err := resolveLocale(c.explicit, c.transcript)
		if err != nil || got != c.want {
			t.Errorf("resolveLocale(%q, %q) = %q, %v; want %q", c.explicit, c.transcript, got, err, c.want)
		}
	}
	_, err := resolveLocale("fr", "bonjour")
	var ge *Error
	if !errors.As(err, &ge) || ge.Status != http.StatusBadRequest {
		t.Fatalf("unknown locale must be 400, got %v", err)
	}
}

func TestCreate_EnglishPromptAndSummary(t *testing.T) {
	f := &fakeCaller{replies: []string{fewShotExpenseEN}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "e1", Transcript: "Make an expense tracker and remind me every evening at 8"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Locale != LocaleEN {
		t.Fatalf("locale: %q", res.Locale)
	}
	want := "I made an Expense Tracker app. Use the button at the bottom right to add expenses. You'll get a reminder every evening at 8."
	if res.Summary != want {
		t.Fatalf("summary:\n got %q\nwant %q", res.Summary, want)
	}
	user := f.calls[0].Messages[0].Content
	if !strings.Contains(user, "## 表示言語(locale)\nen (English)") {
		t.Fatalf("user turn must carry the locale line: %s", user)
	}
	if !strings.Contains(f.calls[0].System, `"name": "Expense Tracker"`) || !strings.Contains(f.calls[0].System, "例 2(locale en)") {
		t.Fatal("system prompt must embed the English few-shot")
	}
	// Japanese request: ja line, Japanese template.
	f.replies = []string{string(kakeibo(t))}
	res, err = svc.Generate(context.Background(), Request{AppID: "j1", Transcript: "家計簿アプリを作って"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Locale != LocaleJA || !strings.HasPrefix(res.Summary, "家計簿アプリを作りました。") {
		t.Fatalf("ja result: %q %q", res.Locale, res.Summary)
	}
	if !strings.Contains(f.calls[1].Messages[0].Content, "## 表示言語(locale)\nja(日本語)") {
		t.Fatal("ja user turn must carry the locale line")
	}
	// Explicit locale overrides detection and is echoed back.
	f.replies = []string{fewShotExpenseEN}
	res, err = svc.Generate(context.Background(), Request{AppID: "x1", Transcript: "家計簿アプリを作って", Locale: LocaleEN})
	if err != nil || res.Locale != LocaleEN || !strings.HasPrefix(res.Summary, "I made ") {
		t.Fatalf("explicit en: %v %+v", err, res)
	}
	if _, err := svc.Generate(context.Background(), Request{AppID: "x1", Transcript: "x", Locale: "de"}); err == nil {
		t.Fatal("unknown locale must fail")
	}
}

func TestEdit_EnglishLocaleLineInPrompt(t *testing.T) {
	f := &fakeCaller{replies: []string{fewShotExpenseEN, `{"patch":[{"op":"replace","path":"/app/name","value":"Budget"}],"summary":"Renamed the app to Budget."}`}}
	svc, _ := newSvc(f)
	ctx := context.Background()
	if _, err := svc.Generate(ctx, Request{AppID: "e", Transcript: "Make an expense tracker"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Generate(ctx, Request{AppID: "e", Transcript: "Rename the app to Budget"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != "patch" || res.Locale != LocaleEN || res.Summary != "Renamed the app to Budget." {
		t.Fatalf("edit: %+v", res)
	}
	if !strings.Contains(f.calls[1].Messages[0].Content, "## 表示言語(locale)\nen (English)") {
		t.Fatal("edit user turn must carry the locale line")
	}
}

func TestUndo_English(t *testing.T) {
	f := &fakeCaller{replies: []string{fewShotExpenseEN, `{"patch":[{"op":"replace","path":"/app/name","value":"Budget"}],"summary":"Renamed the app."}`}}
	svc, _ := newSvc(f)
	ctx := context.Background()
	res, err := svc.Generate(ctx, Request{AppID: "u", Transcript: "undo"})
	if err != nil || res.Summary != "There is nothing to undo." || res.Locale != LocaleEN {
		t.Fatalf("fresh undo: %v %+v", err, res)
	}
	if _, err := svc.Generate(ctx, Request{AppID: "u", Transcript: "Make an expense tracker"}); err != nil { // v2
		t.Fatal(err)
	}
	if _, err := svc.Generate(ctx, Request{AppID: "u", Transcript: "Rename the app to Budget"}); err != nil { // v3
		t.Fatal(err)
	}
	calls := len(f.calls)
	res, err = svc.Generate(ctx, Request{AppID: "u", Transcript: "Go back."})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != calls || res.Mode != "restore" || res.RestoredFrom != 2 {
		t.Fatalf("undo: %+v", res)
	}
	if res.Summary != "I went back to the previous state (version 2)." {
		t.Fatalf("summary: %q", res.Summary)
	}
}

func TestWantsUndo_English(t *testing.T) {
	yes := []string{"undo", "Undo that", "undo the last change", "go back", "Go back to the previous version",
		"revert that", "never mind", "scrap that", "cancel that", "roll back"}
	no := []string{"put the list back at the top", "add an undo button", "make the add button bigger",
		"go back and make the total bigger please", "Revert the list to show photos and also add a chart for categories"}
	for _, s := range yes {
		if !wantsUndo(s) {
			t.Errorf("%q must be undo", s)
		}
	}
	for _, s := range no {
		if wantsUndo(s) {
			t.Errorf("%q must not be undo", s)
		}
	}
	// The 16-rune bound still applies to Japanese.
	if wantsUndo("元に戻してから合計カードをもっと大きくして") {
		t.Error("long Japanese sentence must not be undo")
	}
}

func TestWantsNewApp(t *testing.T) {
	for _, s := range []string{"作り直して", "Make a new app for workouts", "Start over with a reading log", "Build it from scratch"} {
		if !wantsNewApp(s) {
			t.Errorf("%q must be a new app", s)
		}
	}
	for _, s := range []string{"カテゴリにカフェを追加して", "add a tab for history", "make the chart bigger"} {
		if wantsNewApp(s) {
			t.Errorf("%q must not be a new app", s)
		}
	}
}

func TestSummarizeEN_Pieces(t *testing.T) {
	for in, want := range map[string]string{"expense": "expenses", "entry": "entries", "day": "days", "box": "boxes", "book": "books", "": ""} {
		if got := pluralEN(in); got != want {
			t.Errorf("pluralEN(%q) = %q, want %q", in, got, want)
		}
	}
	if got := joinEN([]string{"a"}, "and"); got != "a" {
		t.Errorf("joinEN 1: %q", got)
	}
	if got := joinEN([]string{"a", "b"}, "and"); got != "a and b" {
		t.Errorf("joinEN 2: %q", got)
	}
	if got := joinEN([]string{"a", "b", "c"}, "or"); got != "a, b or c" {
		t.Errorf("joinEN 3: %q", got)
	}
	if withArticle("Expense Tracker app") != "an Expense Tracker app" || withArticle("Budget app") != "a Budget app" {
		t.Error("withArticle")
	}
	for area, want := range map[string]string{"D11:F12": "bottom right", "A1:B2": "top left", "A5:F6": "", "A11:F12": "bottom", "bad": ""} {
		if got := areaPositionEN(area); got != want {
			t.Errorf("areaPositionEN(%q) = %q, want %q", area, got, want)
		}
	}
	notif := []struct {
		repeat, at, want string
	}{
		{"daily", "08:00", "every morning at 8"},
		{"daily", "20:30", "every evening at 8:30"},
		{"daily", "13:00", "every day at 1 pm"},
		{"weekly", "09:00", "every week at 9"},
		{"once", "07:15", "at 7:15"},
		{"weekly", "", "every week"},
		{"daily", "", ""},
	}
	for _, n := range notif {
		if got := notificationWhenEN(dsl.Notification{Repeat: n.repeat, Time: n.at}); got != n.want {
			t.Errorf("notificationWhenEN(%s %s) = %q, want %q", n.repeat, n.at, got, n.want)
		}
	}
	if got := substitutionNoteEN("Track your plans. Calendar view was replaced with a date-sorted list."); got != "Calendar view was replaced with a date-sorted list." {
		t.Errorf("substitutionNoteEN: %q", got)
	}
}

func TestSummarizeEN_TabsAndSubstitution(t *testing.T) {
	app, err := dsl.Parse([]byte(fewShotExpenseEN))
	if err != nil {
		t.Fatal(err)
	}
	app.App.Tabs = []dsl.Tab{{Title: "Home", Screen: "home"}, {Title: "Stats", Screen: "home"}}
	app.App.Description = "Calendar view was replaced with a date-sorted list."
	got := summarizeEN(app)
	want := "I made an Expense Tracker app. It has 2 tabs: Home and Stats. Use the button at the bottom right to add expenses. You'll get a reminder every evening at 8. Calendar view was replaced with a date-sorted list."
	if got != want {
		t.Fatalf("summarizeEN:\n got %q\nwant %q", got, want)
	}
}

func TestFewShotEN_IsValidDSL(t *testing.T) {
	if _, err := dsl.Parse([]byte(fewShotExpenseEN)); err != nil {
		t.Fatalf("examples/expense_en.json must validate: %v", err)
	}
	if !strings.Contains(systemPrompt, "表示言語") || !strings.Contains(systemPrompt+"\n"+editRules, "It's already") {
		t.Fatal("prompt must carry the locale rules")
	}
}
