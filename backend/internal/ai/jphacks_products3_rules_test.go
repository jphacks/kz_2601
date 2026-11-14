package ai

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// The expressions exactly as jphacksProducts3Rules writes them.
const (
	jp3StreakExpr = `{"op":"streak","args":[{"ref":"done_on"}]}`
	jp3StageExpr  = `{"if":{"op":"gte","args":[{"ref":"streak"},21]},"then":"収穫できる","else":{"if":{"op":"gte","args":[{"ref":"streak"},7]},"then":"つぼみ","else":{"if":{"op":"gte","args":[{"ref":"streak"},1]},"then":"芽","else":"種"}}}`
	jp3VotesExpr  = `{"agg":"count","entity":"vote","filter":{"field":"shop","op":"eq","value":{"ref":"id"}}}`
	jp3TopExpr    = `{"agg":"latest","entity":"shop","field":"name","filter":[{"field":"votes","op":"eq","value":{"agg":"max","entity":"shop","field":"votes"}},{"field":"votes","op":"gt","value":0}]}`
	jp3FocusExpr  = `{"op":"round","args":[{"op":"div","args":[{"agg":"sum","entity":"session","field":"seconds","filter":{"field":"date","op":"today","value":null}},60]}]}`
	jp3LevelExpr  = `{"op":"add","args":[{"op":"floor","args":[{"op":"div","args":[{"agg":"count","entity":"meal"},5]}]},1]}`
	jp3NextExpr   = `{"op":"sub","args":[5,{"op":"mod","args":[{"agg":"count","entity":"meal"},5]}]}`
	jp3LenExpr    = `{"op":"len","args":[{"ref":"body"}]}`
	jp3YesExpr    = `{"agg":"count","entity":"check","filter":[{"field":"facility","op":"eq","value":{"ref":"id"}},{"field":"answer","op":"eq","value":"ある"}]}`
	jp3NoExpr     = `{"agg":"count","entity":"check","filter":[{"field":"facility","op":"eq","value":{"ref":"id"}},{"field":"answer","op":"eq","value":"ない"}]}`
	jp3StatusExpr = `{"if":{"op":"gt","args":[{"ref":"yes"},{"ref":"no"}]},"then":"ある","else":{"if":{"op":"gt","args":[{"ref":"no"},{"ref":"yes"}]},"then":"ない","else":"未確認"}}`
	jp3Head       = `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home",`
)

func TestJPHACKSProducts3Rules_InPrompt(t *testing.T) {
	// jp3NoExpr is "no も同じ形" in the prompt, so only its shape is checked (below).
	for _, s := range []string{jp3StreakExpr, jp3StageExpr, jp3VotesExpr, jp3TopExpr, jp3FocusExpr, jp3LevelExpr, jp3NextExpr, jp3LenExpr, jp3YesExpr, jp3StatusExpr} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("systemPrompt lacks %s", s)
		}
	}
	for _, s := range []string{jp3StreakExpr, jp3StageExpr, jp3VotesExpr, jp3TopExpr, jp3FocusExpr, jp3LevelExpr, jp3NextExpr, jp3LenExpr, jp3YesExpr, jp3NoExpr, jp3StatusExpr} {
		if !json.Valid([]byte(s)) {
			t.Errorf("not valid JSON: %s", s)
		}
	}
}

