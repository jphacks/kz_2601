// ai-audit scores ai-eval raw runs on *function*, not shape: dead buttons,
// unreachable/broken screens, entities with no way to add or show records,
// stat labels that promise a period/calculation the DSL does not compute, and
// whether each case's required capabilities (`needs` in audit_cases.json) are
// present. It decodes the DSL loosely (map[string]any) so the same tool can
// read runs produced by branches that extend the schema (computed / stat.value
// / settlement from #100). No API calls.
//
//	go run ./cmd/ai-audit -cases internal/ai/eval/audit_cases.json -run ../docs/backend/ai-eval/raw/<file>.json
//	go run ./cmd/ai-audit ... -compare ../docs/backend/ai-eval/raw/<other>.json   # adds a before/after column
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
)

type auditCase struct {
	ID         string   `json:"id"`
	Transcript string   `json:"transcript"`
	Needs      []string `json:"needs"`
}

type runFile struct {
	Model string `json:"model"`
	Tag   string `json:"tag"`
	Cases []struct {
		ID    string          `json:"id"`
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		DSL   json.RawMessage `json:"dsl"`
	} `json:"cases"`
}

type obj = map[string]any

func str(m obj, k string) string { s, _ := m[k].(string); return s }
func arr(m obj, k string) []any  { a, _ := m[k].([]any); return a }
func sub(m obj, k string) obj    { o, _ := m[k].(obj); return o }

// app is a loosely decoded AppDSL with lookup tables.
type app struct {
	raw      obj
	screens  map[string]obj
	entities map[string]obj
	fields   map[string]map[string]obj // entity -> field id -> field
	comps    []comp                    // every component incl. container children
}

type comp struct {
	screen string
	c      obj
}

func load(raw json.RawMessage) (*app, error) {
	var root obj
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	a := &app{raw: root, screens: map[string]obj{}, entities: map[string]obj{}, fields: map[string]map[string]obj{}}
	for _, e := range arr(root, "entities") {
		eo, _ := e.(obj)
		id := str(eo, "id")
		a.entities[id] = eo
		a.fields[id] = map[string]obj{}
		for _, f := range arr(eo, "fields") {
			fo, _ := f.(obj)
			a.fields[id][str(fo, "id")] = fo
		}
	}
	for _, s := range arr(root, "screens") {
		so, _ := s.(obj)
		a.screens[str(so, "id")] = so
		for _, c := range arr(so, "components") {
			co, _ := c.(obj)
			a.comps = append(a.comps, comp{str(so, "id"), co})
			for _, ch := range arr(co, "children") {
				cho, _ := ch.(obj)
				a.comps = append(a.comps, comp{str(so, "id"), cho})
			}
		}
	}
	return a, nil
}

// actions returns every action with the screen it lives on (buttons, toolbar, onItemTap/onDateTap).
func (a *app) actions() []comp {
	var out []comp
	for id, s := range a.screens {
		for _, t := range arr(s, "toolbar") {
			if to, ok := t.(obj); ok {
				out = append(out, comp{id, sub(to, "action")})
			}
		}
	}
	for _, c := range a.comps {
		for _, k := range []string{"action", "onItemTap", "onDateTap"} {
			if act := sub(c.c, k); act != nil {
				out = append(out, comp{c.screen, act})
			}
		}
	}
	return out
}

func (a *app) has(pred func(obj) bool) bool {
	for _, c := range a.comps {
		if pred(c.c) {
			return true
		}
	}
	return false
}

func isType(types ...string) func(obj) bool {
	return func(c obj) bool {
		for _, t := range types {
			if str(c, "type") == t {
				return true
			}
		}
		return false
	}
}

