package ai

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// mutateKakeibo applies fn to the decoded kakeibo example and returns the JSON.
func mutateKakeibo(t *testing.T, fn func(doc map[string]any)) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(kakeibo(t), &doc); err != nil {
		t.Fatal(err)
	}
	fn(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func parseApp(t *testing.T, raw string) *dsl.App {
	t.Helper()
	a, err := dsl.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("fixture does not parse: %v", err)
	}
	return a
}

func expenseEntity(doc map[string]any) map[string]any {
	return doc["entities"].([]any)[0].(map[string]any)
}

func setDescription(doc map[string]any, d string) { doc["app"].(map[string]any)["description"] = d }

// publicWithCheck: expenses are public and carry a confirmed boolean.
func publicWithCheck(doc map[string]any) {
	e := expenseEntity(doc)
	e["scope"] = "public"
	e["fields"] = append(e["fields"].([]any), map[string]any{"id": "confirmed", "name": "確定", "type": "boolean"})
}

func ids(rs []omissionRule) []string { return omissionIDs(rs) }

func TestOmissions_Notify(t *testing.T) {
	plain := parseApp(t, string(kakeibo(t)))
	if got := ids(omissions("在庫を管理して、少なくなったら教えて", plain, plain.App.Description, true)); !slices.Equal(got, []string{"notify"}) {
		t.Fatalf("missing notification should be reported: %v", got)
	}
	honest := parseApp(t, mutateKakeibo(t, func(d map[string]any) {
		setDescription(d, "在庫を記録する。少なくなった時の通知は今はできないため、不足の一覧で代替しました。")
	}))
	if got := omissions("少なくなったら教えて", honest, honest.App.Description, true); len(got) != 0 {
		t.Fatalf("an honest sentence about the notification is enough: %v", ids(got))
	}
	notified := parseApp(t, mutateKakeibo(t, func(d map[string]any) {
		d["app"].(map[string]any)["notifications"] = []any{map[string]any{"id": "n1", "title": "記録", "body": "今日の支出", "repeat": "daily", "time": "20:00"}}
	}))
	if got := omissions("毎晩8時に通知して", notified, "", true); len(got) != 0 {
		t.Fatalf("a real notification satisfies the request: %v", ids(got))
	}
	for _, tr := range []string{"通知を消して", "リマインドはいらない", "家計簿を作って", "Turn off the reminders"} {
		if got := omissions(tr, plain, "", true); len(got) != 0 {
			t.Fatalf("%q is not a notification request: %v", tr, ids(got))
		}
	}
	// a different topic's honest sentence does not excuse the notification
	other := "順位は合計の降順の表で代替しました。"
	if got := ids(omissions("使いすぎたら知らせて", plain, other, true)); !slices.Equal(got, []string{"notify"}) {
		t.Fatalf("unrelated substitution should not hide it: %v", got)
	}
}

func TestOmissions_SharedUpdate(t *testing.T) {
	a := parseApp(t, mutateKakeibo(t, publicWithCheck))
	if got := ids(omissions("バイトのシフト希望をスタッフが出して、店長が確定できるアプリ", a, a.App.Description, true)); !slices.Equal(got, []string{"shared_update"}) {
		t.Fatalf("manager confirm on public records: %v", got)
	}
	if got := ids(omissions("家族で共有する家事分担。終わったらチェック", a, "", true)); !slices.Equal(got, []string{"shared_update"}) {
		t.Fatalf("family check on public records: %v", got)
	}
	if got := omissions("出欠をメンバーが各自で〇×つけられる。誰が来るか一覧で見たい", a, "", true); len(got) != 0 {
		t.Fatalf("each member edits their own record: %v", ids(got))
	}
	if got := omissions("店長が確定できるアプリ", a, "みんなで使う記録は作った人だけが変更できるため、確定は希望を出した人しか押せません。", true); len(got) != 0 {
		t.Fatalf("honest sentence: %v", ids(got))
	}
	private := parseApp(t, mutateKakeibo(t, func(d map[string]any) {
		publicWithCheck(d)
		delete(expenseEntity(d), "scope")
	}))
	if got := omissions("店長が確定できるアプリ", private, "", true); len(got) != 0 {
		t.Fatalf("private records are not affected: %v", ids(got))
	}
}

func TestOmissions_QuizScore(t *testing.T) {
	a := parseApp(t, string(kakeibo(t)))
	if got := ids(omissions("友達とクイズを出し合って点数を競うアプリ", a, "", true)); !slices.Equal(got, []string{"quiz_score"}) {
		t.Fatalf("hand-typed score: %v", got)
	}
	if got := omissions("友達とクイズを出し合って点数を競うアプリ", a, "", false); len(got) != 0 {
		t.Fatalf("the UI pass has no computed fields yet: %v", ids(got))
	}
	if got := omissions("クイズで点数を競う", a, "自動の採点は今はできないため、点数は自分で入力する形で代替しました。", true); len(got) != 0 {
		t.Fatalf("honest sentence: %v", ids(got))
	}
}

