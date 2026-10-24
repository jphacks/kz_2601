package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// fakeCaller replays scripted responses and records prompts.
type fakeCaller struct {
	replies []string
	calls   []Completion
}

func (f *fakeCaller) Complete(_ context.Context, req Completion) (*Response, error) {
	f.calls = append(f.calls, req)
	if len(f.replies) == 0 {
		return nil, &APIError{Status: 500, Type: "no_more_replies"}
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return &Response{Text: r, StopReason: "end_turn", Usage: Usage{InputTokens: 10, OutputTokens: 5}}, nil
}

func kakeibo(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("examples/kakeibo.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// withOverlap moves the add button onto the summary card (A1:F2 overlap).
func withOverlap(t *testing.T, raw []byte) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	comps := doc["screens"].([]any)[0].(map[string]any)["components"].([]any)
	comps[1].(map[string]any)["area"] = "A1:C2"
	out, _ := json.Marshal(doc)
	return string(out)
}

// withBadEntity makes the list reference a missing entity (area-strip cannot fix it).
func withBadEntity(t *testing.T, raw []byte) string {
	t.Helper()
	return strings.Replace(string(raw), `"type": "list", "entity": "expense"`, `"type": "list", "entity": "ghost"`, 1)
}

func newSvc(f *fakeCaller) (*Service, *appconfigclient.Memory) {
	store := appconfigclient.NewMemory()
	return &Service{Claude: f, Store: store, Model: "test-model"}, store
}

func TestCreate_RegeneratesOnProblemsThenSucceeds(t *testing.T) {
	kb := kakeibo(t)
	f := &fakeCaller{replies: []string{withOverlap(t, kb), string(kb)}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿アプリを作って"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Mode != "full" || res.Version != 2 || res.PreviousVersion != 1 {
		t.Fatalf("mode/version: %+v", res)
	}
	if res.Meta.Attempts != 2 || res.Meta.Regenerations != 1 || res.Meta.AreaFallback {
		t.Fatalf("meta: %+v", res.Meta)
	}
	if !strings.Contains(f.calls[1].Messages[0].Content, "overlaps") {
		t.Fatalf("second prompt should carry the problems: %s", f.calls[1].Messages[0].Content)
	}
	if res.Summary != "家計簿アプリを作りました。ホームと集計の2タブで、右下のボタンから支出を追加できます。" {
		t.Fatalf("summary: %s", res.Summary)
	}
	if res.Meta.Usage.InputTokens != 20 || res.Meta.Usage.OutputTokens != 10 {
		t.Fatalf("usage not accumulated: %+v", res.Meta.Usage)
	}
}

func TestCreate_AreaFallbackAfterMaxRegenerations(t *testing.T) {
	kb := kakeibo(t)
	bad := withOverlap(t, kb)
	f := &fakeCaller{replies: []string{bad, bad, bad}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Meta.Attempts != 3 || !res.Meta.AreaFallback {
		t.Fatalf("meta: %+v", res.Meta)
	}
	if strings.Contains(string(res.DSL), `"area"`) {
		t.Fatal("areas should have been stripped")
	}
	if _, err := dsl.Parse(res.DSL); err != nil {
		t.Fatal(err)
	}
}

func TestCreate_FailsWithProblemsWhenUnfixable(t *testing.T) {
	kb := kakeibo(t)
	bad := withBadEntity(t, kb)
	f := &fakeCaller{replies: []string{bad, bad, bad}}
	svc, _ := newSvc(f)
	_, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿"})
	var ge *Error
	if !errors.As(err, &ge) || ge.Status != http.StatusBadRequest || ge.Code != "generation_failed" {
		t.Fatalf("want 400 generation_failed, got %v", err)
	}
	if ge.Attempts != 3 || len(ge.Problems) == 0 || !strings.Contains(ge.Problems[0], "ghost") {
		t.Fatalf("problems: %+v", ge)
	}
}

func TestEdit_PatchAppliedAndSummaryFromModel(t *testing.T) {
	kb := kakeibo(t)
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "a1", 1, kb, appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	badPatch := `{"patch":[{"op":"add","path":"/entities/9/fields/-","value":"カフェ"}],"summary":"x"}`
	goodPatch := `{"patch":[{"op":"add","path":"/entities/0/fields/1/options/-","value":"カフェ"},{"op":"replace","path":"/screens/0/components/0/area","value":"A1:F4"},{"op":"replace","path":"/screens/0/components/2/area","value":"A5:F10"}],"summary":"カテゴリにカフェを追加し、合計を大きくしました"}`
	f := &fakeCaller{replies: []string{badPatch, goodPatch}}
	svc := &Service{Claude: f, Store: store, Model: "test-model"}
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "カテゴリにカフェを追加して"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Mode != "patch" || len(res.Patch) != 3 || res.Version != 3 || res.PreviousVersion != 2 {
		t.Fatalf("result: mode=%s patch=%d v=%d prev=%d", res.Mode, len(res.Patch), res.Version, res.PreviousVersion)
	}
	if res.Meta.Regenerations != 1 || !strings.Contains(f.calls[1].Messages[0].Content, "apply patch") {
		t.Fatalf("retry prompt: %+v / %s", res.Meta, f.calls[1].Messages[0].Content)
	}
	if !strings.Contains(string(res.DSL), "カフェ") || res.Summary != "カテゴリにカフェを追加し、合計を大きくしました" {
		t.Fatalf("dsl/summary: %s", res.Summary)
	}
	if !strings.Contains(f.calls[0].System, "JSON Patch") || !strings.Contains(f.calls[0].Messages[0].Content, "version 2") {
		t.Fatal("edit prompt should include patch rules and current version")
	}
}

func TestEdit_DowngradesToFullWhenPatchKeepsFailing(t *testing.T) {
	kb := kakeibo(t)
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "a1", 1, kb, appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	badPatch := `{"patch":[{"op":"add","path":"/entities/9/fields/-","value":"x"}],"summary":"x"}`
	f := &fakeCaller{replies: []string{badPatch, badPatch, badPatch, string(kb)}}
	svc := &Service{Claude: f, Store: store, Model: "test-model"}
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "なにか変えて"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Mode != "full" || !res.Meta.DowngradedToFull || res.Meta.Attempts != 4 {
		t.Fatalf("result: %+v", res.Meta)
	}
}

func TestModeResolution(t *testing.T) {
	kb := kakeibo(t)
	store := appconfigclient.NewMemory()
	f := &fakeCaller{replies: []string{string(kb)}}
	svc := &Service{Claude: f, Store: store, Model: "m"}
	if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls[0].Messages[0].Content, "## 今回の指示") || strings.Contains(f.calls[0].System, "JSON Patch") {
		t.Fatal("initial app must use create prompt")
	}
	// Explicit create on a non-initial app includes the current DSL as reference.
	f.replies = []string{string(kb)}
	if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "別のアプリにして", Mode: ModeCreate}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls[1].Messages[0].Content, "参考: 現在のアプリ") {
		t.Fatal("create over existing app should include reference DSL")
	}
	if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "x", Mode: "weird"}); err == nil {
		t.Fatal("unknown mode must fail")
	}
	if _, err := svc.Generate(context.Background(), Request{AppID: "", Transcript: "x"}); err == nil {
		t.Fatal("missing appId must fail")
	}
}
