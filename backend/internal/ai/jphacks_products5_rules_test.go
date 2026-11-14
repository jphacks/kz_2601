package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// The expressions exactly as jphacksProducts5Rules writes them.
const (
	jp5RateExpr      = `{"if":{"op":"gt","args":[{"agg":"count","entity":"check","filter":{"field":"manual","op":"eq","value":{"ref":"id"}}},0]},"then":{"op":"round","args":[{"op":"mul","args":[{"op":"div","args":[{"agg":"count","entity":"check","filter":[{"field":"manual","op":"eq","value":{"ref":"id"}},{"field":"result","op":"eq","value":"できている"}]},{"agg":"count","entity":"check","filter":{"field":"manual","op":"eq","value":{"ref":"id"}}}]},100]}]},"else":0}`
	jp5ReachedExpr   = `{"agg":"count","entity":"drill","filter":[{"field":"shelter","op":"eq","value":{"ref":"id"}},{"field":"arrived","op":"eq","value":true}]}`
	jp5BadgesExpr    = `{"agg":"count","entity":"shelter","filter":{"field":"reached","op":"gt","value":0}}`
	jp5SafeFilter    = `{"field":"safety","op":"eq","value":"大丈夫"}`
	jp5MineFilter    = `{"field":"$author","op":"eq","value":{"me":true}}`
	jp5UrgencyExpr   = `{"op":"add","args":[{"op":"mul","args":[10,{"if":{"op":"eq","args":[{"ref":"priority"},"高"]},"then":3,"else":{"if":{"op":"eq","args":[{"ref":"priority"},"中"]},"then":2,"else":1}}]},{"op":"sub","args":[10,{"op":"clamp","args":[{"op":"coalesce","args":[{"op":"daysUntil","args":[{"ref":"due"}]},10]},0,10]}]}]}`
	jp5StageExpr     = `{"if":{"op":"gte","args":[{"ref":"urgency"},35]},"then":"真っ赤","else":{"if":{"op":"gte","args":[{"ref":"urgency"},25]},"then":"赤","else":{"if":{"op":"gte","args":[{"ref":"urgency"},15]},"then":"オレンジ","else":"緑"}}}`
	jp5AgoExpr       = `{"op":"sub","args":[{"op":"add","args":[{"op":"mul","args":[{"op":"year","args":[{"op":"today","args":[]}]},12]},{"op":"month","args":[{"op":"today","args":[]}]}]},{"op":"add","args":[{"op":"mul","args":[{"op":"year","args":[{"ref":"date"}]},12]},{"op":"month","args":[{"ref":"date"}]}]}]}`
	jp5ThisMonthExpr = `{"agg":"sum","entity":"sale","field":"amount","filter":[{"field":"store","op":"eq","value":{"ref":"id"}},{"field":"ago","op":"eq","value":0}]}`
	jp5LastMonthExpr = `{"agg":"sum","entity":"sale","field":"amount","filter":[{"field":"store","op":"eq","value":{"ref":"id"}},{"field":"ago","op":"eq","value":1}]}`
	jp5GrowthExpr    = `{"if":{"op":"gt","args":[{"ref":"last_month"},0]},"then":{"op":"round","args":[{"op":"mul","args":[{"op":"div","args":[{"ref":"this_month"},{"ref":"last_month"}]},100]}]},"else":0}`
	jp5ManualPrompt  = `次の説明 {{input}} を、初めての人でもできる番号付きの手順(1 手順 1 行・注意点つき)に書き直す`
	jp5CheckPrompt   = `写真の作業が手順どおりにできているかを判定する`
	jp5DrillPrompt   = `{{disaster}} が起きた想定で、近くの避難所まで歩いて逃げる訓練のシナリオ(起きること・気をつけること)`
	jp5SafePrompt    = `子どもの SNS 投稿の練習。名前・住所・学校・電話番号などの個人情報や、人を傷つける言葉があれば 見直そう`
	jp5CommentPrompt = `子どもの SNS 投稿 {{input}} に、フォロワーのようにやさしく感想を書き、直すと良い所を 1 つ教える`
	jp5Head          = `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home",`
)

