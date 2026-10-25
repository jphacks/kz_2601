package ai

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestClassifyShareIntent(t *testing.T) {
	for in, want := range map[string]shareIntent{
		"旅行の割り勘。友達のスマホからも入力したい":          shareEveryone,
		"この割り勘を友達のスマホからも入力できるようにして":      shareEveryone,
		"家族で共有する家事分担のアプリ":                shareEveryone,
		"この家計簿を家族だけで共有して":                shareLimited,
		"招待した人だけが見られるようにして":              shareLimited,
		"Share this with my family only": shareLimited,
		"この記録は自分だけにして":                   sharePrivate,
		"LINE で共有できるボタンを付けて":             shareNone,
		"家族の誕生日を記録したい":                   shareNone,
		"カテゴリにカフェを追加して":                  shareNone,
		"Let me share with my friends":   shareEveryone,
		"家族だけで共有して(無料版で作る: 他のユーザーとデータを共有する機能は入れない。みんなで書き込む・投稿はせず、すべて自分だけの記録として作る)": sharePrivate,
	} {
		if got := classifyShareIntent(in); got != want {
			t.Errorf("%q: got %d want %d", in, got, want)
		}
	}
}

// One rule text reaches the create, edit and chat prompts (#259).
func TestShareRules_InEveryPrompt(t *testing.T) {
	for name, p := range map[string]string{"create": systemPrompt, "edit": systemPrompt + "\n" + editRules} {
		if !strings.Contains(p, shareRules) {
			t.Errorf("%s prompt lacks shareRules", name)
		}
	}
	for name, p := range map[string]string{"chat edit": chatEditSystemPrompt, "chat create": chatCreateSystemPrompt} {
		if !strings.Contains(p, shareChatRule) {
			t.Errorf("%s prompt lacks shareChatRule", name)
		}
	}
	for _, w := range []string{"友達のスマホ", "家族だけ", "/entities/<i>/scope"} {
		if !strings.Contains(shareRules, w) {
			t.Errorf("shareRules lacks %q", w)
		}
	}
}

