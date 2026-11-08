package ai

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// The expressions exactly as jphacksProducts4Rules writes them.
const (
	jp4CO2Expr      = `{"op":"mul","args":[{"ref":"count"},{"if":{"op":"eq","args":[{"ref":"kind"},"ペットボトル"]},"then":30,"else":{"if":{"op":"eq","args":[{"ref":"kind"},"缶"]},"then":40,"else":{"if":{"op":"eq","args":[{"ref":"kind"},"紙"]},"then":10,"else":20}}}]}`
	jp4CO2MonthExpr = `{"agg":"sum","entity":"trash","field":"co2","filter":{"field":"date","op":"thisMonth","value":null}}`
	jp4TodayFilter  = `{"field":"wd","op":"eq","value":{"op":"weekday","args":[{"op":"today","args":[]}]}}`
	jp4WdExpr       = `{"if":{"op":"eq","args":[{"ref":"weekday"},"月"]},"then":2,"else":{"if":{"op":"eq","args":[{"ref":"weekday"},"火"]},"then":3,"else":{"if":{"op":"eq","args":[{"ref":"weekday"},"水"]},"then":4,"else":{"if":{"op":"eq","args":[{"ref":"weekday"},"木"]},"then":5,"else":{"if":{"op":"eq","args":[{"ref":"weekday"},"金"]},"then":6,"else":0}}}}}`
	jp4KcalExpr     = `{"op":"round","args":[{"op":"mul","args":[{"op":"add","args":[{"op":"sub","args":[{"op":"add","args":[{"op":"mul","args":[10,{"ref":"weight"}]},{"op":"mul","args":[6.25,{"ref":"height"}]}]},{"op":"mul","args":[5,{"ref":"age"}]}]},{"if":{"op":"eq","args":[{"ref":"sex"},"男性"]},"then":5,"else":-161}]},{"if":{"op":"eq","args":[{"ref":"activity"},"低い"]},"then":1.5,"else":{"if":{"op":"eq","args":[{"ref":"activity"},"ふつう"]},"then":1.75,"else":2}}]}]}`
	jp4LeftExpr     = `{"op":"sub","args":[{"agg":"latest","entity":"body","field":"kcal"},{"agg":"sum","entity":"meal","field":"kcal","filter":{"field":"date","op":"today","value":null}}]}`
	jp4AvgOnExpr    = `{"op":"div","args":[{"op":"round","args":[{"op":"mul","args":[{"agg":"avg","entity":"day","field":"score","filter":{"field":"drinks","op":"gt","value":0}},10]}]},10]}`
	jp4AvgOffExpr   = `{"op":"div","args":[{"op":"round","args":[{"op":"mul","args":[{"agg":"avg","entity":"day","field":"score","filter":{"field":"drinks","op":"eq","value":0}},10]}]},10]}`
	jp4LevelPrompt  = `次の英文を、英語力が {{level}} くらいの人が読めるように、難しい語だけ日本語に置き換えた日英まじりの文にする: {{input}}`
	jp4MeetPrompt   = `会議の文字起こし {{input}} を、論点・決まったこと・結論が出ないまま同じ話が続いた所(停滞)に分けて書く`
	jp4Head         = `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home",`
)

func TestJPHACKSProducts4Rules_InPrompt(t *testing.T) {
	// jp4AvgOffExpr is "同じ形で filter ..." in the prompt, so only its shape is checked.
	for _, s := range []string{jp4CO2Expr, jp4CO2MonthExpr, jp4TodayFilter, jp4WdExpr, jp4KcalExpr, jp4LeftExpr, jp4AvgOnExpr, jp4LevelPrompt, jp4MeetPrompt} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("systemPrompt lacks %s", s)
		}
	}
	for _, s := range []string{jp4CO2Expr, jp4CO2MonthExpr, jp4TodayFilter, jp4WdExpr, jp4KcalExpr, jp4LeftExpr, jp4AvgOnExpr, jp4AvgOffExpr} {
		if !json.Valid([]byte(s)) {
			t.Errorf("not valid JSON: %s", s)
		}
	}
}

func jp4Str(s string) string { b, _ := json.Marshal(s); return string(b) }

