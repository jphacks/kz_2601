package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
)

// proSet is a paid.ProChecker over a map (true = Pro).
type proSet map[string]bool

func (p proSet) IsPro(_ context.Context, uid string) bool { return p[uid] }

func asProRequired(t *testing.T, err error, ids ...string) {
	t.Helper()
	var ge *Error
	if !errors.As(err, &ge) || ge.Status != http.StatusPaymentRequired || ge.Code != httpx.CodeProRequired {
		t.Fatalf("want 402 pro_required, got %v", err)
	}
	fs, _ := ge.Details["features"].([]PaidFeature)
	if strings.Join(paid.IDs(fs), ",") != strings.Join(ids, ",") {
		t.Fatalf("features %+v, want %v", fs, ids)
	}
}

// #316: the generated DSL decides. Rules for a caller without Pro are not
// saved (402); with Pro they are.
func TestGenerate_PaidFeatureNeedsPro(t *testing.T) {
	for _, c := range []struct {
		uid string
		pro bool
	}{{"free", false}, {"pro", true}} {
		f := &fakeCaller{replies: []string{waterApp, waterRules}}
		svc, store := newSvc(f)
		svc.ServerRulesPass = true
		svc.Pro = proSet{"pro": true}
		ctx := auth.ContextWithUID(context.Background(), c.uid)
		res, err := svc.Generate(ctx, Request{AppID: "pg-" + c.uid, Transcript: "水分の記録。毎朝7時に水分の記録を促して"})
		env, _ := store.Get(context.Background(), "pg-"+c.uid)
		if !c.pro {
			asProRequired(t, err, paid.ServerRules)
			if env.Version != 1 {
				t.Fatalf("nothing saved without Pro: version %d", env.Version)
			}
			continue
		}
		if err != nil || res.Version != 2 || !strings.Contains(string(res.DSL), `"serverRules"`) {
			t.Fatalf("pro: %v %+v", err, res)
		}
	}
}

// An app without paid features is free; an app that already has one keeps it
// on a later edit (a lapsed Pro edits their app).
func TestGenerate_FreeAppAndExistingFeature(t *testing.T) {
	f := &fakeCaller{replies: []string{string(kakeibo(t))}}
	svc, store := newSvc(f)
	svc.Pro = proSet{}
	ctx := auth.ContextWithUID(context.Background(), "free")
	if _, err := svc.Generate(ctx, Request{AppID: "pg-free", Transcript: "家計簿アプリを作って"}); err != nil {
		t.Fatalf("free app: %v", err)
	}

	if _, err := store.Put(context.Background(), "pg-lapsed", 0, sharedKakeibo(t), appconfigclient.Source{Kind: "seed"}); err != nil {
		t.Fatal(err)
	}
	edit := `{"patch":[{"op":"replace","path":"/app/name","value":"みんなの家計簿"}],"summary":"名前を変えました。"}`
	svc2, _ := newSvc(&fakeCaller{replies: []string{edit}})
	svc2.Store, svc2.Pro = store, proSet{}
	if _, err := svc2.Generate(ctx, Request{AppID: "pg-lapsed", Transcript: "名前をみんなの家計簿にして", Mode: ModeEdit}); err != nil {
		t.Fatalf("edit of an app that already shares: %v", err)
	}
}

