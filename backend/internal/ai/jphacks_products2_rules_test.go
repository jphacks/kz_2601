package ai

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// The expressions exactly as jphacksProducts2Rules writes them.
const (
	jp2HPExpr     = `{"op":"clamp","args":[{"op":"add","args":[{"op":"sub","args":[100,{"op":"mul","args":[10,{"agg":"count","entity":"task","filter":{"field":"done","op":"eq","value":true}}]}]},{"op":"mul","args":[10,{"agg":"count","entity":"task","filter":[{"field":"done","op":"eq","value":false},{"field":"due","op":"lt","value":{"op":"today","args":[]}}]}]}]},0,100]}`
	jp2ReadyExpr  = `{"op":"and","args":[{"op":"not","args":[{"ref":"done"}]},{"op":"coalesce","args":[{"ref":"prev.done"},true]}]}`
	jp2SameExpr   = `{"agg":"count","entity":"post","filter":[{"field":"feeling","op":"eq","value":{"ref":"feeling"}},{"field":"date","op":"today","value":null}]}`
	jp2LastExpr   = `{"agg":"latest","entity":"card","field":"date","by":"date","filter":{"field":"friend","op":"eq","value":{"ref":"id"}}}`
	jp2SinceExpr  = `{"op":"coalesce","args":[{"op":"daysBetween","args":[{"ref":"last_sent"},{"op":"today","args":[]}]},9999]}`
	jp2DiagExpr   = `{"if":{"op":"eq","args":[{"ref":"acidity"},"好き"]},"then":"エチオピア(浅煎り)","else":"マンデリン(深煎り)"}`
	jp2NotMine    = `{"field":"$marks.done","op":"neq","value":{"me":true}}`
	jp2MarksDecl  = `[{"id":"done","icon":"check","label":"済み"}]`
	jp2ReadyFilt  = `{"field":"ready","op":"eq","value":true}`
	jp2HeadPrefix = `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home",`
)

func TestJPHACKSProducts2Rules_InPrompt(t *testing.T) {
	for _, s := range []string{jp2HPExpr, jp2ReadyExpr, jp2SameExpr, jp2LastExpr, jp2SinceExpr, jp2DiagExpr, jp2NotMine, jp2MarksDecl, jp2ReadyFilt} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("systemPrompt lacks %s", s)
		}
		if !json.Valid([]byte(s)) {
			t.Errorf("not valid JSON: %s", s)
		}
	}
}

