package ai

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// reviewBuild mirrors the App Store review build (#245): everything except
// healthStat / locationTracker / nearbyShare / sensorValue / webView (#452 G3) /
// chatThread (#451 G2), no Field.health, no camera, no shared data (#451 G2).
func reviewBuild() ([]string, []string) {
	var allowed []string
	for t := range knownComponentTypes {
		switch t {
		case "healthStat", "locationTracker", "nearbyShare", "sensorValue", "webView", "chatThread":
		default:
			allowed = append(allowed, t)
		}
	}
	return allowed, []string{FeatureFieldHealth, FeatureCameraCapture, FeatureSharedData, "someFutureFlag"}
}

func reviewCtx() context.Context {
	a, d := reviewBuild()
	return withRestriction(context.Background(), NewRestriction(a, d))
}

var removedParts = []string{"healthStat", "locationTracker", "nearbyShare", "sensorValue", "webView", "chatThread"}

// withoutNote cuts the appended restriction note (it names the removed parts).
func withoutNote(s string) string {
	if i := strings.Index(s, "## この依頼の制限"); i >= 0 {
		return s[:i]
	}
	return s
}

func readDoc(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRestriction_NoneIsInactive(t *testing.T) {
	for _, r := range []Restriction{
		NewRestriction(nil, nil),
		NewRestriction([]string{}, []string{"unknownFlag"}),
		NewRestriction([]string{"notAType"}, nil), // no known type: ignored
	} {
		if r.Active() {
			t.Fatalf("should be inactive: %+v", r)
		}
	}
	all := make([]string, 0, len(knownComponentTypes))
	for k := range knownComponentTypes {
		all = append(all, k)
	}
	if NewRestriction(all, nil).Active() {
		t.Fatal("allowing every type is no restriction")
	}
}

func TestRestriction_UnrestrictedPromptIsByteIdentical(t *testing.T) {
	ctx := context.Background()
	for _, p := range []string{systemPrompt, systemPrompt + "\n" + editRules, chatSystemPrompt, chatCreateSystemPrompt, chatEditSystemPrompt, logicSystemPrompt} {
		if restrictPrompt(ctx, p) != p {
			t.Fatal("unrestricted prompt changed")
		}
	}
	if string(restrictSchema(ctx, GenerationSchema())) != string(GenerationSchema()) {
		t.Fatal("unrestricted schema changed")
	}

	kb := kakeibo(t)
	f := &fakeCaller{replies: []string{string(kb)}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(ctx, Request{AppID: "a1", Transcript: "家計簿アプリを作って"})
	if err != nil {
		t.Fatal(err)
	}
	if f.calls[0].System != systemPrompt || string(f.calls[0].Schema) != string(GenerationSchema()) {
		t.Fatal("generate without restriction must send the original prompt/schema")
	}
	if !strings.Contains(string(res.DSL), `"camera"`) || len(res.Meta.Restricted) != 0 {
		t.Fatal("unrestricted output must keep the camera source")
	}
}

func TestRestriction_CatalogAndIndexDropParts(t *testing.T) {
	a, d := reviewBuild()
	as := NewRestriction(a, d).assets()
	for _, p := range removedParts {
		if strings.Contains(as.catalog, p) || strings.Contains(as.index, p) || strings.Contains(string(as.genSch), p) {
			t.Fatalf("%s still in restricted catalog/index/schema", p)
		}
		if !strings.Contains(catalog, p) {
			t.Fatalf("full catalog lost %s", p)
		}
	}
	if strings.Contains(as.index, "health:") || strings.Contains(as.catalog, "health(") {
		t.Fatal("Field.health still described")
	}
	if strings.Contains(as.catalog, "camera|library|both") || !strings.Contains(as.catalog, "source(library)") {
		t.Fatal("camera sources still in the catalog")
	}
	for _, keep := range []string{"photoField", "videoField", "list", "stat", "settlement", "section"} {
		if !strings.Contains(as.catalog, keep) {
			t.Fatalf("allowed part %s missing", keep)
		}
	}
}

func TestRestriction_PromptsDropPartsAndAddNote(t *testing.T) {
	ctx := reviewCtx()
	for name, p := range map[string]string{
		"create": systemPrompt, "edit": systemPrompt + "\n" + editRules,
		"chat": chatSystemPrompt, "chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt,
	} {
		got := restrictPrompt(ctx, p)
		if got == p || !strings.Contains(got, "使えない部品: chatThread, healthStat, locationTracker, nearbyShare") {
			t.Fatalf("%s: restriction note missing", name)
		}
		body := withoutNote(got)
		for _, part := range removedParts {
			if strings.Contains(body, part) {
				i := strings.Index(body, part)
				t.Fatalf("%s: %s still recommended: …%s…", name, part, body[max(0, i-80):min(len(body), i+40)])
			}
		}
		if strings.Contains(body, "source camera") || strings.Contains(body, "source both") {
			t.Fatalf("%s: camera source still recommended", name)
		}
		if restrictPrompt(ctx, p) != got {
			t.Fatalf("%s: restricted prompt not stable", name)
		}
	}
	if s := restrictSchema(ctx, GenerationSchema()); strings.Contains(string(s), "healthStat") {
		t.Fatal("output schema still allows healthStat")
	}
}

func TestRestriction_CreateRetriesOnDisallowedPart(t *testing.T) {
	a, d := reviewBuild()
	health := readDoc(t, "../../../docs/dsl/examples/health.json")
	kb := kakeibo(t)
	f := &fakeCaller{replies: []string{string(health), string(kb)}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿アプリを作って", AllowedComponents: a, DisabledFeatures: d})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 2 || !strings.Contains(f.calls[1].Messages[0].Content, `"healthStat" is not available`) {
		t.Fatalf("expected a retry with the violation: %+v", res.Meta)
	}
	if !strings.Contains(f.calls[0].System, "## この依頼の制限") {
		t.Fatal("restricted prompt not sent")
	}
	if strings.Contains(string(res.DSL), `"camera"`) || !strings.Contains(string(res.DSL), `"library"`) {
		t.Fatalf("camera source not sanitized: %s", res.DSL)
	}
	if len(res.Meta.Restricted) == 0 || res.Mode != "full" {
		t.Fatalf("meta: %+v", res.Meta)
	}
}

func TestRestriction_LastAttemptStripsParts(t *testing.T) {
	a, d := reviewBuild()
	health := readDoc(t, "../../../docs/dsl/examples/health.json")
	f := &fakeCaller{replies: []string{string(health)}}
	svc, _ := newSvc(f)
	svc.MaxRegenerations = -1
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "体重記録アプリを作って", AllowedComponents: a, DisabledFeatures: d})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.DSL), "healthStat") || strings.Contains(string(res.DSL), `"health"`) {
		t.Fatalf("disallowed parts left: %s", res.DSL)
	}
	if _, err := dsl.Parse(res.DSL); err != nil {
		t.Fatalf("stripped DSL invalid: %v", err)
	}
}