func warikanWithScope(t *testing.T, scope string) []byte {
	t.Helper()
	raw, err := os.ReadFile("examples/warikan.json")
	if err != nil {
		t.Fatal(err)
	}
	if scope == "" {
		return raw
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, e := range doc["entities"].([]any) {
		e.(map[string]any)["scope"] = scope
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func scopesOf(t *testing.T, raw []byte) map[string]dsl.EntityScope {
	t.Helper()
	a, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]dsl.EntityScope{}
	for i := range a.Entities {
		out[a.Entities[i].ID] = a.Entities[i].EffectiveScope()
	}
	return out
}

const declinedSharePatch = `{"summary":"友達のスマホからの入力はできないため、そのままにしました。","patch":[{"op":"test","path":"/entities/0/id","value":"members"}]}`
const sharePatch = `{"summary":"メンバーと支出を友達と共有できるようにしました。","patch":[{"op":"add","path":"/entities/0/scope","value":"public"},{"op":"add","path":"/entities/1/scope","value":"public"}]}`

func editSvc(t *testing.T, base []byte, replies ...string) (*Service, *fakeCaller) {
	t.Helper()
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "a1", 1, base, appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeCaller{replies: replies}
	return &Service{Claude: f, Store: store, Model: "test-model"}, f
}

// #259: the same request gives the same scopes whether it creates the app or
// edits it, and an edit that declines gets a retry with the patch recipe.
func TestShare_CreateAndEditGiveSameScope(t *testing.T) {
	createSvc, _ := newSvc(&fakeCaller{replies: []string{string(warikanWithScope(t, "public"))}})
	created, err := createSvc.Generate(context.Background(), Request{AppID: "c1",
		Transcript: "旅行の割り勘。友達それぞれが払った分を入力して、友達のスマホからも入力したい"})
	if err != nil {
		t.Fatal(err)
	}

	svc, f := editSvc(t, warikanWithScope(t, ""), declinedSharePatch, sharePatch)
	edited, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "この割り勘を友達のスマホからも入力できるようにして"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || !strings.Contains(f.calls[1].Messages[0].Content, "/entities/<i>/scope") {
		t.Fatalf("want a retry with the scope recipe, calls=%d", len(f.calls))
	}
	cs, es := scopesOf(t, created.DSL), scopesOf(t, edited.DSL)
	for id, s := range cs {
		if es[id] != s {
			t.Errorf("entity %s: create %s, edit %s", id, s, es[id])
		}
	}
	if edited.Unchanged || edited.Mode != "patch" {
		t.Fatalf("edit should be saved as a patch: %+v", edited)
	}
	for name, s := range map[string]string{"create": created.Summary, "edit": edited.Summary} {
		if !strings.Contains(s, "誰でも見られます") {
			t.Errorf("%s summary should state the visibility: %s", name, s)
		}
	}
}

func TestShare_NoRetryForOtherEdits(t *testing.T) {
	notNeeded := `{"summary":"すでにカフェがあります。","patch":[{"op":"test","path":"/entities/0/id","value":"members"}]}`
	svc, f := editSvc(t, warikanWithScope(t, ""), notNeeded)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "LINE で共有できるボタンを付けて"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || !res.Unchanged {
		t.Fatalf("calls=%d unchanged=%v", len(f.calls), res.Unchanged)
	}
}

// #215: "family only" may become public only with the limit said aloud.
func TestShare_FamilyOnlyIsNotSilentPublic(t *testing.T) {
	const note = "限られた人だけで共有することは今はできないため"
	createSvc, _ := newSvc(&fakeCaller{replies: []string{string(warikanWithScope(t, "public"))}})
	created, err := createSvc.Generate(context.Background(), Request{AppID: "c1", Transcript: "家族だけで共有する割り勘アプリを作って"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(created.Summary, note) {
		t.Errorf("create summary: %s", created.Summary)
	}

	svc, f := editSvc(t, warikanWithScope(t, ""), declinedSharePatch, sharePatch)
	edited, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "この割り勘を家族だけで共有して"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || !strings.Contains(edited.Summary, note) {
		t.Errorf("edit calls=%d summary: %s", len(f.calls), edited.Summary)
	}
}

func TestWithShareNote(t *testing.T) {
	private, public := warikanWithScope(t, ""), warikanWithScope(t, "public")
	if got := withShareNote("x。", "割り勘", private, private, LocaleJA); got != "x。" {
		t.Errorf("no change in scope should keep the summary: %s", got)
	}
	if got := withShareNote("x。", "共有して", public, public, LocaleJA); got != "x。" {
		t.Errorf("already shared should keep the summary: %s", got)
	}
	said := "メンバーは誰でも見られます。"
	if got := withShareNote(said, "共有して", private, public, LocaleJA); got != said {
		t.Errorf("already said should not repeat: %s", got)
	}
	if got := withShareNote("家計簿を作りました。", "家族だけで使う家計簿", nil, private, LocaleJA); !strings.Contains(got, "記録は自分だけのままです") {
		t.Errorf("family-only left private should say so: %s", got)
	}
	en := withShareNote("Shared it.", "share this with my family only", private, public, LocaleEN)
	if !strings.Contains(en, "isn't available yet") || !strings.Contains(en, "anyone who opens this app can see") {
		t.Errorf("en: %s", en)
	}
}

// Chat's Pro check reads the same table, so "友達のスマホからも" is not a free
// ready that the generator then makes public (#259).
func TestPaidDetect_UsesShareTable(t *testing.T) {
	for _, in := range []string{"この割り勘を友達のスマホからも入力できるようにして", "この家計簿を家族だけで見たい"} {
		if got := defaultPaidCatalog.Detect(in, LocaleJA); len(got) != 1 || got[0].ID != "sharedData" {
			t.Errorf("%q: %+v", in, got)
		}
	}
	text := "友達のスマホからも入力したい"
	free := defaultPaidCatalog.Without(text, defaultPaidCatalog.Detect(text, LocaleJA), LocaleJA)
	if classifyShareIntent(free) != sharePrivate {
		t.Errorf("free transcript should stay private: %s", free)
	}
}