// The App Store review build has no purchases (and no shared data): no
// server rules are generated for it, so its users never meet the 402.
func TestServerRulesPass_SkippedForReviewBuildAndFreeVersion(t *testing.T) {
	for name, req := range map[string]Request{
		"review build": {Transcript: "水分の記録。毎朝7時に水分の記録を促して", DisabledFeatures: []string{FeatureSharedData}},
		"free version": {Transcript: defaultPaidCatalog.Without("水分の記録。毎朝7時に水分の記録を促して", []PaidFeature{{ID: paid.ServerRules}}, LocaleJA)},
	} {
		f := &fakeCaller{replies: []string{waterApp, waterRules}}
		svc, _ := newSvc(f)
		svc.ServerRulesPass = true
		svc.Pro = proSet{}
		req.AppID = "pg-" + strings.ReplaceAll(name, " ", "-")
		res, err := svc.Generate(auth.ContextWithUID(context.Background(), "free"), req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(f.calls) != 1 || res.Meta.ServerRules != nil || strings.Contains(string(res.DSL), "serverRules") {
			t.Fatalf("%s: the pass must not run (%d calls)", name, len(f.calls))
		}
	}
}

// failingStore answers every save with appconfig's 402.
type failingStore struct{ *appconfigclient.Memory }

func (s failingStore) Put(context.Context, string, int, json.RawMessage, appconfigclient.Source) (*appconfigclient.Envelope, error) {
	return nil, &appconfigclient.Error{Status: http.StatusPaymentRequired, Code: httpx.CodeProRequired, Message: "this needs Tailor It Pro: sharedData",
		Details: map[string]any{"features": []any{map[string]any{"id": "sharedData", "title": "みんなで共有するデータ", "reason": "r"}}}}
}

// appconfig enforces too (ai without a checker, or a changed client): its 402
// reaches the caller with details.features.
func TestGenerate_RelaysAppconfigProRequired(t *testing.T) {
	svc, _ := newSvc(&fakeCaller{replies: []string{string(kakeibo(t))}})
	svc.Store = failingStore{appconfigclient.NewMemory()}
	_, err := svc.Generate(context.Background(), Request{AppID: "pg-relay", Transcript: "家計簿アプリを作って"})
	ge := asError(err)
	if ge.Status != http.StatusPaymentRequired || ge.Code != httpx.CodeProRequired {
		t.Fatalf("relay: %v", err)
	}
	body, _ := json.Marshal(svc.errBody(context.Background(), ge))
	if !strings.Contains(string(body), `"details":{"features":[{"id":"sharedData"`) {
		t.Fatalf("body: %s", body)
	}
}

// POST /v1/text is the online AI of an app: Pro only, Claude not called otherwise.
func TestText_ProRequired(t *testing.T) {
	svc, fc := textSvc(`{"text":"がんばったね"}`)
	svc.Pro = proSet{"pro-user": true}
	h := NewHandler(svc)
	rec := postText(t, h, "free-user", `{"instruction":"励ましの一言","locale":"en"}`)
	if rec.Code != http.StatusPaymentRequired || errCode(t, rec) != httpx.CodeProRequired || len(fc.calls) != 0 {
		t.Fatalf("free: %d %s (calls %d)", rec.Code, rec.Body.String(), len(fc.calls))
	}
	if !strings.Contains(rec.Body.String(), `"id":"onlineAI","title":"Online AI in your apps"`) {
		t.Fatalf("details: %s", rec.Body.String())
	}
	if rec := postText(t, h, "pro-user", `{"instruction":"励ましの一言"}`); rec.Code != http.StatusOK {
		t.Fatalf("pro: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPaidCatalog_DetectServerRules(t *testing.T) {
	for text, want := range map[string]bool{
		"出欠がついたら参加者に通知するアプリ":                    true,
		"メンバーが記録したら自動で数えて":                      true,
		"毎朝7時にリマインドする日記":                        false, // 端末の通知で足りる
		"体重の記録。通知はいらない":                         false,
		"Notify the members when someone posts": true,
	} {
		got := false
		for _, f := range defaultPaidCatalog.Detect(text, LocaleJA) {
			got = got || f.ID == paid.ServerRules
		}
		if got != want {
			t.Errorf("Detect(%q) serverRules = %v, want %v", text, got, want)
		}
	}
	// the free version's own instruction is not read as asking for it
	free := defaultPaidCatalog.Without("参加者に通知する出欠表", []PaidFeature{{ID: paid.ServerRules}}, LocaleJA)
	if !defaultPaidCatalog.dropped(free, paid.ServerRules) {
		t.Fatal("dropped")
	}
	if got := defaultPaidCatalog.Detect(strings.Replace(free, "参加者に通知する", "", 1), LocaleJA); len(got) != 0 {
		t.Fatalf("withoutInstruction detected as paid: %+v", got)
	}
}
