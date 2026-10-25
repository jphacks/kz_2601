package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestPaidCatalog_DefaultHasSharedDataOnly(t *testing.T) {
	if len(defaultPaidCatalog.features) != 1 || defaultPaidCatalog.features[0].ID != "sharedData" {
		t.Fatalf("初期の有料リストは sharedData だけ: %+v", defaultPaidCatalog.features)
	}
}

func TestPaidCatalog_Detect(t *testing.T) {
	c := defaultPaidCatalog
	cases := []struct {
		text string
		want bool
	}{
		{"みんなで書き込める掲示板アプリ", true},
		{"家族で共有する買い物リスト", true},
		{"友達と投稿し合う写真日記", true},
		{"A leaderboard app for everyone in my club", true},
		{"毎日の支出を記録する家計簿", false},
		{"共有ボタン付きの日記", false}, // iOS の共有シート(シェア)は有料ではない
		{"体重を記録してグラフで見る。共有はしない、共有せずに自分だけ", false},
		{"A habit tracker with a share sheet", false},
	}
	for _, tc := range cases {
		got := c.Detect(tc.text, LocaleJA)
		if (len(got) > 0) != tc.want {
			t.Errorf("Detect(%q) = %+v, want paid=%v", tc.text, got, tc.want)
		}
	}
	if got := c.Detect("Everyone posts on a shared board", LocaleEN); len(got) != 1 || got[0].Title != "Shared data" || got[0].Reason == "" {
		t.Fatalf("en title/reason: %+v", got)
	}
}

func TestPaidCatalog_ParseEditable(t *testing.T) {
	raw := `{"features":[
	  {"id":"sharedData","enabled":false,"keywords":["共有"]},
	  {"id":"externalApi","enabled":true,"title":{"ja":"外部 API"},"keywords":["天気"],"withoutInstruction":{"ja":"(外部 API は使わない)"}}]}`
	c, err := ParsePaidCatalog([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Detect("みんなで共有", LocaleJA); len(got) != 0 {
		t.Fatalf("enabled=false は判定しない: %+v", got)
	}
	got := c.Detect("天気を表示する", LocaleJA)
	if len(got) != 1 || got[0].ID != "externalApi" {
		t.Fatalf("足した機能を判定する: %+v", got)
	}
	if w := c.Without("天気を表示する", got, LocaleEN); w != "天気を表示する\n(外部 API は使わない)" {
		t.Fatalf("en が無ければ ja: %q", w)
	}
	if _, err := ParsePaidCatalog([]byte(`{"features":[{"id":"a"},{"id":"a"}]}`)); err == nil {
		t.Fatal("重複 id はエラー")
	}
}

func sharedKakeibo(t *testing.T) []byte {
	t.Helper()
	app, err := dsl.Parse(kakeibo(t))
	if err != nil {
		t.Fatal(err)
	}
	app.Entities[0].Scope = dsl.ScopePublic
	raw, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPaidCatalog_InApp(t *testing.T) {
	app, _ := dsl.Parse(kakeibo(t))
	if got := defaultPaidCatalog.InApp(app, LocaleJA); len(got) != 0 {
		t.Fatalf("private のみ: %+v", got)
	}
	shared, err := dsl.Parse(sharedKakeibo(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := defaultPaidCatalog.InApp(shared, LocaleJA); len(got) != 1 || got[0].ID != "sharedData" {
		t.Fatalf("public entity: %+v", got)
	}
}

const createReadyShared = `{"kind":"ready","text":"この内容で作りますね。","suggestions":[],"options":[],
  "brief":{"name":"サークル掲示板","purpose":"みんなで予定を書き込む","entities":["投稿"],"features":["一覧"],"native":[]},
  "transcript":"サークルのみんなで予定を書き込める掲示板アプリ"}`

const createReadyPrivate = `{"kind":"ready","text":"この内容で作りますね。","suggestions":[],"options":[],
  "brief":{"name":"家計簿","purpose":"支出の記録","entities":["支出"],"features":["月の合計"],"native":[]},
  "transcript":"毎日の支出を記録する家計簿アプリ"}`

const editProposalShared = `{"kind":"proposal","text":"","suggestions":[],"options":[
  {"title":"みんなで使う","detail":"","instruction":"支出を家族で共有して書き込めるようにして"},
  {"title":"グラフ","detail":"","instruction":"ホームにカテゴリ別の円グラフを追加して"}],"transcript":""}`

func TestChat_ReadyRequiresProForSharedData(t *testing.T) {
	svc, _ := newSvc(&fakeCaller{replies: []string{createReadyShared}})
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "n1", Mode: "create", Messages: user("みんなで使う掲示板")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "ready" || !r.RequiresPro || len(r.PaidFeatures) != 1 || r.PaidFeatures[0].ID != "sharedData" {
		t.Fatalf("ready: %+v", r)
	}
	if !strings.HasPrefix(r.FreeTranscript, r.Transcript+"\n") || !strings.Contains(r.FreeTranscript, "private") {
		t.Fatalf("freeTranscript: %q", r.FreeTranscript)
	}
	body, _ := json.Marshal(r)
	if !strings.Contains(string(body), `"requiresPro":true`) || !strings.Contains(string(body), `"paidFeatures":[{"id":"sharedData"`) {
		t.Fatalf("json: %s", body)
	}
}

func TestChat_ReadyFreeHasNoPaidFields(t *testing.T) {
	svc, _ := newSvc(&fakeCaller{replies: []string{createReadyPrivate}})
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "n1", Mode: "create", Messages: user("家計簿")})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(r)
	if r.RequiresPro || strings.Contains(string(body), "requiresPro") || strings.Contains(string(body), "freeTranscript") {
		t.Fatalf("無料なら追加フィールドは出さない(旧クライアントと同じ JSON): %s", body)
	}
}

func TestChat_ClarifyReadyAlsoAnnotated(t *testing.T) {
	reply := `{"kind":"ready","text":"作ります","suggestions":[],"brief":{"name":"x","purpose":"","entities":[],"features":[],"native":[]},"transcript":"チームで共有するタスク表"}`
	svc, _ := newSvc(&fakeCaller{replies: []string{reply}})
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "n1", Messages: user("タスク表")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "ready" || !r.RequiresPro {
		t.Fatalf("旧 clarify の ready: %+v", r)
	}
}

func TestChat_ProposalOptionsAnnotatedPerIdea(t *testing.T) {
	svc, _ := newEditChatSvc(t, editProposalShared)
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Mode: "edit", Messages: user("便利にして")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "proposal" || len(r.Options) != 2 {
		t.Fatalf("proposal: %+v", r)
	}
	if o := r.Options[0]; !o.RequiresPro || o.FreeInstruction == "" || o.PaidFeatures[0].ID != "sharedData" {
		t.Fatalf("案1 は有料: %+v", o)
	}
	if o := r.Options[1]; o.RequiresPro || o.FreeInstruction != "" {
		t.Fatalf("案2 は無料: %+v", o)
	}
	if r.RequiresPro {
		t.Fatal("proposal は案ごとに付ける")
	}
}

func TestChat_EditOfAlreadySharedAppDoesNotAskAgain(t *testing.T) {
	f := &fakeCaller{replies: []string{editProposalShared}}
	svc, store := newSvc(f)
	if _, err := store.Put(context.Background(), "s1", 0, sharedKakeibo(t), appconfigclient.Source{Kind: "seed"}); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "s1", Mode: "edit", Messages: user("便利にして")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Options[0].RequiresPro {
		t.Fatalf("既に共有のアプリの改変では聞かない: %+v", r.Options[0])
	}
}