// filterOps collects filter ops of the component, including filters nested in
// a stat.value Expr ({agg, filter}).
func filterOps(c obj) []string {
	var ops []string
	var walk func(v any, inFilter bool)
	walk = func(v any, inFilter bool) {
		switch x := v.(type) {
		case obj:
			if inFilter {
				if _, ok := x["field"]; ok {
					ops = append(ops, str(x, "op"))
				}
			}
			for k, y := range x {
				walk(y, inFilter || k == "filter")
			}
		case []any:
			for _, y := range x {
				walk(y, inFilter)
			}
		}
	}
	walk(c["filter"], true)
	walk(c["value"], false)
	return ops
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// hasComputed reports a computed field (#100) anywhere, or one matching pred.
func (a *app) hasComputed() bool {
	for _, fs := range a.fields {
		for _, f := range fs {
			if f["computed"] != nil {
				return true
			}
		}
	}
	return a.has(func(c obj) bool { return str(c, "type") == "stat" && c["value"] != nil })
}

// exprUses reports whether any Expr in the DSL uses one of the ops.
func exprUses(v any, ops ...string) bool {
	switch x := v.(type) {
	case obj:
		if op, ok := x["op"].(string); ok && contains(ops, op) {
			if _, isExpr := x["args"]; isExpr {
				return true
			}
		}
		for _, y := range x {
			if exprUses(y, ops...) {
				return true
			}
		}
	case []any:
		for _, y := range x {
			if exprUses(y, ops...) {
				return true
			}
		}
	}
	return false
}

// usesDoneInDays reports whether c computes doneInDays directly or reads a
// computed field that does (agg sum of last7).
func (a *app) usesDoneInDays(c obj) bool {
	if exprUses(c, "doneInDays") {
		return true
	}
	ids := map[string]bool{}
	for _, fs := range a.fields {
		for id, f := range fs {
			if exprUses(f["computed"], "doneInDays") {
				ids[id] = true
			}
		}
	}
	var mentions func(v any) bool
	mentions = func(v any) bool {
		switch x := v.(type) {
		case obj:
			if ids[str(x, "field")] {
				return true
			}
			for _, y := range x {
				if mentions(y) {
					return true
				}
			}
		case []any:
			for _, y := range x {
				if mentions(y) {
					return true
				}
			}
		}
		return false
	}
	return len(ids) > 0 && mentions(c)
}

// ---------- lint: mechanical defects ----------

var (
	reMonth    = regexp.MustCompile(`今月|this month`) // not "monthly": "Monthly budget" is a setting (#142)
	reToday    = regexp.MustCompile(`今日|本日|today`)
	reWeek     = regexp.MustCompile(`今週|this week`)
	reLast7    = regexp.MustCompile(`直近7日|直近 7 日|last 7 days|past 7 days|past week`)
	reYear     = regexp.MustCompile(`今年|年間|this year|year`)
	reValueish = regexp.MustCompile(`[0-9０-９]|—|\?`)
	reDerive   = regexp.MustCompile(`残り|あと|達成率|あたり|1人|一人|順位|ランキング|連続|BMI|精算|left|remaining|per |rank|streak|winning|%`)
)

func lint(a *app) []string {
	var out []string
	add := func(f string, args ...any) { out = append(out, fmt.Sprintf(f, args...)) }

	// navigation
	reached := map[string]bool{str(a.raw, "initialScreen"): true}
	for _, t := range arr(sub(a.raw, "app"), "tabs") {
		reached[str(t.(obj), "screen")] = true
	}
	saveOn := map[string]bool{}
	for _, act := range a.actions() {
		switch str(act.c, "type") {
		case "":
			add("dead_button: %s に type の無い action", act.screen)
		case "navigate":
			tgt := str(act.c, "screen")
			if _, ok := a.screens[tgt]; !ok {
				add("bad_nav: %s → 存在しない画面 %q", act.screen, tgt)
			}
			reached[tgt] = true
		case "saveRecord":
			saveOn[act.screen] = true
			if t := str(act.c, "screen"); t != "" {
				reached[t] = true
			}
		}
	}
	for id, s := range a.screens {
		if !reached[id] {
			add("unreachable: 画面 %s にどこからも行けない", id)
		}
		if e := str(s, "entity"); e != "" && !saveOn[id] {
			add("no_save: フォーム %s(entity %s)に saveRecord が無い", id, e)
		}
	}
	// entities: add path / display path / required inputs / never-input fields
	inputTypes := []string{"textField", "numberField", "datePicker", "picker", "toggle", "photoField", "slider", "stepper", "textEditor", "secureField", "timePicker", "colorPicker", "rating", "locationButton", "qrScanner", "timerDisplay"}
	shown := map[string]bool{}
	formOf := map[string][]string{}
	for id, s := range a.screens {
		if e := str(s, "entity"); e != "" {
			formOf[e] = append(formOf[e], id)
		}
	}
	for _, c := range a.comps {
		switch str(c.c, "type") {
		case "list", "table", "timeline", "calendar", "mapView", "settlement":
			shown[str(c.c, "entity")] = true
		}
	}
	ids := make([]string, 0, len(a.entities))
	for id := range a.entities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, e := range ids {
		forms := formOf[e]
		if len(forms) == 0 {
			add("no_add_form: entity %s を追加するフォーム画面が無い", e)
		} else {
			anyReach := false
			for _, f := range forms {
				anyReach = anyReach || reached[f]
			}
			if !anyReach {
				add("form_unreachable: entity %s のフォームに行けない", e)
			}
		}
		if !shown[e] {
			add("no_list: entity %s のレコードを一覧する部品が無い(入れたものが見えない)", e)
		}
		bound := map[string]bool{}
		for _, c := range a.comps {
			if contains(forms, c.screen) && contains(inputTypes, str(c.c, "type")) {
				bound[str(c.c, "field")] = true
			}
		}
		for fid, f := range a.fields[e] {
			if f["computed"] != nil || len(forms) == 0 {
				continue
			}
			if !bound[fid] {
				if r, _ := f["required"].(bool); r {
					add("required_no_input: %s.%s は必須なのに入力欄が無い(保存できない)", e, fid)
				} else if str(f, "type") != "date" && str(f, "type") != "datetime" {
					add("field_no_input: %s.%s に入力欄が無い(常に空)", e, fid)
				}
			}
		}
	}
	// labels vs filters / derived promises
	for _, c := range a.comps {
		t := str(c.c, "type")
		label := str(c.c, "label") + str(c.c, "title")
		if t == "text" || t == "label" || t == "markdown" {
			if reDerive.MatchString(str(c.c, "text")) && reValueish.MatchString(str(c.c, "text")) {
				add("static_value_text: %s の text %q は計算値に見えるが固定文字列", c.screen, str(c.c, "text"))
			}
			continue
		}
		if t != "stat" && t != "progressBar" && t != "gauge" && t != "chart" && t != "list" {
			continue
		}
		ops := filterOps(c.c)
		chk := func(re *regexp.Regexp, op, name string) {
			if re.MatchString(strings.ToLower(label)) && !contains(ops, op) {
				add("label_filter_mismatch: %s %s %q は%sと言うが filter %s が無い", c.screen, t, label, name, op)
			}
		}
		chk(reMonth, "thisMonth", "今月")
		chk(reToday, "today", "今日")
		chk(reWeek, "thisWeek", "今週")
		if !a.usesDoneInDays(c.c) { // doneInDays counts the last n days itself (#172)
			chk(reLast7, "gte", "直近7日")
		}
		if reYear.MatchString(strings.ToLower(label)) && !contains(ops, "thisYear") && !exprUses(c.c, "year") {
			add("label_filter_mismatch: %s %s %q は今年と言うが filter thisYear が無い(全期間になる)", c.screen, t, label)
		}
		if t == "stat" && c.c["value"] == nil && reDerive.MatchString(strings.ToLower(label)) {
			add("label_promises_calc: %s stat %q は単純集計(%s)しかしていない", c.screen, label, str(c.c, "aggregate"))
		}
	}
	// toggles whose state never changes anything visible
	for _, c := range a.comps {
		if str(c.c, "type") != "toggle" {
			continue
		}
		used := false
		for _, d := range a.comps {
			if f, ok := d.c["filter"].(obj); ok && str(f, "field") == str(c.c, "field") {
				used = true
			}
			if strings.Contains(fmt.Sprint(d.c["filter"]), str(c.c, "field")) || strings.Contains(fmt.Sprint(d.c["item"]), str(c.c, "field")) {
				used = true
			}
		}
		if !used {
			add("toggle_invisible: toggle %s は一覧の filter/表示に使われない(チェックしても何も変わらない)", str(c.c, "field"))
		}
	}
	sort.Strings(out)
	return out
}

// ---------- needs: capability coverage ----------

// mark is ○ / △ / ×.
func need(a *app, n string) (string, string) {
	hasType := func(t ...string) bool { return a.has(isType(t...)) }
	hasOp := func(op string) bool { return a.has(func(c obj) bool { return contains(filterOps(c), op) }) }
	switch n {
	case "agg":
		return yes(hasType("stat", "progressBar", "gauge", "chart"), "集計部品")
	case "filter_month":
		return yes(hasOp("thisMonth"), "filter thisMonth")
	case "filter_today":
		return yes(hasOp("today"), "filter today")
	case "filter_week", "filter_year":
		op := map[string]string{"filter_week": "thisWeek", "filter_year": "thisYear"}[n]
		if hasOp(op) || anyFilterOp(a.raw, op) || (n == "filter_year" && exprUses(a.raw, "year")) {
			return "○", "filter " + op
		}
		if exprUses(a.raw, "year", "daysBetween", "weekday", "addDays") {
			return "△", "日付関数で近似"
		}
		return "×", "週/年の filter が無い"
	case "toggle":
		if a.has(func(c obj) bool { return str(sub(c, "item"), "toggle") != "" }) {
			return "○", "list.item.toggle(一覧からワンタップ)"
		}
		if hasType("toggle") {
			return "△", "フォームの toggle のみ(一覧からチェックできない)"
		}
		return "×", "チェックが無い"
	case "tabs3":
		return yes(len(arr(sub(a.raw, "app"), "tabs")) >= 3, "タブ 3 つ以上")
	case "filter_bool":
		ok := a.has(func(c obj) bool {
			f := sub(c, "filter")
			if f == nil {
				return false
			}
			for e, fs := range a.fields {
				if str(c, "entity") == e && str(fs[str(f, "field")], "type") == "boolean" {
					return true
				}
			}
			return false
		})
		return yes(ok, "boolean で絞った一覧")
	case "notify":
		ok := len(arr(sub(a.raw, "app"), "notifications")) > 0
		for _, act := range a.actions() {
			ok = ok || str(act.c, "type") == "scheduleNotification"
		}
		return yes(ok, "通知")
	case "timer":
		return yes(hasType("timerDisplay"), "timerDisplay")
	case "timer_cycle":
		return "×", "集中→休憩の自動切替は DSL に無い(timer は単発)"
	case "chart":
		return yes(hasType("chart"), "chart")
	case "progress":
		return yes(hasType("progressBar", "gauge"), "進捗バー")
	case "sort_time":
		return yes(a.has(func(c obj) bool { return str(c, "type") == "list" && sub(c, "sort") != nil }), "list.sort")
	case "two_entities":
		return yes(len(a.entities) >= 2, "entity 2 つ")
	case "group_by", "group_by_month":
		if hasType("chart") && a.has(func(c obj) bool { return str(c, "groupBy") != "" || str(c, "bucket") != "" }) {
			return "△", "chart の groupBy/bucket のみ(数値の表は無い)"
		}
		return "×", "グループ別の集計が無い"
	case "group_by_member":
		if hasComputedAgg(a) {
			return "○", "メンバー行の computed 集計"
		}
		if hasType("chart") {
			return "△", "chart のみ"
		}
		return "×", "メンバーごとの合計が無い"
	case "ref":
		for _, fs := range a.fields {
			for _, f := range fs {
				if str(f, "type") == "ref" {
					return "○", "ref field"
				}
			}
		}
		return "×", "人を enum/文字列で持つ(メンバー entity と繋がらない)"
	case "settlement":
		return yes(hasType("settlement"), "settlement 部品")
	case "calc":
		return yes(a.hasComputed(), "computed / stat.value")
	case "remaining":
		if a.has(func(c obj) bool { return str(c, "type") == "stat" && c["value"] != nil }) {
			return "○", "stat.value"
		}
		if hasType("progressBar", "gauge") {
			return "△", "進捗バーのみ(残額の数値は出ない)"
		}
		return "×", "目標との差が出ない"
	case "days_until", "date_add":
		return yes(exprUses(a.raw, "daysUntil", "addDays", "daysBetween"), "日付の計算")
	case "time_diff":
		return yes(exprUses(a.raw, "hoursBetween"), "hoursBetween")
	case "filter_due":
		return yes(exprUses(a.raw, "daysUntil", "addDays", "today", "lte"), "期限での絞り込み")
	case "pb_detect":
		if hasComputedAgg(a) {
			return "○", "computed で max と比較"
		}
		if a.has(func(c obj) bool { return str(c, "aggregate") == "max" }) {
			return "△", "全体の max のみ(種目別・更新判定なし)"
		}
		return "×", "自己ベストが無い"
	case "rank":
		return "×", "順位(並べ替え/順位番号)を出す手段が無い"
	case "streak":
		if exprUses(a.raw, "streak") {
			return "○", "streak(毎日チェックの履歴)"
		}
		if exprUses(a.raw, "doneInDays") {
			return "△", "直近 N 日の回数のみ(連続日数は出ない)"
		}
		return "×", "連続日数を出していない"
	case "bulk_delete":
		return "×", "条件で一括削除するアクションが無い"
	}
	return "?", "未定義の need"
}

// anyFilterOp finds a filter {field, op} anywhere (e.g. in a computed agg).
func anyFilterOp(v any, op string) bool {
	switch x := v.(type) {
	case obj:
		if _, ok := x["field"]; ok && str(x, "op") == op {
			return true
		}
		for _, y := range x {
			if anyFilterOp(y, op) {
				return true
			}
		}
	case []any:
		for _, y := range x {
			if anyFilterOp(y, op) {
				return true
			}
		}
	}
	return false
}

func hasComputedAgg(a *app) bool {
	for _, fs := range a.fields {
		for _, f := range fs {
			if c := f["computed"]; c != nil && strings.Contains(fmt.Sprint(c), "agg:") {
				return true
			}
		}
	}
	return false
}

func yes(ok bool, what string) (string, string) {
	if ok {
		return "○", what
	}
	return "×", what + " が無い"
}

// ---------- main ----------

type result struct {
	ok    bool
	err   string
	lint  []string
	marks map[string][2]string
}

func evaluate(cases []auditCase, path string) (map[string]result, runFile) {
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var rf runFile
	if err := json.Unmarshal(raw, &rf); err != nil {
		log.Fatal(err)
	}
	res := map[string]result{}
	for _, rc := range rf.Cases {
		r := result{ok: rc.OK, err: rc.Error, marks: map[string][2]string{}}
		if rc.OK {
			a, err := load(rc.DSL)
			if err != nil {
				r.ok, r.err = false, err.Error()
			} else {
				r.lint = lint(a)
				for _, c := range cases {
					if c.ID != rc.ID {
						continue
					}
					for _, n := range c.Needs {
						m, why := need(a, n)
						r.marks[n] = [2]string{m, why}
					}
				}
			}
		}
		res[rc.ID] = r
	}
	return res, rf
}

func score(r result) (ok, part, ng int) {
	for _, m := range r.marks {
		switch m[0] {
		case "○":
			ok++
		case "△":
			part++
		default:
			ng++
		}
	}
	return
}

func main() {
	casesPath := flag.String("cases", "internal/ai/eval/audit_cases.json", "audit cases (with needs)")
	runPath := flag.String("run", "", "ai-eval raw run JSON")
	cmpPath := flag.String("compare", "", "optional second run (e.g. the expression branch) for a side-by-side column")
	flag.Parse()
	if *runPath == "" {
		log.Fatal("-run is required")
	}
	raw, err := os.ReadFile(*casesPath)
	if err != nil {
		log.Fatal(err)
	}
	var cases []auditCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		log.Fatal(err)
	}
	res, rf := evaluate(cases, *runPath)
	var cmp map[string]result
	if *cmpPath != "" {
		cmp, _ = evaluate(cases, *cmpPath)
	}

	fmt.Printf("## 機械判定(%s %s)\n\n", rf.Model, rf.Tag)
	fmt.Println("| id | 生成 | needs ○/△/× | 欠落した needs | lint 件数 |" + map[bool]string{true: " 比較 run ○/△/× |", false: ""}[cmp != nil])
	fmt.Println("|---|---|---|---|---|" + map[bool]string{true: "---|", false: ""}[cmp != nil])
	needTotals := map[string][3]int{}
	lintKinds := map[string]int{}
	for _, c := range cases {
		r := res[c.ID]
		gen := "OK"
		if !r.ok {
			gen = "FAIL"
		}
		ok, part, ng := score(r)
		var miss []string
		for _, n := range c.Needs {
			m := r.marks[n]
			t := needTotals[n]
			switch m[0] {
			case "○":
				t[0]++
			case "△":
				t[1]++
				miss = append(miss, n+"(△)")
			default:
				t[2]++
				miss = append(miss, n)
			}
			needTotals[n] = t
		}
		for _, l := range r.lint {
			lintKinds[strings.SplitN(l, ":", 2)[0]]++
		}
		line := fmt.Sprintf("| %s | %s | %d/%d/%d | %s | %d |", c.ID, gen, ok, part, ng, strings.Join(miss, ", "), len(r.lint))
		if cmp != nil {
			o2, p2, n2 := score(cmp[c.ID])
			line += fmt.Sprintf(" %d/%d/%d |", o2, p2, n2)
		}
		fmt.Println(line)
	}

	fmt.Println("\n## needs 別の充足(○/△/×)")
	fmt.Println("\n| need | ○ | △ | × |")
	fmt.Println("|---|---|---|---|")
	keys := make([]string, 0, len(needTotals))
	for k := range needTotals {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if needTotals[keys[i]][2] != needTotals[keys[j]][2] {
			return needTotals[keys[i]][2] > needTotals[keys[j]][2]
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		t := needTotals[k]
		fmt.Printf("| %s | %d | %d | %d |\n", k, t[0], t[1], t[2])
	}

	fmt.Println("\n## lint 種別の件数")
	fmt.Println("\n| 種別 | 件数 |")
	fmt.Println("|---|---|")
	lk := make([]string, 0, len(lintKinds))
	for k := range lintKinds {
		lk = append(lk, k)
	}
	sort.Slice(lk, func(i, j int) bool {
		return lintKinds[lk[i]] > lintKinds[lk[j]] || lintKinds[lk[i]] == lintKinds[lk[j]] && lk[i] < lk[j]
	})
	for _, k := range lk {
		fmt.Printf("| %s | %d |\n", k, lintKinds[k])
	}

	fmt.Println("\n## ケース別の詳細")
	for _, c := range cases {
		r := res[c.ID]
		fmt.Printf("\n### %s\n\n「%s」\n\n", c.ID, c.Transcript)
		if !r.ok {
			fmt.Printf("- 生成失敗: %s\n", r.err)
			continue
		}
		for _, n := range c.Needs {
			m := r.marks[n]
			fmt.Printf("- %s %s — %s\n", m[0], n, m[1])
		}
		for _, l := range r.lint {
			fmt.Printf("- lint %s\n", l)
		}
	}
}