func TestJPHACKSProducts5Rules_InPrompt(t *testing.T) {
	// jp5LastMonthExpr is "同じ形で ago eq 1" in the prompt, so only its shape is checked.
	for _, s := range []string{jp5RateExpr, jp5ReachedExpr, jp5BadgesExpr, jp5SafeFilter, jp5MineFilter, jp5UrgencyExpr, jp5StageExpr, jp5AgoExpr, jp5ThisMonthExpr, jp5GrowthExpr,
		jp5ManualPrompt, jp5CheckPrompt, jp5DrillPrompt, jp5SafePrompt, jp5CommentPrompt} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("systemPrompt lacks %s", s)
		}
	}
	for _, s := range []string{jp5RateExpr, jp5ReachedExpr, jp5BadgesExpr, jp5SafeFilter, jp5MineFilter, jp5UrgencyExpr, jp5StageExpr, jp5AgoExpr, jp5ThisMonthExpr, jp5LastMonthExpr, jp5GrowthExpr} {
		if !json.Valid([]byte(s)) {
			t.Errorf("not valid JSON: %s", s)
		}
	}
}

// The inputPhoto rule is in the full prompt only: with AI_TEXT_IMAGE off
// (the production default) the restricted prompt drops it, the rest stays.
func TestJPHACKSProducts5PhotoRule_GatedByTextImage(t *testing.T) {
	on := (&Service{TextImage: true}).clientRestriction(nil, nil, nil)
	full := restrictPrompt(withRestriction(context.Background(), on), systemPrompt)
	off := (&Service{}).clientRestriction(nil, nil, nil)
	restricted := withoutNote(restrictPrompt(withRestriction(context.Background(), off), systemPrompt))
	for _, s := range []string{jphacksProducts5PhotoRule, jp5CheckPrompt, jp5RateExpr} {
		if !strings.Contains(full, s) {
			t.Errorf("full prompt lacks %.60s", s)
		}
		if strings.Contains(restricted, s) {
			t.Errorf("restricted prompt still has %.60s", s)
		}
	}
	for _, s := range []string{jp5DrillPrompt, jp5UrgencyExpr, jp5GrowthExpr} {
		if !strings.Contains(restricted, s) {
			t.Errorf("restricted prompt lost %.60s", s)
		}
	}
}

func jp5Str(s string) string { b, _ := json.Marshal(s); return string(b) }