// The smallest app that follows each rule. They must pass dsl.Parse with no lint.
var jp3RuleApps = map[string]string{
	"plant": jp3Head + `"entities":[
 {"id":"habit","name":"習慣","titleField":"name","fields":[{"id":"name","name":"習慣","type":"string","required":true},
  {"id":"done_on","name":"やった日","type":"date"},
  {"id":"streak","name":"続けた日数","type":"integer","unit":"日","computed":` + jp3StreakExpr + `},
  {"id":"stage","name":"育ち具合","type":"string","computed":` + jp3StageExpr + `}]}],
"screens":[
 {"id":"home","title":"習慣","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"habit_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"habit","item":{"title":"name","subtitle":"stage","trailing":"streak","toggle":"done_on","resets":"daily"},"onItemTap":{"type":"navigate","screen":"habit_form"}}]},
 {"id":"habit_form","title":"習慣","entity":"habit","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"habit"}}],
  "components":[{"type":"textField","field":"name"}]}]}`,

	"vote": jp3Head + `"entities":[
 {"id":"shop","name":"お店","scope":"public","titleField":"name","fields":[{"id":"name","name":"お店","type":"string","required":true},
  {"id":"votes","name":"票","type":"integer","unit":"票","computed":` + jp3VotesExpr + `}]},
 {"id":"vote","name":"票","scope":"public","titleField":"shop","unique":["$author"],"fields":[{"id":"shop","name":"お店","type":"ref","refEntity":"shop","required":true}]}],
"screens":[
 {"id":"home","title":"投票","toolbar":[{"label":"候補を追加","action":{"type":"navigate","screen":"shop_form","presentation":"sheet"}}],
  "components":[{"type":"stat","label":"一番多いお店","value":` + jp3TopExpr + `},
   {"type":"list","entity":"shop","sort":{"field":"votes","direction":"desc"},"item":{"title":"name","trailing":"votes"},"onItemTap":{"type":"navigate","screen":"vote_form","presentation":"sheet","prefillRef":"shop"}},
   {"type":"button","label":"候補を直す","action":{"type":"navigate","screen":"shop_list"}}]},
 {"id":"shop_list","title":"候補","components":[{"type":"list","entity":"shop","item":{"title":"name"},"onItemTap":{"type":"navigate","screen":"shop_form"}},
   {"type":"list","entity":"vote","item":{"title":"shop","subtitle":"$author"},"onItemTap":{"type":"navigate","screen":"vote_form"}}]},
 {"id":"shop_form","title":"お店","entity":"shop","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"shop"}}],
  "components":[{"type":"textField","field":"name"}]},
 {"id":"vote_form","title":"投票","entity":"vote","toolbar":[{"label":"投票","action":{"type":"saveRecord","entity":"vote"}}],
  "components":[{"type":"picker","field":"shop"}]}]}`,

	"pomodoro": jp3Head + `"entities":[
 {"id":"session","name":"集中","scope":"shared","titleField":"date","fields":[{"id":"seconds","name":"集中した秒","type":"integer","unit":"秒"},
  {"id":"date","name":"日付","type":"date","defaultValue":"now"}]}],
"screens":[
 {"id":"home","title":"ポモドーロ","toolbar":[{"label":"始める","action":{"type":"navigate","screen":"session_form","presentation":"sheet"}}],
  "components":[{"type":"stat","label":"今日のみんなの集中(分)","value":` + jp3FocusExpr + `},
   {"type":"list","entity":"session","filter":{"field":"date","op":"today","value":null},"item":{"title":"date","subtitle":"$author","trailing":"seconds"},"onItemTap":{"type":"navigate","screen":"session_form"}}]},
 {"id":"session_form","title":"集中","entity":"session","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"session"}}],
  "components":[{"type":"timerDisplay","mode":"countdown","seconds":1500,"field":"seconds","label":"集中"}]}]}`,

	"level": jp3Head + `"entities":[
 {"id":"meal","name":"自炊","titleField":"name","fields":[{"id":"name","name":"料理","type":"string","required":true},
  {"id":"photo","name":"写真","type":"photo"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]}],
"screens":[
 {"id":"home","title":"自炊","toolbar":[{"label":"記録","action":{"type":"navigate","screen":"meal_form","presentation":"sheet"}}],
  "components":[{"type":"stat","label":"モンスターのレベル","value":` + jp3LevelExpr + `},
   {"type":"stat","label":"次のレベルまで(回)","value":` + jp3NextExpr + `},
   {"type":"list","entity":"meal","item":{"title":"name","subtitle":"date","image":"photo"},"onItemTap":{"type":"navigate","screen":"meal_form"}}]},
 {"id":"meal_form","title":"自炊","entity":"meal","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"meal"}}],
  "components":[{"type":"textField","field":"name"},{"type":"photoField","field":"photo","source":"camera"},{"type":"datePicker","field":"date"}]}]}`,

	"chars": jp3Head + `"entities":[
 {"id":"doc","name":"文書","titleField":"title","fields":[{"id":"title","name":"題名","type":"string","required":true},
  {"id":"body","name":"本文","type":"text"},
  {"id":"chars","name":"文字数","type":"integer","unit":"字","computed":` + jp3LenExpr + `}]}],
"screens":[
 {"id":"home","title":"文書","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"doc_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"doc","item":{"title":"title","trailing":"chars"},"onItemTap":{"type":"navigate","screen":"doc_form"}},
   {"type":"fileImport","entity":"doc","accept":["pdf","text"],"field":"body","label":"PDF を取り込む"}]},
 {"id":"doc_form","title":"文書","entity":"doc","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"doc"}}],
  "components":[{"type":"textField","field":"title"},{"type":"textEditor","field":"body"}]}]}`,

	"confirm": jp3Head + `"entities":[
 {"id":"facility","name":"設備","scope":"public","titleField":"name","fields":[{"id":"name","name":"設備","type":"string","required":true},
  {"id":"place","name":"場所","type":"location"},
  {"id":"yes","name":"ある","type":"integer","unit":"人","computed":` + jp3YesExpr + `},
  {"id":"no","name":"ない","type":"integer","unit":"人","computed":` + jp3NoExpr + `},
  {"id":"status","name":"判定","type":"string","computed":` + jp3StatusExpr + `}]},
 {"id":"check","name":"確認","scope":"public","titleField":"answer","unique":["facility","$author"],"fields":[{"id":"facility","name":"設備","type":"ref","refEntity":"facility","required":true},
  {"id":"answer","name":"あった?","type":"enum","options":["ある","ない"],"required":true}]}],
"screens":[
 {"id":"home","title":"設備","toolbar":[{"label":"投稿","action":{"type":"navigate","screen":"facility_form","presentation":"sheet"}}],
  "components":[{"type":"mapView","entity":"facility","locationField":"place","titleField":"name","onItemTap":{"type":"navigate","screen":"check_form","presentation":"sheet","prefillRef":"facility"}},
   {"type":"list","entity":"facility","item":{"title":"name","subtitle":"status"},"onItemTap":{"type":"navigate","screen":"check_form","presentation":"sheet","prefillRef":"facility"}},
   {"type":"button","label":"設備を直す","action":{"type":"navigate","screen":"facility_list"}}]},
 {"id":"facility_list","title":"設備の一覧","components":[{"type":"list","entity":"facility","item":{"title":"name"},"onItemTap":{"type":"navigate","screen":"facility_form"}},
   {"type":"list","entity":"check","item":{"title":"facility","subtitle":"answer"},"onItemTap":{"type":"navigate","screen":"check_form"}}]},
 {"id":"facility_form","title":"設備","entity":"facility","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"facility"}}],
  "components":[{"type":"textField","field":"name"},{"type":"locationButton","field":"place"}]},
 {"id":"check_form","title":"確認","entity":"check","toolbar":[{"label":"送る","action":{"type":"saveRecord","entity":"check"}}],
  "components":[{"type":"picker","field":"facility"},{"type":"picker","field":"answer"}]}]}`,
}