func TestOmissions_SaveCopy(t *testing.T) {
	pub := parseApp(t, mutateKakeibo(t, func(d map[string]any) { expenseEntity(d)["scope"] = "public" }))
	tr := "みんなでレシピを投稿して、気に入ったのを保存できるアプリ"
	if got := ids(omissions(tr, pub, "", true)); !slices.Equal(got, []string{"save_copy"}) {
		t.Fatalf("no private ref to the shared posts: %v", got)
	}
	withFav := parseApp(t, mutateKakeibo(t, func(d map[string]any) {
		expenseEntity(d)["scope"] = "public"
		d["entities"] = append(d["entities"].([]any), map[string]any{"id": "fav", "name": "お気に入り", "fields": []any{
			map[string]any{"id": "post", "name": "投稿", "type": "ref", "refEntity": "expense"}}})
	}))
	if got := omissions(tr, withFav, "", true); len(got) != 0 {
		t.Fatalf("a private favourite pointing at the post satisfies it: %v", ids(got))
	}
	private := parseApp(t, string(kakeibo(t)))
	if got := omissions("気に入った支出に印をつけたい", private, "", true); len(got) != 0 {
		t.Fatalf("own records can just be marked: %v", ids(got))
	}
}

// Create turn without AI_LINT_RETRY: no second call, the summary gets the fixed note.
func TestGenerate_OmissionNoteWithoutRetry(t *testing.T) {
	f := &fakeCaller{replies: []string{string(kakeibo(t))}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿アプリ。使いすぎたら教えて"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || !slices.Equal(res.Meta.Omissions, []string{"notify"}) {
		t.Fatalf("calls=%d meta=%+v", len(f.calls), res.Meta)
	}
	if !strings.HasSuffix(res.Summary, "条件で知らせる通知は今はできないため、通知は入れていません。") {
		t.Fatalf("summary should say the notification is missing: %q", res.Summary)
	}
}

// With AI_LINT_RETRY the omission is sent back once; an honest retry is read out as is.
func TestGenerate_OmissionRetryThenHonest(t *testing.T) {
	honest := mutateKakeibo(t, func(d map[string]any) {
		setDescription(d, "使ったお金を記録する。使いすぎの通知は今はできないため、今月の合計で代替しました。")
	})
	f := &fakeCaller{replies: []string{string(kakeibo(t)), honest}}
	svc, _ := newSvc(f)
	svc.LintRetry = true
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿アプリ。使いすぎたら教えて"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || !res.Meta.LintRetried || len(res.Meta.Omissions) != 0 {
		t.Fatalf("calls=%d meta=%+v", len(f.calls), res.Meta)
	}
	if p := f.calls[1].Messages[0].Content; !strings.Contains(p, "omission notify") || !strings.Contains(p, "中身に以下の問題") {
		t.Fatalf("retry prompt should carry the omission: %s", p)
	}
	if !strings.Contains(res.Summary, "使いすぎの通知は今はできないため、今月の合計で代替しました。") || strings.Contains(res.Summary, "入れていません") {
		t.Fatalf("summary: %q", res.Summary)
	}
}

// Edit turns: the note goes after the patch summary.
func TestGenerate_OmissionNoteOnEdit(t *testing.T) {
	patch := `{"summary":"アプリ名を変えました。","patch":[{"op":"replace","path":"/app/name","value":"お財布"}]}`
	svc, _ := newEditChatSvc(t, patch)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "名前をお財布にして、使いすぎたら知らせて"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary != "アプリ名を変えました。条件で知らせる通知は今はできないため、通知は入れていません。" {
		t.Fatalf("summary: %q", res.Summary)
	}
}

func TestSubstitutionNote_ReadsCannotSentences(t *testing.T) {
	d := "シフト希望を出す。店長の確定は今はできません。カレンダーは日付順の一覧で代替しました。毎日使う。"
	if got := substitutionNote(d); got != "店長の確定は今はできません。カレンダーは日付順の一覧で代替しました。" {
		t.Fatalf("got %q", got)
	}
	if got := substitutionNoteEN("Track shifts. Manager approval isn't possible yet. Calendar view was replaced with a list."); got != "Manager approval isn't possible yet. Calendar view was replaced with a list." {
		t.Fatalf("en got %q", got)
	}
}

func TestSystemPrompt_RequestCheckRules(t *testing.T) {
	for _, want := range []string{"依頼の点検", "依頼の一部を黙って落とさない", "作った人だけが変更・削除できる", "条件で知らせる通知", "自動の採点", "ほかの人の投稿を自分用に保存"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("systemPrompt lacks %q", want)
		}
	}
}