// The smallest app that follows each rule. They must pass dsl.Parse with no lint.
var jp5RuleApps = map[string]string{
	"manual": jp5Head + `"entities":[
 {"id":"manual","name":"手順書","scope":"shared","titleField":"title","fields":[{"id":"title","name":"題","type":"string","required":true},
  {"id":"request","name":"説明","type":"text"},{"id":"steps","name":"手順","type":"text"},
  {"id":"rate","name":"できた割合","type":"integer","unit":"%","computed":` + jp5RateExpr + `}]},
 {"id":"check","name":"確認","scope":"shared","titleField":"result","fields":[{"id":"manual","name":"手順書","type":"ref","refEntity":"manual","required":true},
  {"id":"photo","name":"写真","type":"photo"},
  {"id":"result","name":"判定","type":"enum","options":["できている","一部できている","できていない"]},
  {"id":"date","name":"日付","type":"date","defaultValue":"now"}]}],
"screens":[
 {"id":"home","title":"手順書","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"manual_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"manual","item":{"title":"title","trailing":"rate"},"onItemTap":{"type":"navigate","screen":"manual_form"}},
   {"type":"list","entity":"check","sort":{"field":"date","direction":"desc"},"item":{"title":"result","subtitle":"manual","image":"photo"},"onItemTap":{"type":"navigate","screen":"check_form"}}]},
 {"id":"manual_form","title":"手順書","entity":"manual","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"manual"}}],
  "components":[{"type":"textField","field":"title"},{"type":"textField","field":"request","multiline":true},
   {"type":"aiButton","task":"rewrite","input":"request","output":"steps","length":"long","label":"手順にする","prompt":` + jp5Str(jp5ManualPrompt) + `},
   {"type":"textField","field":"steps","multiline":true},
   {"type":"button","label":"できたか確かめる","action":{"type":"navigate","screen":"check_form","prefillRef":"manual"}}]},
 {"id":"check_form","title":"確認","entity":"check","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"check"}}],
  "components":[{"type":"picker","field":"manual"},{"type":"photoField","field":"photo","source":"camera"},
   {"type":"aiButton","task":"classify","inputPhoto":"photo","output":"result","label":"写真で判定","prompt":` + jp5Str(jp5CheckPrompt) + `},
   {"type":"picker","field":"result"}]}]}`,

	"drill": jp5Head + `"entities":[
 {"id":"shelter","name":"避難所","titleField":"name","fields":[{"id":"name","name":"避難所","type":"string","required":true},
  {"id":"place","name":"場所","type":"location","required":true},
  {"id":"reached","name":"着いた回数","type":"integer","computed":` + jp5ReachedExpr + `}]},
 {"id":"drill","name":"訓練","titleField":"disaster","fields":[{"id":"shelter","name":"避難所","type":"ref","refEntity":"shelter","required":true},
  {"id":"disaster","name":"災害","type":"enum","options":["地震","大雨","火事"]},
  {"id":"scenario","name":"シナリオ","type":"text"},
  {"id":"arrived","name":"着いた","type":"boolean","defaultValue":false},
  {"id":"date","name":"日付","type":"date","defaultValue":"now"}]}],
"screens":[
 {"id":"home","title":"避難訓練","toolbar":[{"label":"訓練","action":{"type":"navigate","screen":"drill_form","presentation":"sheet"}}],
  "components":[{"type":"stat","label":"集めたバッジ","value":` + jp5BadgesExpr + `},
   {"type":"routeGuide","entity":"shelter","locationField":"place","titleField":"name","mode":"walking"},
   {"type":"list","entity":"shelter","item":{"title":"name","trailing":"reached"},"onItemTap":{"type":"navigate","screen":"shelter_form"}},
   {"type":"list","entity":"drill","sort":{"field":"date","direction":"desc"},"item":{"title":"disaster","subtitle":"shelter","trailing":"date"},"onItemTap":{"type":"navigate","screen":"drill_form"}},
   {"type":"button","label":"避難所を登録","action":{"type":"navigate","screen":"shelter_form","presentation":"sheet"}}]},
 {"id":"shelter_form","title":"避難所","entity":"shelter","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"shelter"}}],
  "components":[{"type":"textField","field":"name"},{"type":"locationPicker","field":"place","nameField":"name"}]},
 {"id":"drill_form","title":"訓練","entity":"drill","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"drill"}}],
  "components":[{"type":"picker","field":"shelter"},{"type":"picker","field":"disaster"},
   {"type":"aiButton","task":"generate","output":"scenario","length":"long","label":"シナリオを作る","prompt":` + jp5Str(jp5DrillPrompt) + `},
   {"type":"textField","field":"scenario","multiline":true},{"type":"toggle","field":"arrived","label":"着いた"}]}]}`,

	"post": jp5Head + `"entities":[
 {"id":"post","name":"投稿","scope":"shared","titleField":"body","marks":[{"id":"like","icon":"heart","label":"いいね"}],
  "fields":[{"id":"body","name":"投稿","type":"text","required":true},
  {"id":"safety","name":"AI の判定","type":"enum","options":["大丈夫","見直そう"]},
  {"id":"comment","name":"AI の感想","type":"text"},
  {"id":"date","name":"日時","type":"datetime","defaultValue":"now"}]}],
"screens":[
 {"id":"home","title":"タイムライン","toolbar":[{"label":"投稿","action":{"type":"navigate","screen":"post_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"post","filter":` + jp5SafeFilter + `,"sort":{"field":"date","direction":"desc"},"item":{"title":"body","subtitle":"date","mark":"like"},"onItemTap":{"type":"navigate","screen":"post_form"}},
   {"type":"button","label":"自分の投稿","action":{"type":"navigate","screen":"mine"}}]},
 {"id":"mine","title":"自分の投稿","components":[{"type":"list","entity":"post","filter":` + jp5MineFilter + `,"item":{"title":"body","subtitle":"safety"},"onItemTap":{"type":"navigate","screen":"post_form"}}]},
 {"id":"post_form","title":"投稿","entity":"post","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"post"}}],
  "components":[{"type":"textField","field":"body","multiline":true},
   {"type":"aiButton","task":"classify","input":"body","output":"safety","label":"AI に見てもらう","prompt":` + jp5Str(jp5SafePrompt) + `},
   {"type":"picker","field":"safety"},
   {"type":"aiButton","task":"suggest","input":"body","output":"comment","label":"AI の感想","prompt":` + jp5Str(jp5CommentPrompt) + `},
   {"type":"textField","field":"comment","multiline":true}]}]}`,

	"urgency": jp5Head + `"entities":[
 {"id":"task","name":"タスク","titleField":"title","fields":[{"id":"title","name":"タスク","type":"string","required":true},
  {"id":"priority","name":"優先度","type":"enum","options":["高","中","低"]},
  {"id":"due","name":"期限","type":"date"},
  {"id":"done","name":"完了","type":"boolean","defaultValue":false},
  {"id":"urgency","name":"急ぎ度","type":"integer","computed":` + jp5UrgencyExpr + `},
  {"id":"stage","name":"色","type":"string","computed":` + jp5StageExpr + `}]}],
"screens":[
 {"id":"home","title":"タスク","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"task_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"task","filter":{"field":"done","op":"eq","value":false},"sort":{"field":"urgency","direction":"desc"},"item":{"title":"title","subtitle":"due","trailing":"stage"},"onItemTap":{"type":"navigate","screen":"task_form"}}]},
 {"id":"task_form","title":"タスク","entity":"task","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"task"}}],
  "components":[{"type":"textField","field":"title"},{"type":"picker","field":"priority"},{"type":"datePicker","field":"due"},{"type":"toggle","field":"done","label":"完了"}]}]}`,

	"sales": jp5Head + `"entities":[
 {"id":"store","name":"店","scope":"shared","titleField":"name","fields":[{"id":"name","name":"店","type":"string","required":true},
  {"id":"this_month","name":"今月","type":"integer","unit":"円","computed":` + jp5ThisMonthExpr + `},
  {"id":"last_month","name":"先月","type":"integer","unit":"円","computed":` + jp5LastMonthExpr + `},
  {"id":"growth","name":"前月比","type":"integer","unit":"%","computed":` + jp5GrowthExpr + `}]},
 {"id":"sale","name":"売上","scope":"shared","titleField":"store","fields":[{"id":"store","name":"店","type":"ref","refEntity":"store","required":true},
  {"id":"date","name":"日付","type":"date","defaultValue":"now"},
  {"id":"amount","name":"売上","type":"integer","unit":"円"},
  {"id":"ago","name":"何か月前","type":"integer","computed":` + jp5AgoExpr + `}]}],
"screens":[
 {"id":"home","title":"売上","toolbar":[{"label":"記録","action":{"type":"navigate","screen":"sale_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"store","sort":{"field":"this_month","direction":"desc"},"item":{"title":"name","subtitle":"this_month","trailing":"growth"},"onItemTap":{"type":"navigate","screen":"store_form"}},
   {"type":"chart","entity":"sale","kind":"bar","groupBy":"date","bucket":"month","aggregate":"sum","field":"amount","title":"月ごとの売上"},
   {"type":"list","entity":"sale","sort":{"field":"date","direction":"desc"},"item":{"title":"store","subtitle":"date","trailing":"amount"},"onItemTap":{"type":"navigate","screen":"sale_form"}},
   {"type":"button","label":"店を登録","action":{"type":"navigate","screen":"store_form","presentation":"sheet"}}]},
 {"id":"store_form","title":"店","entity":"store","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"store"}}],
  "components":[{"type":"textField","field":"name"}]},
 {"id":"sale_form","title":"売上","entity":"sale","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"sale"}}],
  "components":[{"type":"picker","field":"store"},{"type":"datePicker","field":"date"},{"type":"numberField","field":"amount"}]}]}`,

	"emotion": jp5Head + `"entities":[
 {"id":"msg","name":"メッセージ","titleField":"body","fields":[{"id":"body","name":"メッセージ","type":"text","required":true},
  {"id":"emotion","name":"感情","type":"enum","options":["うれしい","ふつう","かなしい","いらいら"]},
  {"id":"date","name":"日付","type":"date","defaultValue":"now"}]}],
"screens":[
 {"id":"home","title":"感情","toolbar":[{"label":"貼る","action":{"type":"navigate","screen":"msg_form","presentation":"sheet"}}],
  "components":[{"type":"chart","entity":"msg","kind":"pie","groupBy":"emotion","aggregate":"count","title":"感情の割合"},
   {"type":"chart","entity":"msg","kind":"bar","groupBy":"date","bucket":"month","aggregate":"count","title":"月ごとの数"},
   {"type":"list","entity":"msg","sort":{"field":"date","direction":"desc"},"item":{"title":"body","subtitle":"emotion"},"onItemTap":{"type":"navigate","screen":"msg_form"}}]},
 {"id":"msg_form","title":"メッセージ","entity":"msg","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"msg"}}],
  "components":[{"type":"textField","field":"body","multiline":true},
   {"type":"aiButton","task":"classify","input":"body","output":"emotion","label":"感情を分ける"},
   {"type":"picker","field":"emotion"}]}]}`,
}

