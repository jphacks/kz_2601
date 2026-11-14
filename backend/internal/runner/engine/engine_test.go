package engine

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// The #340 example (pkg/dsl/testdata/server_rules.json) plus extra rules.
func parseApp(t *testing.T, raw []byte) *dsl.App {
	t.Helper()
	a, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func exampleApp(t *testing.T) *dsl.App {
	t.Helper()
	raw, err := os.ReadFile("../../../pkg/dsl/testdata/server_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	return parseApp(t, raw)
}

const extraApp = `{
  "schemaVersion": 1,
  "app": {"name": "部活", "voiceEntry": "floating"},
  "initialScreen": "home",
  "entities": [
    {"id": "events", "name": "予定", "scope": "shared", "titleField": "title", "fields": [
      {"id": "title", "name": "タイトル", "type": "string", "required": true},
      {"id": "status", "name": "状態", "type": "enum", "options": ["予定", "確定", "中止"]},
      {"id": "points", "name": "点", "type": "integer"},
      {"id": "phone", "name": "番号", "type": "integer"},
      {"id": "last_at", "name": "最後", "type": "datetime"},
      {"id": "last_day", "name": "最後の日", "type": "date"},
      {"id": "secret", "name": "秘密", "type": "string", "hidden": true}
    ]},
    {"id": "entries", "name": "参加", "scope": "shared", "fields": [
      {"id": "event", "name": "予定", "type": "ref", "refEntity": "events"},
      {"id": "score", "name": "点", "type": "integer"},
      {"id": "memo", "name": "メモ", "type": "string"},
      {"id": "done", "name": "済", "type": "boolean"}
    ]},
    {"id": "log", "name": "記録", "scope": "shared", "fields": [
      {"id": "note", "name": "メモ", "type": "string", "required": true},
      {"id": "score", "name": "点", "type": "integer"}
    ]}
  ],
  "screens": [{"id": "home", "title": "予定", "components": [
    {"type": "list", "entity": "events", "item": {"title": "title"}}]}],
  "serverRules": [
    {"id": "confirmed_entry", "on": {"record": "created", "entity": "entries"},
     "if": {"op": "eq", "args": [{"ref": "event.status"}, "確定"]},
     "do": [
       {"increment": {"ref": "event", "field": "points", "by": {"op": "mul", "args": [{"ref": "score"}, 2]}}},
       {"updateRecord": {"ref": "event", "values": {"last_at": {"op": "now", "args": []}, "last_day": {"op": "today", "args": []}}}},
       {"createRecord": {"entity": "log", "values": {"note": {"op": "concat", "args": [{"ref": "event.title"}, " に参加"]}, "score": {"ref": "score"}}}},
       {"notify": {"to": {"kind": "authorOf", "field": "event"}, "title": "参加", "body": "{$author}: {memo}"}}
     ]},
    {"id": "score_up", "on": {"record": "updated", "entity": "entries", "fields": ["score"]},
     "if": {"op": "gt", "args": [{"ref": "score"}, {"ref": "$prev.score"}]},
     "do": [
       {"updateRecord": {"values": {"done": true}}},
       {"increment": {"ref": "event", "field": "points", "by": {"op": "sub", "args": [{"ref": "score"}, {"ref": "$prev.score"}]}}}
     ]}
  ]
}`

func rule(t *testing.T, a *dsl.App, id string) *dsl.ServerRule {
	t.Helper()
	for i := range a.ServerRules {
		if a.ServerRules[i].ID == id {
			return &a.ServerRules[i]
		}
	}
	t.Fatalf("no rule %q", id)
	return nil
}

// mapRefs is a RefReader over fixed records; it counts reads.
type mapRefs struct {
	recs  map[string]map[string]expreval.Record
	reads int
	err   error
}

func (m *mapRefs) Record(_ context.Context, entity, id string) (expreval.Record, bool, error) {
	m.reads++
	if m.err != nil {
		return expreval.Record{}, false, m.err
	}
	r, ok := m.recs[entity][id]
	return r, ok, nil
}

var tokyo = func() *time.Location {
	l, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		panic(err)
	}
	return l
}()

var at = time.Date(2026, 11, 6, 23, 30, 0, 0, time.UTC) // 2026-11-07 08:30 JST

func created(entity string, rec expreval.Record) Trigger {
	return Trigger{AppID: "app1", Scope: dsl.ScopeShared, Kind: KindCreated, Entity: entity, Record: rec,
		Actor: "u2", Version: 3, At: at, Location: tokyo}
}

func updated(entity string, prev, rec expreval.Record) Trigger {
	ev := created(entity, rec)
	ev.Kind, ev.Prev = KindUpdated, &prev
	return ev
}

func TestExampleRules(t *testing.T) {
	a := exampleApp(t)
	refs := &mapRefs{recs: map[string]map[string]expreval.Record{"events": {"e1": {ID: "e1", Author: "u1", Data: map[string]any{"title": "合宿"}}}}}
	rsvp := func(answer string) expreval.Record {
		return expreval.Record{ID: "r1", Author: "u2", AuthorName: "花子", Data: map[string]any{"event": "e1", "answer": answer}}
	}
	cases := []struct {
		name string
		rule string
		ev   Trigger
		want Plan
	}{
		{"出欠 yes: 通知 + 数える", "notify_rsvp", created("rsvps", rsvp("yes")), Plan{
			Writes: []PlannedWrite{{Action: 1, Kind: WriteIncrement, Entity: "events", RecordID: "e1", Field: "yes_count", By: 1}},
			Notifies: []PlannedNotify{{Action: 0, To: dsl.RuleRecipient{Kind: dsl.RecipientAuthorOf, Field: "event"},
				RefEntity: "events", RefID: "e1", Title: "出欠", Body: "花子さんが参加にしました"}},
		}},
		{"出欠 no: 条件が偽なので何もしない", "notify_rsvp", created("rsvps", rsvp("no")), Plan{}},
		{"新着メッセージ", "new_message", created("messages", expreval.Record{ID: "m1", AuthorName: "太郎", Data: map[string]any{"body": "明日は\n雨"}}), Plan{
			Notifies: []PlannedNotify{{To: dsl.RuleRecipient{Kind: dsl.RecipientMembers}, Title: "新着メッセージ", Body: "太郎: 明日は 雨"}},
		}},
		{"状態が変わった($prev)", "status_changed", updated("events",
			expreval.Record{ID: "e1", Data: map[string]any{"title": "合宿", "status": "予定"}},
			expreval.Record{ID: "e1", Data: map[string]any{"title": "合宿", "status": "確定"}}), Plan{
			Writes:   []PlannedWrite{{Action: 1, Kind: WriteCreate, Entity: "log", Values: map[string]any{"note": "状態の変更: 合宿"}}},
			Notifies: []PlannedNotify{{To: dsl.RuleRecipient{Kind: dsl.RecipientMembers}, Title: "予定が変わりました", Body: "合宿 は 確定 になりました"}},
		}},
		{"見ている field が変わっていない", "status_changed", updated("events",
			expreval.Record{ID: "e1", Data: map[string]any{"title": "旧", "status": "確定"}},
			expreval.Record{ID: "e1", Data: map[string]any{"title": "新", "status": "確定"}}), Plan{}},
		{"時刻のきっかけ", "morning", Trigger{AppID: "app1", Kind: KindScheduled, At: at, Location: tokyo}, Plan{
			Notifies: []PlannedNotify{{To: dsl.RuleRecipient{Kind: dsl.RecipientOwner}, Title: "今週の予定", Body: "今週の予定を見直しましょう"}},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := PlanRule(context.Background(), a, rule(t, a, c.rule), c.ev, refs)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestRefLookupAndValues(t *testing.T) {
	a := parseApp(t, []byte(extraApp))
	events := map[string]expreval.Record{
		"ok": {ID: "ok", Data: map[string]any{"title": "決勝", "status": "確定"}},
		"no": {ID: "no", Data: map[string]any{"title": "練習", "status": "予定"}},
	}
	entry := func(event string) expreval.Record {
		return expreval.Record{ID: "x1", AuthorName: "花子", Data: map[string]any{"event": event, "score": 3.0, "memo": "よろしく"}}
	}
	cases := []struct {
		name  string
		event string
		want  Plan
	}{
		{"ref 先が確定: 全部の処理", "ok", Plan{
			Writes: []PlannedWrite{
				{Action: 0, Kind: WriteIncrement, Entity: "events", RecordID: "ok", Field: "points", By: 6},
				{Action: 1, Kind: WriteUpdate, Entity: "events", RecordID: "ok", Values: map[string]any{
					"last_at": "2026-11-06T23:30:00Z", "last_day": "2026-11-07"}},
				{Action: 2, Kind: WriteCreate, Entity: "log", Values: map[string]any{"note": "決勝 に参加", "score": 3.0}},
			},
			Notifies: []PlannedNotify{{Action: 3, To: dsl.RuleRecipient{Kind: dsl.RecipientAuthorOf, Field: "event"},
				RefEntity: "events", RefID: "ok", Title: "参加", Body: "花子: よろしく"}},
		}},
		{"ref 先が確定でない: 何もしない", "no", Plan{}},
		{"ref 先が無い: 何もしない", "gone", Plan{}},
		{"ref が空: 何もしない", "", Plan{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			refs := &mapRefs{recs: map[string]map[string]expreval.Record{"events": events}}
			got, err := PlanRule(context.Background(), a, rule(t, a, "confirmed_entry"), created("entries", entry(c.event)), refs)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
			if c.event != "" && refs.reads != 1 {
				t.Errorf("reads = %d, want 1 (cached)", refs.reads)
			}
		})
	}
}

func TestPrev(t *testing.T) {
	a := parseApp(t, []byte(extraApp))
	r := rule(t, a, "score_up")
	rec := func(score any) expreval.Record {
		return expreval.Record{ID: "x1", Data: map[string]any{"event": "ev", "score": score}}
	}
	cases := []struct {
		name       string
		prev, next any
		want       Plan
	}{
		{"上がった", 3.0, 5.0, Plan{Writes: []PlannedWrite{
			{Action: 0, Kind: WriteUpdate, Entity: "entries", RecordID: "x1", Values: map[string]any{"done": true}},
			{Action: 1, Kind: WriteIncrement, Entity: "events", RecordID: "ev", Field: "points", By: 2},
		}}},
		{"下がった", 5.0, 3.0, Plan{}},
		{"前が空(nil との比較は偽)", nil, 3.0, Plan{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := PlanRule(context.Background(), a, r, updated("entries", rec(c.prev), rec(c.next)), &mapRefs{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestSanitizeInsert(t *testing.T) {
	cases := []struct{ in, want string }{
		{"よろしく", "よろしく"},
		{"", Missing},
		{"   ", Missing},
		{"詳しくは https://evil.example/x を見て", "詳しくは *** を見て"},
		{"www.example.com", Mask},
		{"evil.co へ", "*** へ"},
		{"連絡は foo@example.com まで", "連絡は *** まで"},
		{"ｆｏｏ＠ｅｘａｍｐｌｅ．ｃｏｍ", Mask}, // full width: NFKC first
		{"電話 090-1234-5678", "電話 ***"},
		{"０９０１２３４５６７８", Mask},
		{"部屋 302 番", "部屋 302 番"}, // short numbers stay
		{"改行\nあり\tタブ", "改行 あり タブ"},
		{"見えない\u200b文字\u202e", "見えない文字"},
		{strings.Repeat("あ", 40), strings.Repeat("あ", dsl.MaxNotifyInsertRunes)},
	}
	for _, c := range cases {
		if got := SanitizeInsert(c.in); got != c.want {
			t.Errorf("SanitizeInsert(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderCutsAndHides(t *testing.T) {
	a := exampleApp(t)
	r := rule(t, a, "new_message")
	long := strings.Repeat("長", 50) + " https://x.example"
	ev := created("messages", expreval.Record{ID: "m1", AuthorName: "foo@example.com", Data: map[string]any{"body": long}})
	got, err := PlanRule(context.Background(), a, r, ev, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Mask + ": " + strings.Repeat("長", dsl.MaxNotifyInsertRunes)
	if b := got.Notifies[0].Body; b != want {
		t.Errorf("body = %q, want %q", b, want)
	}
}

func TestRenderPlaceholders(t *testing.T) {
	a := parseApp(t, []byte(extraApp))
	row := &expreval.Row{Entity: a.EntityByID("events"), Record: expreval.Record{ID: "e1", AuthorName: "花子",
		Data: map[string]any{"title": "合宿", "points": 3.0, "phone": 9012345678.0, "last_at": "2026-11-06T23:30:00Z", "secret": "見せない"}}}
	p := planner{app: a, row: row, e: expreval.New(expreval.Env{Entities: a.Entities, Location: tokyo}), loc: tokyo}
	cases := []struct{ tmpl, want string }{
		{"{title}/{points}", "合宿/3"},
		{"{phone}", Mask}, // a number field can hold a phone number
		{"{last_at}", "2026-11-07 08:30"},
		{"{status}", Missing},
		{"{secret}", Missing},  // hidden
		{"{unknown}", Missing}, // not a field
		{"{$author}さん", "花子さん"},
		{strings.Repeat("あ", 78) + "{title}{title}", strings.Repeat("あ", 78) + "合宿"}, // body cut to 80
	}
	for _, c := range cases {
		if got := p.render(c.tmpl, dsl.MaxNotifyBodyRunes, nil); got != c.want {
			t.Errorf("render(%q) = %q, want %q", c.tmpl, got, c.want)
		}
	}
}

func TestErrors(t *testing.T) {
	a := exampleApp(t)
	r := rule(t, a, "notify_rsvp")
	rec := expreval.Record{ID: "r1", Data: map[string]any{"event": "e1", "answer": "yes"}}
	cases := []struct {
		name string
		rule *dsl.ServerRule
		ev   Trigger
		refs RefReader
	}{
		{"別の entity", r, created("messages", rec), nil},
		{"別の種類", r, updated("rsvps", rec, rec), nil},
		{"時刻のルールに記録のきっかけ", rule(t, a, "morning"), created("rsvps", rec), nil},
		{"更新で前の値が無い", rule(t, a, "status_changed"), Trigger{Kind: KindUpdated, Entity: "events", Record: rec}, nil},
		{"rule が無い", nil, created("rsvps", rec), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := PlanRule(context.Background(), a, c.rule, c.ev, c.refs); err == nil {
				t.Error("want an error")
			}
		})
	}
	// A failing RefReader stops the plan when the condition reads the ref.
	b := parseApp(t, []byte(extraApp))
	boom := errors.New("boom")
	_, err := PlanRule(context.Background(), b, rule(t, b, "confirmed_entry"),
		created("entries", expreval.Record{ID: "x", Data: map[string]any{"event": "ok"}}), &mapRefs{err: boom})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}
