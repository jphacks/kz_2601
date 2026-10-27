package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestPIIKind(t *testing.T) {
	customer := &dsl.Entity{ID: "reservations", Name: "予約"}
	pet := &dsl.Entity{ID: "pets", Name: "ペット"}
	shop := &dsl.Entity{ID: "items", Name: "買うもの"}
	for _, c := range []struct {
		e    *dsl.Entity
		f    dsl.Field
		want string
	}{
		{customer, dsl.Field{ID: "name", Name: "お名前", Type: dsl.FieldString}, piiName},
		{shop, dsl.Field{ID: "guest_name", Name: "お客さんの名前", Type: dsl.FieldString}, piiName},
		{shop, dsl.Field{ID: "member", Name: "会員名", Type: dsl.FieldString}, piiName},
		{customer, dsl.Field{ID: "guestTel", Name: "TEL", Type: dsl.FieldString}, piiPhone},
		{customer, dsl.Field{ID: "phone", Name: "電話番号", Type: dsl.FieldString}, piiPhone},
		{customer, dsl.Field{ID: "email", Name: "メールアドレス", Type: dsl.FieldString}, piiEmail},
		{customer, dsl.Field{ID: "addr", Name: "住所", Type: dsl.FieldText}, piiAddress},
		{customer, dsl.Field{ID: "birthday", Name: "生年月日", Type: dsl.FieldDate}, piiBirthday},
		{customer, dsl.Field{ID: "spot", Name: "場所", Type: dsl.FieldLocation}, piiLocation},
		// misses
		{pet, dsl.Field{ID: "name", Name: "名前", Type: dsl.FieldString}, ""},
		{shop, dsl.Field{ID: "name", Name: "商品名", Type: dsl.FieldString}, ""},
		{shop, dsl.Field{ID: "hotel", Name: "ホテル", Type: dsl.FieldString}, ""},
		{customer, dsl.Field{ID: "date", Name: "予約日時", Type: dsl.FieldDatetime}, ""},
		{customer, dsl.Field{ID: "authorName", Name: "投稿者", Type: dsl.FieldString}, ""},
		{customer, dsl.Field{ID: "staff", Name: "担当者名", Type: dsl.FieldRef, RefEntity: "staff"}, ""},
	} {
		if got := piiKind(c.e, &c.f); got != c.want {
			t.Errorf("%s/%s(%s): got %q want %q", c.e.Name, c.f.ID, c.f.Name, got, c.want)
		}
	}
}

// warikanWithPhone: the warikan example, members get a phone field.
func warikanWithPhone(t *testing.T, scope string) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(warikanWithScope(t, scope), &doc); err != nil {
		t.Fatal(err)
	}
	m := doc["entities"].([]any)[0].(map[string]any)
	m["fields"] = append(m["fields"].([]any), map[string]any{"id": "tel", "name": "電話番号", "type": "string"})
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func piiFields(ps []PublicPersonalData) string {
	var s []string
	for _, p := range ps {
		s = append(s, p.Entity+"/"+p.Field+"="+p.Kind)
	}
	return strings.Join(s, ",")
}

func TestGenerate_PublicPIIWarnsOnCreate(t *testing.T) {
	svc, _ := newSvc(&fakeCaller{replies: []string{string(warikanWithPhone(t, "public"))}})
	res, err := svc.Generate(context.Background(), Request{AppID: "c1", Transcript: "旅行の割り勘。友達のスマホからも入力したい"})
	if err != nil {
		t.Fatal(err)
	}
	if got := piiFields(res.Meta.PublicPersonalData); got != "members/name=name,members/tel=phone" {
		t.Fatalf("meta: %s", got)
	}
	if !strings.Contains(res.Summary, "「名前」「電話番号」のような個人の情報も公開されます") {
		t.Errorf("summary lacks the field warning: %s", res.Summary)
	}
	if n := strings.Count(res.Summary, "誰でも見られ"); n != 1 {
		t.Errorf("visibility should be said once (share_intent.go), got %d: %s", n, res.Summary)
	}
	raw, _ := json.Marshal(res.Meta)
	if !strings.Contains(string(raw), `"publicPersonalData":[{"entity":"members","entityName":"メンバー","field":"name","fieldName":"名前","kind":"name"}`) {
		t.Errorf("meta json: %s", raw)
	}
}

func TestGenerate_NoPIIWarningWhenPrivate(t *testing.T) {
	svc, _ := newSvc(&fakeCaller{replies: []string{string(warikanWithPhone(t, ""))}})
	res, err := svc.Generate(context.Background(), Request{AppID: "c1", Transcript: "旅行の割り勘アプリ"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Meta.PublicPersonalData) != 0 || strings.Contains(res.Summary, "個人の情報") {
		t.Fatalf("private app must not warn: %s %+v", res.Summary, res.Meta.PublicPersonalData)
	}
}

// An edit warns only about fields that became public in this edit.
func TestGenerate_PublicPIIWarnsOnlyNewFieldsOnEdit(t *testing.T) {
	addPhone := `{"summary":"メンバーに電話番号を足しました。","patch":[{"op":"add","path":"/entities/0/fields/-","value":{"id":"tel","name":"電話番号","type":"string"}}]}`
	svc, _ := editSvc(t, warikanWithScope(t, "public"), addPhone)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "メンバーに電話番号を足して"})
	if err != nil {
		t.Fatal(err)
	}
	if got := piiFields(res.Meta.PublicPersonalData); got != "members/tel=phone" {
		t.Fatalf("meta: %s", got)
	}
	if !strings.Contains(res.Summary, "「電話番号」のような個人の情報も公開されます") || strings.Contains(res.Summary, "「名前」") {
		t.Errorf("summary: %s", res.Summary)
	}
}