func TestJPHACKSProducts5Rules_AppsValidate(t *testing.T) {
	for name, raw := range jp5RuleApps {
		app, err := dsl.Parse([]byte(raw))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if l := app.Lint(); len(l) > 0 {
			t.Errorf("%s: lint %v", name, l)
		}
	}
}

func TestJPHACKSProducts5Rules_ExprsEvaluate(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	rec := func(id string, data map[string]any) expreval.Record { return expreval.Record{ID: id, Data: data} }
	setup := func(name string, recs map[string][]expreval.Record, at time.Time) (*dsl.App, *expreval.Evaluator) {
		app, err := dsl.Parse([]byte(jp5RuleApps[name]))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		src := expreval.SourceFunc(func(id string) []expreval.Record { return recs[id] })
		return app, expreval.New(expreval.Env{Entities: app.Entities, Source: src, Now: at})
	}
	expr := func(raw string) dsl.Expr {
		var e dsl.Expr
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	field := func(app *dsl.App, ev *expreval.Evaluator, entity, fid string, r expreval.Record) expreval.Value {
		return ev.FieldValue(fid, &expreval.Row{Entity: app.EntityByID(entity), Record: r})
	}
	num := func(v expreval.Value, want float64) bool { return v.Kind == expreval.Number && v.Num == want }
	str := func(v expreval.Value, want string) bool { return v.Kind == expreval.String && v.Str == want }

	{ // manual: share of checks judged done, 0 with no checks
		ms := []expreval.Record{rec("m1", map[string]any{"title": "棚の組み立て"}), rec("m2", map[string]any{"title": "掃除"})}
		cs := []expreval.Record{
			rec("c1", map[string]any{"manual": "m1", "result": "できている"}),
			rec("c2", map[string]any{"manual": "m1", "result": "一部できている"}),
			rec("c3", map[string]any{"manual": "m1", "result": "できている"}),
		}
		app, ev := setup("manual", map[string][]expreval.Record{"manual": ms, "check": cs}, now)
		for i, want := range []float64{67, 0} {
			if v := field(app, ev, "manual", "rate", ms[i]); !num(v, want) {
				t.Errorf("rate(%s) = %v, want %v", ms[i].ID, v, want)
			}
		}
	}

	{ // drill: shelters reached at least once
		ss := []expreval.Record{rec("s1", map[string]any{"name": "小学校"}), rec("s2", map[string]any{"name": "公民館"}), rec("s3", map[string]any{"name": "公園"})}
		ds := []expreval.Record{
			rec("d1", map[string]any{"shelter": "s1", "arrived": true}),
			rec("d2", map[string]any{"shelter": "s1", "arrived": true}),
			rec("d3", map[string]any{"shelter": "s2", "arrived": false}),
			rec("d4", map[string]any{"shelter": "s3", "arrived": true}),
		}
		app, ev := setup("drill", map[string][]expreval.Record{"shelter": ss, "drill": ds}, now)
		for i, want := range []float64{2, 0, 1} {
			if v := field(app, ev, "shelter", "reached", ss[i]); !num(v, want) {
				t.Errorf("reached(%s) = %v, want %v", ss[i].ID, v, want)
			}
		}
		if v := ev.Eval(expr(jp5BadgesExpr), nil); !num(v, 2) {
			t.Errorf("badges = %v, want 2", v)
		}
	}

	{ // urgency: priority weight + days left; overdue and no due
		ts := []expreval.Record{
			rec("t1", map[string]any{"priority": "高", "due": "2026-10-29"}), // 30 + 9 = 39
			rec("t2", map[string]any{"priority": "中", "due": "2026-11-20"}), // 20 + 0 = 20
			rec("t3", map[string]any{"priority": "低", "due": "2026-10-20"}), // 10 + 10 = 20 (overdue)
			rec("t4", map[string]any{"priority": "低"}),                      // 10 + 0 = 10 (no due)
			rec("t5", map[string]any{"priority": "中", "due": "2026-10-31"}), // 20 + 7 = 27
		}
		app, ev := setup("urgency", map[string][]expreval.Record{"task": ts}, now)
		for i, c := range []struct {
			u float64
			s string
		}{{39, "真っ赤"}, {20, "オレンジ"}, {20, "オレンジ"}, {10, "緑"}, {27, "赤"}} {
			if v := field(app, ev, "task", "urgency", ts[i]); !num(v, c.u) {
				t.Errorf("urgency(%s) = %v, want %v", ts[i].ID, v, c.u)
			}
			if v := field(app, ev, "task", "stage", ts[i]); !str(v, c.s) {
				t.Errorf("stage(%s) = %v, want %s", ts[i].ID, v, c.s)
			}
		}
	}

	{ // sales: this month, last month (also across New Year), growth
		st := []expreval.Record{rec("a", map[string]any{"name": "駅前店"}), rec("b", map[string]any{"name": "本店"})}
		sl := []expreval.Record{
			rec("1", map[string]any{"store": "a", "date": "2026-10-03", "amount": 30000.0}),
			rec("2", map[string]any{"store": "a", "date": "2026-10-20", "amount": 15000.0}),
			rec("3", map[string]any{"store": "a", "date": "2026-09-30", "amount": 40000.0}),
			rec("4", map[string]any{"store": "a", "date": "2026-08-15", "amount": 99000.0}),
			rec("5", map[string]any{"store": "b", "date": "2026-10-01", "amount": 12000.0}),
		}
		app, ev := setup("sales", map[string][]expreval.Record{"store": st, "sale": sl}, now)
		for i, c := range []struct{ this, last, growth float64 }{{45000, 40000, 113}, {12000, 0, 0}} {
			for _, f := range []struct {
				id   string
				want float64
			}{{"this_month", c.this}, {"last_month", c.last}, {"growth", c.growth}} {
				if v := field(app, ev, "store", f.id, st[i]); !num(v, f.want) {
					t.Errorf("%s(%s) = %v, want %v", f.id, st[i].ID, v, f.want)
				}
			}
		}
		jan := time.Date(2027, 1, 10, 12, 0, 0, 0, time.UTC)
		app, ev = setup("sales", map[string][]expreval.Record{"store": st, "sale": {
			rec("6", map[string]any{"store": "a", "date": "2026-12-31", "amount": 20000.0}),
			rec("7", map[string]any{"store": "a", "date": "2027-01-05", "amount": 25000.0}),
		}}, jan)
		if v := field(app, ev, "store", "growth", st[0]); !num(v, 125) {
			t.Errorf("growth across New Year = %v, want 125", v)
		}
	}
}