// The smallest app that follows each rule. They must pass dsl.Parse with no lint.
var jp4RuleApps = map[string]string{
	"level": jp4Head + `"entities":[
 {"id":"reading","name":"英文","titleField":"original","fields":[{"id":"original","name":"英文","type":"text","required":true},
  {"id":"level","name":"英語力","type":"enum","options":["中学生","高校生","大学生"]},
  {"id":"result","name":"言い換え","type":"text"}]}],
"screens":[
 {"id":"home","title":"英文","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"reading_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"reading","item":{"title":"original","subtitle":"level"},"onItemTap":{"type":"navigate","screen":"reading_form"}}]},
 {"id":"reading_form","title":"英文","entity":"reading","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"reading"}}],
  "components":[{"type":"textField","field":"original","multiline":true},{"type":"picker","field":"level"},
   {"type":"aiButton","task":"rewrite","input":"original","output":"result","length":"long","label":"言い換える","prompt":` + jp4Str(jp4LevelPrompt) + `},
   {"type":"textField","field":"result","multiline":true}]}]}`,

	"meeting": jp4Head + `"entities":[
 {"id":"meeting","name":"会議","scope":"shared","titleField":"title","fields":[{"id":"title","name":"会議","type":"string","required":true},
  {"id":"date","name":"日時","type":"datetime","defaultValue":"now"},
  {"id":"transcript","name":"文字起こし","type":"text"},{"id":"summary","name":"振り返り","type":"text"}]}],
"screens":[
 {"id":"home","title":"会議","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"meeting_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"meeting","sort":{"field":"date","direction":"desc"},"item":{"title":"title","subtitle":"date"},"onItemTap":{"type":"navigate","screen":"meeting_form"}}]},
 {"id":"meeting_form","title":"会議","entity":"meeting","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"meeting"}}],
  "components":[{"type":"textField","field":"title"},{"type":"transcribeField","field":"transcript","label":"会議の声を文字に"},
   {"type":"aiButton","task":"summarize","input":"transcript","output":"summary","length":"long","label":"振り返る","prompt":` + jp4Str(jp4MeetPrompt) + `},
   {"type":"textField","field":"summary","multiline":true}]}]}`,

	"co2": jp4Head + `"entities":[
 {"id":"trash","name":"ゴミ","titleField":"kind","fields":[{"id":"kind","name":"種類","type":"enum","options":["ペットボトル","缶","紙","プラ"],"required":true},
  {"id":"count","name":"数","type":"integer","defaultValue":1},
  {"id":"date","name":"日付","type":"date","defaultValue":"now"},
  {"id":"photo","name":"写真","type":"photo"},
  {"id":"co2","name":"CO2 削減","type":"number","unit":"g","computed":` + jp4CO2Expr + `}]}],
"screens":[
 {"id":"home","title":"リサイクル","toolbar":[{"label":"記録","action":{"type":"navigate","screen":"trash_form","presentation":"sheet"}}],
  "components":[{"type":"stat","label":"今月の CO2 削減(g)","value":` + jp4CO2MonthExpr + `},
   {"type":"list","entity":"trash","item":{"title":"kind","subtitle":"date","trailing":"co2","image":"photo"},"onItemTap":{"type":"navigate","screen":"trash_form"}}]},
 {"id":"trash_form","title":"ゴミ","entity":"trash","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"trash"}}],
  "components":[{"type":"photoField","field":"photo","source":"camera","analyze":[{"kind":"classify","into":"kind"}]},{"type":"picker","field":"kind"},
   {"type":"numberField","field":"count"},{"type":"datePicker","field":"date"}]}]}`,

	"today": jp4Head + `"entities":[
 {"id":"lecture","name":"授業","titleField":"subject","fields":[{"id":"subject","name":"授業","type":"string","required":true},
  {"id":"weekday","name":"曜日","type":"enum","options":["月","火","水","木","金"],"required":true},
  {"id":"period","name":"時限","type":"integer"},{"id":"room","name":"教室","type":"string"},{"id":"items","name":"持ち物","type":"string"},
  {"id":"wd","name":"曜日の番号","type":"integer","computed":` + jp4WdExpr + `}]}],
"screens":[
 {"id":"home","title":"今日","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"lecture_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"lecture","filter":` + jp4TodayFilter + `,"sort":{"field":"period","direction":"asc"},"item":{"title":"subject","subtitle":"room","trailing":"items"},"onItemTap":{"type":"navigate","screen":"lecture_form"}},
   {"type":"button","label":"週の時間割","action":{"type":"navigate","screen":"week"}}]},
 {"id":"week","title":"時間割","components":[{"type":"gridTable","entity":"lecture","columnField":"weekday","rowField":"period","rowRange":{"from":1,"to":6},"titleField":"subject","onItemTap":{"type":"navigate","screen":"lecture_form"}}]},
 {"id":"lecture_form","title":"授業","entity":"lecture","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"lecture"}}],
  "components":[{"type":"textField","field":"subject"},{"type":"picker","field":"weekday"},{"type":"numberField","field":"period"},
   {"type":"textField","field":"room"},{"type":"textField","field":"items"}]}]}`,

	"kcal": jp4Head + `"entities":[
 {"id":"body","name":"自分","titleField":"sex","fields":[{"id":"sex","name":"性別","type":"enum","options":["男性","女性"],"required":true},
  {"id":"age","name":"年齢","type":"integer","unit":"歳"},{"id":"height","name":"身長","type":"number","unit":"cm"},{"id":"weight","name":"体重","type":"number","unit":"kg"},
  {"id":"activity","name":"活動","type":"enum","options":["低い","ふつう","高い"]},
  {"id":"kcal","name":"必要なカロリー","type":"integer","unit":"kcal","computed":` + jp4KcalExpr + `}]},
 {"id":"meal","name":"食事","titleField":"name","fields":[{"id":"name","name":"食事","type":"string","required":true},
  {"id":"kcal","name":"カロリー","type":"integer","unit":"kcal"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]}],
"screens":[
 {"id":"home","title":"献立","toolbar":[{"label":"食事","action":{"type":"navigate","screen":"meal_form","presentation":"sheet"}}],
  "components":[{"type":"stat","label":"今日の残り(kcal)","value":` + jp4LeftExpr + `},
   {"type":"aiButton","task":"generate","entity":"body","limit":1,"prompt":"{{input}} の人が 1 日にとる目安のカロリーに合う献立","shape":"fields","items":["朝","昼","夜"]},
   {"type":"list","entity":"meal","item":{"title":"name","subtitle":"date","trailing":"kcal"},"onItemTap":{"type":"navigate","screen":"meal_form"}},
   {"type":"list","entity":"body","item":{"title":"sex","trailing":"kcal"},"onItemTap":{"type":"navigate","screen":"body_form"}},
   {"type":"button","label":"自分の情報","action":{"type":"navigate","screen":"body_form","presentation":"sheet"}}]},
 {"id":"body_form","title":"自分","entity":"body","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"body"}}],
  "components":[{"type":"picker","field":"sex"},{"type":"numberField","field":"age"},{"type":"numberField","field":"height"},{"type":"numberField","field":"weight"},{"type":"picker","field":"activity"}]},
 {"id":"meal_form","title":"食事","entity":"meal","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"meal"}}],
  "components":[{"type":"textField","field":"name"},{"type":"numberField","field":"kcal"},{"type":"datePicker","field":"date"}]}]}`,

	"compare": jp4Head + `"entities":[
 {"id":"day","name":"1 日","titleField":"date","fields":[{"id":"date","name":"日付","type":"date","defaultValue":"now"},
  {"id":"drinks","name":"エナドリ","type":"integer","unit":"本","defaultValue":0},{"id":"sleep","name":"睡眠","type":"number","unit":"時間"},
  {"id":"score","name":"作業の出来","type":"integer"}]}],
"screens":[
 {"id":"home","title":"記録","toolbar":[{"label":"記録","action":{"type":"navigate","screen":"day_form","presentation":"sheet"}}],
  "components":[{"type":"stat","label":"飲んだ日の出来","value":` + jp4AvgOnExpr + `},{"type":"stat","label":"飲まなかった日の出来","value":` + jp4AvgOffExpr + `},
   {"type":"chart","entity":"day","kind":"line","groupBy":"date","aggregate":"avg","field":"score","title":"出来"},
   {"type":"aiButton","task":"generate","entity":"day","limit":14,"prompt":"{{input}} の記録から、飲み方と睡眠についての助言"},
   {"type":"list","entity":"day","sort":{"field":"date","direction":"desc"},"item":{"title":"date","subtitle":"drinks","trailing":"score"},"onItemTap":{"type":"navigate","screen":"day_form"}}]},
 {"id":"day_form","title":"1 日","entity":"day","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"day"}}],
  "components":[{"type":"datePicker","field":"date"},{"type":"numberField","field":"drinks"},{"type":"numberField","field":"sleep"},{"type":"numberField","field":"score"}]}]}`,
}