func TestWithPIINote_EnglishAndCap(t *testing.T) {
	ps := []PublicPersonalData{{FieldName: "Name"}, {FieldName: "Phone"}}
	if got := withPIINote("Built it.", ps, LocaleEN); !strings.Contains(got, `"Name" and "Phone" will be public too`) {
		t.Errorf("en: %s", got)
	}
	many := make([]PublicPersonalData, 6)
	for i := range many {
		many[i].FieldName = string(rune('A' + i))
	}
	if got := withPIINote("", many, LocaleJA); !strings.Contains(got, "「D」など") || strings.Contains(got, "「E」") {
		t.Errorf("cap: %s", got)
	}
}

func TestPublicPIIKinds(t *testing.T) {
	for _, c := range []struct {
		text   string
		shared bool
		want   string
	}{
		{"美容室の予約アプリ。お客さんの名前と電話番号と予約日時をスタッフみんなで共有したい", false, "name,phone"},
		{"美容室の予約アプリ。お客さんの名前と電話番号と予約日時を記録したい", false, ""}, // not shared
		{"家族のペットの記録をみんなで共有して。ペットの名前と体重", false, ""},      // pet name
		{"アプリの名前を変えて", true, ""},         // not a person
		{"予約に住所の欄を足して", true, "address"}, // app already public
		{"Share with my friends: the full names and email of guests", false, "name,email"},
	} {
		if got := strings.Join(publicPIIKinds(c.text, nil, c.shared), ","); got != c.want {
			t.Errorf("%q: got %q want %q", c.text, got, c.want)
		}
	}
}

const piiReadyReply = `{"kind":"ready","text":"では作りますね。","suggestions":[],
  "brief":{"name":"予約","purpose":"美容室の予約","entities":["予約(お客さんの名前・電話番号・日時)"],"features":["予約の一覧"],"native":[]},
  "transcript":"美容室の予約アプリを作って。お客さんの名前と電話番号と予約日時をスタッフみんなで共有したい。"}`

// #253: chat asks once before a ready that puts personal data in public records.
func TestChat_PublicPIIAsksBeforeReady(t *testing.T) {
	svc, _ := newChatSvc(piiReadyReply, piiReadyReply)
	ctx := context.Background()
	r1, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("美容室の予約をスタッフみんなで共有したい。名前と電話も")})
	if err != nil {
		t.Fatal(err)
	}
	if r1.Kind != "question" || !strings.Contains(r1.Text, "名前・電話番号") || !strings.Contains(r1.Text, "公開してよいですか") {
		t.Fatalf("turn 1 should confirm: %+v", r1)
	}
	if strings.Join(r1.ConfirmPublic, ",") != "name,phone" || len(r1.Suggestions) != 3 || r1.Transcript != "" || r1.Brief != nil {
		t.Fatalf("question shape: %+v", r1)
	}
	r2, err := svc.Chat(ctx, ChatRequest{AppID: "a1", Messages: user("公開してよい")})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Kind != "ready" || r2.Transcript == "" || len(r2.ConfirmPublic) != 0 {
		t.Fatalf("after the answer it should be ready: %+v", r2)
	}
}

func TestChat_NoPIIQuestionForPrivateReady(t *testing.T) {
	svc, _ := newChatSvc(readyReply)
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Messages: user("家計簿")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "ready" || len(r.ConfirmPublic) != 0 {
		t.Fatalf("plain ready should pass: %+v", r)
	}
}

func TestChatFirst_PublicPIIAsksBeforeReady(t *testing.T) {
	reply := `{"kind":"ready","text":"作りますね。","suggestions":[],"options":[],
	  "brief":{"name":"予約","purpose":"美容室の予約","entities":["予約"],"features":["一覧"],"native":[]},
	  "transcript":"美容室の予約アプリ。お客さんの名前と電話番号をスタッフみんなで共有したい。"}`
	svc, _ := newChatSvc(reply)
	r, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Mode: "create", Messages: user("美容室の予約をみんなで共有")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "question" || len(r.ConfirmPublic) == 0 {
		t.Fatalf("chat-first create should confirm: %+v", r)
	}
}

func TestChatPrompts_PublicPIIRule(t *testing.T) {
	for name, p := range map[string]string{"chat create": chatCreateSystemPrompt, "chat edit": chatEditSystemPrompt} {
		if !strings.Contains(p, publicPIIChatRule) {
			t.Errorf("%s prompt lacks publicPIIChatRule", name)
		}
	}
}