// The smallest app that follows each rule. They must pass dsl.Parse with no lint.
var jp2RuleApps = map[string]string{
	"monster hp": jp2HeadPrefix + `"entities":[
 {"id":"task","name":"タスク","titleField":"title","fields":[{"id":"title","name":"タスク","type":"string","required":true},
  {"id":"due","name":"期限","type":"date"},{"id":"done","name":"完了","type":"boolean","defaultValue":false}]}],
"screens":[
 {"id":"home","title":"モンスター","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"task_form","presentation":"sheet"}}],
  "components":[{"type":"stat","label":"モンスターの HP","value":` + jp2HPExpr + `},
   {"type":"list","entity":"task","item":{"title":"title","subtitle":"due","toggle":"done"},"onItemTap":{"type":"navigate","screen":"task_form"}}]},
 {"id":"task_form","title":"タスク","entity":"task","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"task"}}],
  "components":[{"type":"textField","field":"title"},{"type":"datePicker","field":"due"},{"type":"toggle","field":"done","label":"完了"}]}]}`,

	"own done": jp2HeadPrefix + `"entities":[
 {"id":"assignment","name":"課題","scope":"shared","titleField":"title","marks":` + jp2MarksDecl + `,"fields":[{"id":"title","name":"課題","type":"string","required":true},{"id":"due","name":"締め切り","type":"date"}]}],
"screens":[
 {"id":"home","title":"課題","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"assignment_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"assignment","filter":` + jp2NotMine + `,"item":{"title":"title","subtitle":"due","mark":"done"},"onItemTap":{"type":"navigate","screen":"assignment_form"}},
   {"type":"list","entity":"assignment","item":{"title":"title","subtitle":"due","mark":"done"},"onItemTap":{"type":"navigate","screen":"assignment_form"}}]},
 {"id":"assignment_form","title":"課題","entity":"assignment","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"assignment"}}],
  "components":[{"type":"textField","field":"title"},{"type":"datePicker","field":"due"}]}]}`,

	"prev task": jp2HeadPrefix + `"entities":[
 {"id":"task","name":"作業","titleField":"title","fields":[{"id":"title","name":"作業","type":"string","required":true},
  {"id":"prev","name":"前の作業","type":"ref","refEntity":"task"},{"id":"done","name":"済み","type":"boolean","defaultValue":false},
  {"id":"ready","name":"着手できる","type":"boolean","computed":` + jp2ReadyExpr + `}]}],
"screens":[
 {"id":"home","title":"作業","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"task_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"task","filter":` + jp2ReadyFilt + `,"item":{"title":"title","toggle":"done"},"onItemTap":{"type":"navigate","screen":"task_form"}},
   {"type":"list","entity":"task","item":{"title":"title","subtitle":"prev","trailing":"ready","toggle":"done"},"onItemTap":{"type":"navigate","screen":"task_form"}}]},
 {"id":"task_form","title":"作業","entity":"task","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"task"}}],
  "components":[{"type":"textField","field":"title"},{"type":"picker","field":"prev"},{"type":"toggle","field":"done","label":"済み"}]}]}`,

	"same feeling": jp2HeadPrefix + `"entities":[
 {"id":"post","name":"気分","scope":"public","titleField":"feeling","fields":[{"id":"feeling","name":"気分","type":"enum","options":["うれしい","ふつう","かなしい","イライラ"],"required":true},
  {"id":"comment","name":"ひとこと","type":"string"},{"id":"date","name":"日付","type":"date","defaultValue":"now"},
  {"id":"same_today","name":"今日同じ気分の人","type":"integer","unit":"人","computed":` + jp2SameExpr + `}]}],
"screens":[
 {"id":"home","title":"気分","toolbar":[{"label":"投稿","action":{"type":"navigate","screen":"post_form","presentation":"sheet"}}],
  "components":[{"type":"chart","kind":"bar","entity":"post","groupBy":"feeling","aggregate":"count","filter":{"field":"date","op":"today","value":null}},
   {"type":"list","entity":"post","item":{"title":"feeling","subtitle":"comment","trailing":"same_today"},"onItemTap":{"type":"navigate","screen":"post_form"}}]},
 {"id":"post_form","title":"気分","entity":"post","toolbar":[{"label":"投稿","action":{"type":"saveRecord","entity":"post"}}],
  "components":[{"type":"picker","field":"feeling"},{"type":"textField","field":"comment"}]}]}`,

	"days since": jp2HeadPrefix + `"entities":[
 {"id":"friend","name":"友達","titleField":"name","fields":[{"id":"name","name":"名前","type":"string","required":true},
  {"id":"last_sent","name":"最後に送った日","type":"date","computed":` + jp2LastExpr + `},
  {"id":"days_since","name":"送ってから","type":"integer","unit":"日","computed":` + jp2SinceExpr + `}]},
 {"id":"card","name":"カード","titleField":"message","fields":[{"id":"friend","name":"相手","type":"ref","refEntity":"friend","required":true},
  {"id":"date","name":"日付","type":"date","defaultValue":"now"},{"id":"message","name":"メッセージ","type":"string"}]}],
"screens":[
 {"id":"home","title":"友達","toolbar":[{"label":"友達を追加","action":{"type":"navigate","screen":"friend_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"friend","sort":{"field":"days_since","direction":"desc"},"item":{"title":"name","trailing":"last_sent"},"onItemTap":{"type":"navigate","screen":"card_form","presentation":"sheet","prefillRef":"friend"}},
   {"type":"list","entity":"card","item":{"title":"friend","subtitle":"message"},"onItemTap":{"type":"navigate","screen":"card_form"}},
   {"type":"button","label":"友達を直す","action":{"type":"navigate","screen":"friend_list"}}]},
 {"id":"friend_list","title":"友達の一覧","components":[{"type":"list","entity":"friend","item":{"title":"name"},"onItemTap":{"type":"navigate","screen":"friend_form"}}]},
 {"id":"friend_form","title":"友達","entity":"friend","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"friend"}}],
  "components":[{"type":"textField","field":"name"}]},
 {"id":"card_form","title":"カード","entity":"card","toolbar":[{"label":"送った","action":{"type":"saveRecord","entity":"card"}}],
  "components":[{"type":"picker","field":"friend"},{"type":"datePicker","field":"date"},{"type":"textField","field":"message"}]}]}`,

	"diagnosis": jp2HeadPrefix + `"entities":[
 {"id":"answer","name":"診断","titleField":"bean","fields":[{"id":"acidity","name":"酸味","type":"enum","options":["好き","苦手"],"required":true},
  {"id":"bean","name":"おすすめの豆","type":"string","computed":` + jp2DiagExpr + `}]}],
"screens":[
 {"id":"home","title":"診断","toolbar":[{"label":"診断する","action":{"type":"navigate","screen":"answer_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"answer","item":{"title":"bean","subtitle":"acidity"},"onItemTap":{"type":"navigate","screen":"answer_form"}}]},
 {"id":"answer_form","title":"質問","entity":"answer","toolbar":[{"label":"診断する","action":{"type":"saveRecord","entity":"answer"}}],
  "components":[{"type":"picker","field":"acidity"}]}]}`,
}

