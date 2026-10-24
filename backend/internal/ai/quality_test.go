package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// withoutMonthFilter drops the thisMonth filter of the 今月の支出 stat (#87-1).
func withoutMonthFilter(raw []byte) string {
	return strings.Replace(string(raw), `"format": "currency",
              "filter": { "field": "date", "op": "thisMonth", "value": null } }`, `"format": "currency" }`, 1)
}

func TestCreate_LintRetryOnceThenClean(t *testing.T) {
	kb := kakeibo(t)
	bad := withoutMonthFilter(kb)
	if bad == string(kb) {
		t.Fatal("fixture did not change")
	}
	f := &fakeCaller{replies: []string{bad, string(kb)}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 2 || !res.Meta.LintRetried || len(res.Meta.Lint) != 0 {
		t.Fatalf("meta: %+v", res.Meta)
	}
	p := f.calls[1].Messages[0].Content
	if !strings.Contains(p, "中身に以下の問題") || !strings.Contains(p, `"今月"`) {
		t.Fatalf("retry prompt should carry the lint warning: %s", p)
	}
	if !strings.Contains(string(res.DSL), "thisMonth") {
		t.Fatal("expected the fixed document")
	}
}

func TestCreate_LintRetryKeepsFirstWhenRetryBreaks(t *testing.T) {
	kb := kakeibo(t)
	bad := withoutMonthFilter(kb)
	f := &fakeCaller{replies: []string{bad, withBadEntity(t, kb), string(kb)}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿"})
	if err != nil {
		t.Fatal(err)
	}
	// no third call: the lint retry is best effort
	if res.Meta.Attempts != 2 || len(res.Meta.Lint) == 0 || res.Meta.AreaFallback {
		t.Fatalf("meta: %+v", res.Meta)
	}
	if strings.Contains(string(res.DSL), "ghost") {
		t.Fatal("must return the first valid output")
	}
}

func TestCreate_LintStillPresentAfterRetryIsAccepted(t *testing.T) {
	bad := withoutMonthFilter(kakeibo(t))
	f := &fakeCaller{replies: []string{bad, bad}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 2 || len(res.Meta.Lint) != 1 {
		t.Fatalf("meta: %+v", res.Meta)
	}
}

func TestEdit_LintOnlyForWarningsThePatchAdds(t *testing.T) {
	ctx := context.Background()
	// the stored app already has a lint warning; a harmless patch must not retry
	store := appconfigclient.NewMemory()
	if _, err := store.Put(ctx, "a1", 1, []byte(withoutMonthFilter(kakeibo(t))), appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	ok := `{"summary":"カテゴリにカフェを追加しました。","patch":[{"op":"add","path":"/entities/0/fields/1/options/-","value":"カフェ"}]}`
	f := &fakeCaller{replies: []string{ok}}
	svc := &Service{Claude: f, Store: store, Model: "m"}
	res, err := svc.Generate(ctx, Request{AppID: "a1", Transcript: "カテゴリにカフェを追加して"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 1 || res.Meta.LintRetried {
		t.Fatalf("meta: %+v", res.Meta)
	}

	// a patch that adds a 今日 stat without a filter gets one retry
	store2 := appconfigclient.NewMemory()
	if _, err := store2.Put(ctx, "a1", 1, kakeibo(t), appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	add := func(filter string) string {
		return `{"summary":"今日の支出を追加しました。","patch":[{"op":"add","path":"/screens/0/components/0/children/-","value":{"type":"stat","label":"今日の支出","entity":"expense","field":"amount"` + filter + `}}]}`
	}
	f2 := &fakeCaller{replies: []string{add(""), add(`,"filter":{"field":"date","op":"today"}`)}}
	svc2 := &Service{Claude: f2, Store: store2, Model: "m"}
	res, err = svc2.Generate(ctx, Request{AppID: "a1", Transcript: "今日の支出も出して"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 2 || !res.Meta.LintRetried || len(res.Meta.Lint) != 0 {
		t.Fatalf("meta: %+v", res.Meta)
	}
}

func TestNewLintIgnoresIndexShifts(t *testing.T) {
	before := []string{`screen "home" components[1]: label "今月" x`}
	after := []string{`screen "home" components[2]: label "今月" x`, `entity "e" has no list`}
	got := newLint(before, after)
	if len(got) != 1 || got[0] != `entity "e" has no list` {
		t.Fatalf("got %v", got)
	}
}

func TestDropFormPhotoPreviews(t *testing.T) {
	in := `{"screens":[
	 {"id":"form","entity":"spot","components":[
	   {"type":"section","children":[{"type":"textField","field":"name"},{"type":"image","field":"photo"}]},
	   {"type":"photoField","field":"photo","area":"A1:F3"},
	   {"type":"image","field":"photo","area":"A4:F6"}]},
	 {"id":"detail","components":[{"type":"image","field":"photo"}]}]}`
	var doc struct {
		Screens []struct {
			Components []map[string]any `json:"components"`
		} `json:"screens"`
	}
	if err := json.Unmarshal(dropFormPhotoPreviews([]byte(in)), &doc); err != nil {
		t.Fatal(err)
	}
	form := doc.Screens[0].Components
	if len(form) != 2 || form[1]["type"] != "photoField" || len(form[0]["children"].([]any)) != 1 {
		t.Fatalf("form: %v", form)
	}
	if len(doc.Screens[1].Components) != 1 {
		t.Fatal("non-form screens are untouched")
	}
	// nothing to drop -> same bytes
	kb := kakeibo(t)
	if string(dropFormPhotoPreviews(kb)) != string(kb) {
		t.Fatal("unchanged documents must keep their bytes")
	}
}

func TestNormalizeDigits(t *testing.T) {
	if got := normalizeDigits("ホームとグラフの２タブ、１０件"); got != "ホームとグラフの2タブ、10件" {
		t.Fatal(got)
	}
}

func TestLintExampleFewShotsClean(t *testing.T) {
	for _, raw := range []string{fewShotKakeibo, fewShotExpenseEN, fewShotWarikan} {
		app, err := dsl.Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if w := app.Lint(); len(w) > 0 {
			t.Errorf("few-shot must be lint-clean (Claude copies it): %v", w)
		}
	}
}
