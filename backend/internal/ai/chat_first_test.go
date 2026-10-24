package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

const (
	editAnswerReply   = `{"kind":"answer","text":"いいえ、WebView ではなく iOS のネイティブ画面です。","suggestions":[],"options":[],"transcript":""}`
	editProposalReply = `{"kind":"proposal","text":"","suggestions":[],"options":[
	  {"title":" 数字を大きく ","detail":"今月の支出を大きく表示","instruction":"ホームの今月の支出の枠を広げて大きくして"},
	  {"title":"","detail":"x","instruction":"タイトルなしは捨てる"},
	  {"title":"グラフで見せる","detail":"ホームに円グラフ","instruction":"ホームにカテゴリ別の円グラフを追加して"},
	  {"title":"整理する","detail":"件数を消す","instruction":"ホームの今月の件数を消して"},
	  {"title":"4 個目","detail":"x","instruction":"上限で捨てる"}],"transcript":""}`
	editReadyReply = `{"kind":"ready","text":"メモ欄を消しますね。","suggestions":[],"options":[],"transcript":"支出を記録画面のメモ欄を消して。"}`
)

// newEditChatSvc seeds a1 with the kakeibo example so auto mode picks edit.
func newEditChatSvc(t *testing.T, replies ...string) (*Service, *fakeCaller) {
	t.Helper()
	f := &fakeCaller{replies: replies}
	svc, store := newSvc(f)
	if _, err := store.Put(context.Background(), "a1", 0, kakeibo(t), appconfigclient.Source{Kind: "seed"}); err != nil {
		t.Fatal(err)
	}
	return svc, f
}

func TestChatFirst_EditAutoAnswer(t *testing.T) {
	svc, f := newEditChatSvc(t, editAnswerReply)
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Messages: user("これ WebView?"),
		Capabilities: []string{"answer", "proposal"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "answer" || r.Mode != "edit" || !strings.Contains(r.Text, "ネイティブ") || r.Transcript != "" {
		t.Fatalf("answer: %+v", r)
	}
	c := f.calls[0]
	if string(c.Schema) != string(ChatEditSchema()) || c.System != chatEditSystemPrompt || c.MaxTokens != chatFirstMaxTokens {
		t.Fatalf("edit call setup: schema=%s", c.Schema)
	}
	p := c.Messages[0].Content
	for _, want := range []string{"## 今のアプリ", "アプリ名: 家計簿", "今月の支出", "タブ: ホーム(画面 home) / 集計(画面 stats)", "ユーザー: これ WebView?"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt lacks %q:\n%s", want, p)
		}
	}
}

func TestChatFirst_ProposalOptionsCleaned(t *testing.T) {
	svc, f := newEditChatSvc(t, editProposalReply, editReadyReply)
	ctx := context.Background()
	r, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Mode: "edit", Messages: user("見やすくして")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "proposal" || r.Text != "こんな案はどうでしょう。" || len(r.Options) != 3 {
		t.Fatalf("proposal: %+v", r)
	}
	if o := r.Options[0]; o.ID != "opt1" || o.Title != "数字を大きく" || o.Instruction == "" {
		t.Fatalf("option 1: %+v", o)
	}
	if r.Options[1].Title != "グラフで見せる" || r.Options[2].ID != "opt3" {
		t.Fatalf("options: %+v", r.Options)
	}
	// the ideas stay in the history for 「2 番目で」
	if _, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Mode: "edit", Messages: user("メモ欄を消して")}); err != nil {
		t.Fatal(err)
	}
	if p := f.calls[1].Messages[0].Content; !strings.Contains(p, "案2「グラフで見せる」") {
		t.Fatalf("proposal not in history:\n%s", p)
	}
}