func TestJPHACKSProducts4Rules_AppsValidate(t *testing.T) {
	for name, raw := range jp4RuleApps {
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

func TestJPHACKSProducts4Rules_ExprsEvaluate(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) // Wednesday
	rec := func(id string, data map[string]any) expreval.Record { return expreval.Record{ID: id, Data: data} }
	setup := func(name string, recs map[string][]expreval.Record, at time.Time) (*dsl.App, *expreval.Evaluator) {
		app, err := dsl.Parse([]byte(jp4RuleApps[name]))
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

	{ // co2: count × factor of the kind, summed this month
		ts := []expreval.Record{
			rec("1", map[string]any{"kind": "ペットボトル", "count": 2.0, "date": "2026-10-03"}),
			rec("2", map[string]any{"kind": "缶", "count": 1.0, "date": "2026-10-20"}),
			rec("3", map[string]any{"kind": "プラ", "count": 3.0, "date": "2026-10-21"}),
			rec("4", map[string]any{"kind": "紙", "count": 5.0, "date": "2026-09-30"}),
		}
		app, ev := setup("co2", map[string][]expreval.Record{"trash": ts}, now)
		for i, want := range []float64{60, 40, 60, 50} {
			if v := field(app, ev, "trash", "co2", ts[i]); !num(v, want) {
				t.Errorf("co2(%s) = %v, want %v", ts[i].ID, v, want)
			}
		}
		if v := ev.Eval(expr(jp4CO2MonthExpr), nil); !num(v, 160) {
			t.Errorf("month = %v, want 160", v)
		}
	}

	{ // today: only the lectures on today's weekday; none on weekends
		ls := []expreval.Record{
			rec("l1", map[string]any{"subject": "数学", "weekday": "水"}), rec("l2", map[string]any{"subject": "英語", "weekday": "水"}),
			rec("l3", map[string]any{"subject": "体育", "weekday": "金"}), rec("l4", map[string]any{"subject": "国語", "weekday": "月"}),
		}
		recs := map[string][]expreval.Record{"lecture": ls}
		count := expr(`{"agg":"count","entity":"lecture","filter":` + jp4TodayFilter + `}`)
		for _, c := range []struct {
			days int
			want float64
		}{{0, 2}, {2, 1}, {5, 1}, {4, 0}} { // Wed, Fri, Mon, Sun
			_, ev := setup("today", recs, now.AddDate(0, 0, c.days))
			if v := ev.Eval(count, nil); !num(v, c.want) {
				t.Errorf("today(+%d) = %v, want %v", c.days, v, c.want)
			}
		}
	}

	{ // kcal: Mifflin-St Jeor × activity; what is left today
		bs := []expreval.Record{
			rec("b1", map[string]any{"sex": "男性", "age": 20.0, "height": 170.0, "weight": 60.0, "activity": "ふつう"}),
			rec("b2", map[string]any{"sex": "女性", "age": 30.0, "height": 160.0, "weight": 50.0, "activity": "低い"}),
		}
		// b1: 600+1062.5-100+5 = 1567.5 × 1.75 = 2743.125 → 2743; b2: 500+1000-150-161 = 1189 × 1.5 = 1783.5 → 1784
		app, ev := setup("kcal", map[string][]expreval.Record{"body": bs}, now)
		for i, want := range []float64{2743, 1784} {
			if v := field(app, ev, "body", "kcal", bs[i]); !num(v, want) {
				t.Errorf("kcal(%s) = %v, want %v", bs[i].ID, v, want)
			}
		}
		_, ev = setup("kcal", map[string][]expreval.Record{"body": bs[:1], "meal": {
			rec("m1", map[string]any{"name": "朝", "kcal": 500.0, "date": "2026-10-28"}),
			rec("m2", map[string]any{"name": "昼", "kcal": 800.0, "date": "2026-10-28"}),
			rec("m3", map[string]any{"name": "昨日", "kcal": 900.0, "date": "2026-10-27"}),
		}}, now)
		if v := ev.Eval(expr(jp4LeftExpr), nil); !num(v, 1443) {
			t.Errorf("left = %v, want 1443", v)
		}
	}

	{ // compare: average score on days with and without drinks
		ds := []expreval.Record{
			rec("1", map[string]any{"date": "2026-10-25", "drinks": 1.0, "score": 4.0}),
			rec("2", map[string]any{"date": "2026-10-26", "drinks": 2.0, "score": 3.0}),
			rec("3", map[string]any{"date": "2026-10-27", "drinks": 1.0, "score": 4.0}),
			rec("4", map[string]any{"date": "2026-10-28", "drinks": 0.0, "score": 2.0}),
			rec("5", map[string]any{"date": "2026-10-24", "drinks": 0.0, "score": 5.0}),
		}
		_, ev := setup("compare", map[string][]expreval.Record{"day": ds}, now)
		if v := ev.Eval(expr(jp4AvgOnExpr), nil); !num(v, 3.7) { // 11/3
			t.Errorf("on = %v, want 3.7", v)
		}
		if v := ev.Eval(expr(jp4AvgOffExpr), nil); !num(v, 3.5) {
			t.Errorf("off = %v, want 3.5", v)
		}
	}
}