func TestJPHACKSProducts3Rules_AppsValidate(t *testing.T) {
	for name, raw := range jp3RuleApps {
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

func TestJPHACKSProducts3Rules_ExprsEvaluate(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	rec := func(id string, data map[string]any) expreval.Record { return expreval.Record{ID: id, Data: data} }
	setup := func(name string, recs map[string][]expreval.Record) (*dsl.App, *expreval.Evaluator) {
		app, err := dsl.Parse([]byte(jp3RuleApps[name]))
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
	field := func(app *dsl.App, ev *expreval.Evaluator, entity, fid string, r expreval.Record) expreval.Value {
		return ev.FieldValue(fid, &expreval.Row{Entity: app.EntityByID(entity), Record: r})
	}
	str := func(v expreval.Value, want string) bool { return v.Kind == expreval.String && v.Str == want }
	num := func(v expreval.Value, want float64) bool { return v.Kind == expreval.Number && v.Num == want }
	days := func(last string, n int) []any {
		end, _ := time.Parse("2006-01-02", last)
		var out []any
		for i := n - 1; i >= 0; i-- {
			out = append(out, end.AddDate(0, 0, -i).Format("2006-01-02"))
		}
		return out
	}

	{ // plant: the stage follows the streak
		recs := map[string][]expreval.Record{"habit": {
			rec("a", map[string]any{"name": "a"}),
			rec("b", map[string]any{"name": "b", "done_on": "2026-10-25", "done_on__dates": days("2026-10-25", 3)}),
			rec("c", map[string]any{"name": "c", "done_on": "2026-10-24", "done_on__dates": days("2026-10-24", 7)}), // today not yet: from yesterday
			rec("d", map[string]any{"name": "d", "done_on": "2026-10-25", "done_on__dates": days("2026-10-25", 21)}),
			rec("e", map[string]any{"name": "e", "done_on": "2026-10-22", "done_on__dates": days("2026-10-22", 30)}), // broken
		}}
		app, ev := setup("plant", recs)
		want := map[string]string{"a": "種", "b": "芽", "c": "つぼみ", "d": "収穫できる", "e": "種"}
		for _, r := range recs["habit"] {
			if v := field(app, ev, "habit", "stage", r); !str(v, want[r.ID]) {
				t.Errorf("stage(%s) = %v, want %s", r.ID, v, want[r.ID])
			}
		}
	}

	{ // vote: the shop with the most votes; none before anyone votes
		shops := []expreval.Record{rec("s1", map[string]any{"name": "ラーメン"}), rec("s2", map[string]any{"name": "カレー"}), rec("s3", map[string]any{"name": "そば"})}
		recs := map[string][]expreval.Record{"shop": shops, "vote": {
			rec("v1", map[string]any{"shop": "s2"}), rec("v2", map[string]any{"shop": "s2"}), rec("v3", map[string]any{"shop": "s1"}),
		}}
		app, ev := setup("vote", recs)
		if v := ev.Eval(expr(jp3TopExpr), nil); !str(v, "カレー") {
			t.Errorf("top = %v, want カレー", v)
		}
		if v := field(app, ev, "shop", "votes", shops[1]); !num(v, 2) {
			t.Errorf("votes(s2) = %v, want 2", v)
		}
		_, ev = setup("vote", map[string][]expreval.Record{"shop": shops})
		if v := ev.Eval(expr(jp3TopExpr), nil); !v.IsNil() {
			t.Errorf("top with no votes = %v, want blank", v)
		}
	}

	{ // pomodoro: today's seconds in minutes
		recs := map[string][]expreval.Record{"session": {
			rec("1", map[string]any{"seconds": 1500.0, "date": "2026-10-25"}),
			rec("2", map[string]any{"seconds": 1430.0, "date": "2026-10-25"}),
			rec("3", map[string]any{"seconds": 1500.0, "date": "2026-10-24"}),
		}}
		_, ev := setup("pomodoro", recs)
		if v := ev.Eval(expr(jp3FocusExpr), nil); !num(v, 49) { // 2930 s = 48.8 min
			t.Errorf("focus = %v, want 49", v)
		}
	}

	{ // level: +1 every 5 records, and how many to the next
		for n, want := range map[int][2]float64{0: {1, 5}, 4: {1, 1}, 5: {2, 5}, 12: {3, 3}} {
			var meals []expreval.Record
			for i := 0; i < n; i++ {
				meals = append(meals, rec(string(rune('a'+i)), map[string]any{"name": "x"}))
			}
			_, ev := setup("level", map[string][]expreval.Record{"meal": meals})
			if v := ev.Eval(expr(jp3LevelExpr), nil); !num(v, want[0]) {
				t.Errorf("level(%d) = %v, want %v", n, v, want[0])
			}
			if v := ev.Eval(expr(jp3NextExpr), nil); !num(v, want[1]) {
				t.Errorf("next(%d) = %v, want %v", n, v, want[1])
			}
		}
	}

	{ // chars: characters, not bytes
		app, ev := setup("chars", nil)
		if v := field(app, ev, "doc", "chars", rec("1", map[string]any{"title": "t", "body": "日本語の文。\nabc"})); !num(v, 10) {
			t.Errorf("chars = %v, want 10", v)
		}
	}

	{ // confirm: majority of the checks per facility
		fs := []expreval.Record{rec("f1", map[string]any{"name": "EV"}), rec("f2", map[string]any{"name": "トイレ"}), rec("f3", map[string]any{"name": "授乳室"})}
		recs := map[string][]expreval.Record{"facility": fs, "check": {
			rec("c1", map[string]any{"facility": "f1", "answer": "ある"}), rec("c2", map[string]any{"facility": "f1", "answer": "ある"}),
			rec("c3", map[string]any{"facility": "f1", "answer": "ない"}), rec("c4", map[string]any{"facility": "f2", "answer": "ない"}),
		}}
		app, ev := setup("confirm", recs)
		for i, want := range []string{"ある", "ない", "未確認"} {
			if v := field(app, ev, "facility", "status", fs[i]); !str(v, want) {
				t.Errorf("status(%s) = %v, want %s", fs[i].ID, v, want)
			}
		}
	}
}