func TestChatFirst_EditReady(t *testing.T) {
	svc, _ := newEditChatSvc(t, editReadyReply, `{"kind":"ready","text":"","suggestions":[],"options":[],"transcript":""}`)
	ctx := context.Background()
	r, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Mode: "edit", Messages: user("メモ欄を消して")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "ready" || r.Transcript != "支出を記録画面のメモ欄を消して。" || r.Brief != nil {
		t.Fatalf("ready: %+v", r)
	}
	// empty transcript -> the utterance itself; empty text -> the default line
	r, err = svc.Chat(ctx, ChatRequest{AppID: "a1", Mode: "edit", Messages: user("金額を大きくして")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Transcript != "金額を大きくして" || r.Text != "では変えますね。" {
		t.Fatalf("fallbacks: %+v", r)
	}
}

func TestChatFirst_ProposalWithoutOptionsBecomesAnswer(t *testing.T) {
	svc, _ := newEditChatSvc(t, `{"kind":"proposal","text":"共有するなら公開にできます。","suggestions":[],"options":[{"title":"x","detail":"","instruction":" "}],"transcript":""}`)
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Mode: "edit", Messages: user("家族でも使いたい")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "answer" || len(r.Options) != 0 || r.Text == "" {
		t.Fatalf("fallback: %+v", r)
	}
}

func TestChatFirst_CreateWhenInitialAndSchema(t *testing.T) {
	f := &fakeCaller{replies: []string{`{"kind":"answer","text":"記録アプリなら作れます。","suggestions":["家計簿","日記"],"options":[],
	  "brief":{"name":"","purpose":"","entities":[],"features":[],"native":[]},"transcript":""}`}}
	svc, _ := newSvc(f) // memory store seeds the initial (voice-only) DSL
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "new", Capabilities: []string{"answer"}, Messages: user("何が作れるの?")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "answer" || r.Mode != "create" || len(r.Suggestions) != 2 || r.Brief != nil {
		t.Fatalf("create answer: %+v", r)
	}
	if string(f.calls[0].Schema) != string(ChatCreateSchema()) || f.calls[0].System != chatCreateSystemPrompt {
		t.Fatal("create call setup")
	}
}

func TestChatFirst_CreateQuestionLimitForcesReady(t *testing.T) {
	q := `{"kind":"question","text":"何を記録しますか?","suggestions":["a","b"],"options":[],
	  "brief":{"name":"記録","purpose":"","entities":[],"features":[],"native":[]},"transcript":"記録アプリを作って。"}`
	f := &fakeCaller{replies: []string{q, q, q, q}}
	svc, _ := newSvc(f)
	ctx := context.Background()
	var r *ChatResult
	for i, say := range []string{"記録", "うーん", "まだ", "どうしよう"} {
		var err error
		r, err = svc.Chat(ctx, ChatRequest{AppID: "c1", Mode: "create", Messages: user(say)})
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 && r.Kind != "question" {
			t.Fatalf("turn %d: %+v", i+1, r)
		}
	}
	if r.Kind != "ready" || r.Source != "forced" || r.Transcript != "記録アプリを作って。" || r.Text != "では作りますね。" {
		t.Fatalf("turn 4: %+v", r)
	}
	if !strings.Contains(f.calls[3].Messages[0].Content, "## 質問の上限") {
		t.Fatal("limit not in prompt")
	}
}

func TestChatFirst_ModeSwitchResetsConversation(t *testing.T) {
	svc, f := newEditChatSvc(t, editAnswerReply, `{"kind":"answer","text":"ok","suggestions":[],"options":[],
	  "brief":{"name":"","purpose":"","entities":[],"features":[],"native":[]},"transcript":""}`)
	ctx := context.Background()
	if _, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Mode: "edit", Messages: user("これ WebView?")}); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Mode: "create", Messages: user("何が作れる?")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Turn != 1 || strings.Contains(f.calls[1].Messages[0].Content, "WebView") {
		t.Fatalf("history leaked across modes: turn=%d", r.Turn)
	}
}

func TestChatFirst_BadModeAndLegacyUntouched(t *testing.T) {
	svc, f := newEditChatSvc(t, questionReply)
	_, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Mode: "rewrite", Messages: user("x")})
	var ge *Error
	if !errors.As(err, &ge) || ge.Status != 400 {
		t.Fatalf("bad mode: %v", err)
	}
	// no mode / capabilities: the clarify path, even with a generated app
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Messages: user("家計簿")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "question" || r.Mode != "" || string(f.calls[0].Schema) != string(ChatSchema()) {
		t.Fatalf("legacy: %+v", r)
	}
}

func TestPartialFirstReply(t *testing.T) {
	full := `{"kind":"proposal","text":"案です","suggestions":[],"options":[{"title":"a","detail":"b","instruction":"c"}],"transcript":""}`
	cut := strings.Index(full, `,"transcript"`)
	if _, ok := partialFirstReply(full[:cut-5]); ok {
		t.Fatal("options not complete yet")
	}
	r, ok := partialFirstReply(full[:cut])
	if !ok || r.Kind != "proposal" || len(r.Options) != 1 {
		t.Fatalf("complete prefix: %+v %v", r, ok)
	}
	if _, ok := partialFirstReply(`{"kind":"ready","text":"x","suggestions":[],"options":[]`); ok {
		t.Fatal("ready must wait for the transcript")
	}
	if _, ok := partialFirstReply(`{"kind":"proposal","text":"x","suggestions":[],"options":[]`); ok {
		t.Fatal("proposal without options is not early")
	}
}

func TestSummarizeApp_Kakeibo(t *testing.T) {
	app, err := dsl.Parse(kakeibo(t))
	if err != nil {
		t.Fatal(err)
	}
	s := summarizeApp(app)
	for _, want := range []string{"expense「支出」(scope private)", `"id":"memo"`, "画面 expense_form「支出を記録」・expense の入力フォーム",
		`"type":"chart"`, `"label":"今月の支出"`, "通知: なし", "ツールバー"} {
		if !strings.Contains(s, want) {
			t.Fatalf("summary lacks %q:\n%s", want, s)
		}
	}
	// children are indented under their container
	if !strings.Contains(s, "\n    - {\"type\":\"stat\"") {
		t.Fatalf("children not nested:\n%s", s)
	}
}