func TestRestriction_EditRemovesExistingAndRejectsNew(t *testing.T) {
	a, d := reviewBuild()
	health := readDoc(t, "../../../docs/dsl/examples/health.json")
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "a1", 1, health, appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	addHealth := `{"patch":[{"op":"add","path":"/screens/0/components/-","value":{"type":"healthStat","metric":"steps","label":"歩数2"}}],"summary":"歩数を足しました"}`
	rename := `{"patch":[{"op":"replace","path":"/app/name","value":"からだメモ"}],"summary":"名前を変えました"}`
	f := &fakeCaller{replies: []string{addHealth, rename}}
	svc := &Service{Claude: f, Store: store, Model: "test-model"}
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Mode: ModeEdit, Transcript: "名前をからだメモにして", AllowedComponents: a, DisabledFeatures: d})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 2 || !strings.Contains(f.calls[1].Messages[0].Content, `"healthStat" is not available`) {
		t.Fatalf("new healthStat should be rejected: %+v", res.Meta)
	}
	if res.Mode != "full" || strings.Contains(string(res.DSL), "healthStat") || !strings.Contains(string(res.DSL), "からだメモ") {
		t.Fatalf("existing healthStat should be removed and saved full: mode=%s %s", res.Mode, res.DSL)
	}
	if res.Summary != "名前を変えました" {
		t.Fatalf("edit summary kept: %q", res.Summary)
	}
}

func TestRestriction_MainUtterancesPass(t *testing.T) {
	a, d := reviewBuild()
	cases := map[string]string{
		"家計簿":  "examples/kakeibo.json",
		"割り勘":  "examples/warikan.json",
		"お小遣い": "examples/okozukai.json",
		"日記":   "../../../docs/dsl/examples/diary_ai.json",
		"TODO": "../../../docs/dsl/examples/shopping.json",
		"習慣":   "../../../docs/dsl/examples/water.json",
		"筋トレ":  "../../../docs/dsl/examples/training.json",
	}
	for name, path := range cases {
		doc := readDoc(t, path)
		f := &fakeCaller{replies: []string{string(doc)}}
		svc, _ := newSvc(f)
		res, err := svc.Generate(context.Background(), Request{AppID: "a1", Mode: ModeCreate, Transcript: name + "アプリを作って", AllowedComponents: a, DisabledFeatures: d})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.Meta.Attempts != 1 {
			t.Fatalf("%s: unexpected retry: %v", name, res.Meta.LastProblems)
		}
		for _, p := range removedParts {
			if strings.Contains(string(res.DSL), p) {
				t.Fatalf("%s: %s in output", name, p)
			}
		}
		if strings.Contains(string(res.DSL), `"source":"camera"`) || strings.Contains(string(res.DSL), `"source":"both"`) {
			t.Fatalf("%s: camera source in output", name)
		}
	}
}

func TestRestriction_ChatUsesRestrictedPrompt(t *testing.T) {
	a, d := reviewBuild()
	svc, f := newChatSvc(questionReply, questionReply)
	if _, err := svc.Chat(context.Background(), ChatRequest{AppID: "c1", Messages: user("家計簿")}); err != nil {
		t.Fatal(err)
	}
	if f.calls[0].System != chatSystemPrompt {
		t.Fatal("unrestricted chat prompt changed")
	}
	if _, err := svc.Chat(context.Background(), ChatRequest{AppID: "c2", Messages: user("家計簿"), AllowedComponents: a, DisabledFeatures: d}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls[1].System, "使えない部品: chatThread, healthStat") {
		t.Fatal("restricted chat prompt not sent")
	}
}

// #452 G3: without webView, sites go to openUrlButton and the share / maps
// guidance on the same rule line survives.
func TestRestriction_NoWebViewRoutesSitesToOpenURL(t *testing.T) {
	got := restrictPrompt(reviewCtx(), systemPrompt)
	for _, want := range []string{"「○○のサイト/ニュースを見たい」は openUrlButton", "「LINE/Slack/Discord で送りたい」は shareButton"} {
		if !strings.Contains(got, want) {
			t.Fatalf("restricted prompt lost %q", want)
		}
	}
	if chat := withoutNote(restrictPrompt(reviewCtx(), chatEditSystemPrompt)); !strings.Contains(chat, "openUrlButton でそのサイトを Safari で開く") {
		t.Fatal("chat limits still suggest webView")
	}
}