func TestJPHACKSProducts2Rules_AppsValidate(t *testing.T) {
	for name, raw := range jp2RuleApps {
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

func TestJPHACKSProducts2Rules_ExprsEvaluate(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	rec := func(id string, data map[string]any) expreval.Record { return expreval.Record{ID: id, Data: data} }
	setup := func(name string, recs map[string][]expreval.Record) (*dsl.App, *expreval.Evaluator) {
		app, err := dsl.Parse([]byte(jp2RuleApps[name]))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		src := expreval.SourceFunc(func(id string) []expreval.Record { return recs[id] })
		return app, expreval.New(expreval.Env{Entities: app.Entities, Source: src, Now: now})
	}
	expr := func(raw string) dsl.Expr {
		var e dsl.Expr
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	// field evaluates a (computed) field of one record.
	field := func(app *dsl.App, ev *expreval.Evaluator, entity, fid string, r expreval.Record) expreval.Value {
		return ev.FieldValue(fid, &expreval.Row{Entity: app.EntityByID(entity), Record: r})
	}

	{ // monster hp: 100 - 10*done + 10*overdue, clamped
		recs := map[string][]expreval.Record{"task": {
			rec("1", map[string]any{"title": "a", "done": true, "due": "2026-10-20"}),
			rec("2", map[string]any{"title": "b", "done": true}),
			rec("3", map[string]any{"title": "c", "done": false, "due": "2026-10-24"}), // overdue
			rec("4", map[string]any{"title": "d", "done": false, "due": "2026-10-25"}), // due today: not overdue
			rec("5", map[string]any{"title": "e", "done": false, "due": "2026-10-30"}),
		}}
		_, ev := setup("monster hp", recs)
		if v := ev.Eval(expr(jp2HPExpr), nil); v.Kind != expreval.Number || v.Num != 90 {
			t.Errorf("hp = %v, want 90", v)
		}
		_, ev = setup("monster hp", nil)
		if v := ev.Eval(expr(jp2HPExpr), nil); v.Kind != expreval.Number || v.Num != 100 {
			t.Errorf("hp with no tasks = %v, want 100", v)
		}
	}

	{ // own done: the "not mine" filter keeps what I have not marked, whoever else did
		recs := map[string][]expreval.Record{"assignment": {
			{ID: "1", Data: map[string]any{"title": "a"}, Marks: map[string]expreval.Mark{"done": {Count: 2, Mine: true}}},
			{ID: "2", Data: map[string]any{"title": "b"}, Marks: map[string]expreval.Mark{"done": {Count: 3, Mine: false}}},
			{ID: "3", Data: map[string]any{"title": "c"}},
		}}
		_, ev := setup("own done", recs)
		var f dsl.Filter
		if err := json.Unmarshal([]byte(jp2NotMine), &f); err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, r := range ev.Matching("assignment", []dsl.Filter{f}, nil) {
			ids = append(ids, r.ID)
		}
		if strings.Join(ids, ",") != "2,3" {
			t.Errorf("not-mine filter keeps %v, want 2,3", ids)
		}
	}

	{ // prev task: ready when not done yet and there is no prev or the prev is done
		recs := map[string][]expreval.Record{"task": {
			rec("1", map[string]any{"title": "design", "done": true}),
			rec("2", map[string]any{"title": "build", "done": false, "prev": "1"}),
			rec("3", map[string]any{"title": "test", "done": false, "prev": "2"}),
			rec("4", map[string]any{"title": "solo", "done": false}),
		}}
		app, ev := setup("prev task", recs)
		want := map[string]bool{"1": false, "2": true, "3": false, "4": true}
		for _, r := range recs["task"] {
			v := field(app, ev, "task", "ready", r)
			if v.Kind != expreval.Bool || v.Bool != want[r.ID] {
				t.Errorf("ready(%s) = %v, want %v", r.ID, v, want[r.ID])
			}
		}
		// List filters read computed fields (expressions.md §4). Matching reads stored
		// values only, so check the same filter through agg count, which evaluates computed.
		if v := ev.Eval(expr(`{"agg":"count","entity":"task","filter":`+jp2ReadyFilt+`}`), nil); v.Kind != expreval.Number || v.Num != 2 {
			t.Errorf("今できる count = %v, want 2 (build, solo)", v)
		}
	}

	{ // same feeling today, counting the row itself
		recs := map[string][]expreval.Record{"post": {
			rec("1", map[string]any{"feeling": "うれしい", "date": "2026-10-25"}),
			rec("2", map[string]any{"feeling": "うれしい", "date": "2026-10-25"}),
			rec("3", map[string]any{"feeling": "うれしい", "date": "2026-10-24"}),
			rec("4", map[string]any{"feeling": "かなしい", "date": "2026-10-25"}),
		}}
		app, ev := setup("same feeling", recs)
		for id, want := range map[int]float64{0: 2, 3: 1} {
			v := field(app, ev, "post", "same_today", recs["post"][id])
			if v.Kind != expreval.Number || v.Num != want {
				t.Errorf("same_today(%s) = %v, want %v", recs["post"][id].ID, v, want)
			}
		}
	}

	{ // days since the last card to each friend
		recs := map[string][]expreval.Record{
			"friend": {rec("a", map[string]any{"name": "A"}), rec("b", map[string]any{"name": "B"}), rec("c", map[string]any{"name": "C"})},
			"card": {
				rec("1", map[string]any{"friend": "a", "date": "2026-09-01"}),
				rec("2", map[string]any{"friend": "a", "date": "2026-10-15"}),
				rec("3", map[string]any{"friend": "b", "date": "2026-08-26"}),
			},
		}
		app, ev := setup("days since", recs)
		for id, want := range map[int]float64{0: 10, 1: 60} {
			v := field(app, ev, "friend", "days_since", recs["friend"][id])
			if v.Kind != expreval.Number || v.Num != want {
				t.Errorf("days_since(%s) = %v, want %v", recs["friend"][id].ID, v, want)
			}
		}
		if v := field(app, ev, "friend", "days_since", recs["friend"][2]); v.Kind != expreval.Number || v.Num != 9999 {
			t.Errorf("days_since with no card = %v, want 9999", v)
		}
		if v := field(app, ev, "friend", "last_sent", recs["friend"][2]); !v.IsNil() {
			t.Errorf("last_sent with no card = %v, want blank (the trailing column)", v)
		}
		// sort days_since desc: the never-contacted friend comes first
		order := []expreval.Record{recs["friend"][0], recs["friend"][1], recs["friend"][2]}
		sort.SliceStable(order, func(i, j int) bool {
			return field(app, ev, "friend", "days_since", order[i]).Num > field(app, ev, "friend", "days_since", order[j]).Num
		})
		if got := order[0].ID + order[1].ID + order[2].ID; got != "cba" {
			t.Errorf("days_since desc order = %s, want cba (no card first)", got)
		}
	}

	{ // diagnosis
		app, ev := setup("diagnosis", nil)
		for ans, want := range map[string]string{"好き": "エチオピア(浅煎り)", "苦手": "マンデリン(深煎り)"} {
			v := field(app, ev, "answer", "bean", rec(ans, map[string]any{"acidity": ans})) // one id each: computed values are memoized by id
			if v.Kind != expreval.String || v.Str != want {
				t.Errorf("bean(%s) = %v, want %s", ans, v, want)
			}
		}
	}
}
