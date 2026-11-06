package ai

import (
	"encoding/json"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// singleRules (#267): one-of values (a cycle length, a goal, a budget) were a
// "settings" entity whose newest record was read with agg latest, so the user
// had to "add a setting" (d_period). They now live in an entity with
// "single": true (one record, edited on its form) and are read with
// {"setting": "<fieldId>"}. Appended like repeatRules so prompt.go /
// chat_first.go stay untouched; init() swaps prompt.go's "read settings with
// latest" sentence (restrict.go swaps it back for builds without
// FeatureSingleEntity).
var singleRules = strings.TrimSpace(`
## 設定値の置き場は single の entity(#267)
- **アプリ全体で 1 つだけの値**(周期の日数・目標体重・目標の歩数・月の予算・1 日の上限・貯金の目標額など)は、記録の entity とは別に **"single": true** の entity(id 例 settings・name「設定」)に field として置き、値の field に defaultValue(発話の数値・無ければよくある値: 生理の周期 28・歩数 8000・予算 30000)を付ける。「設定を追加」してレコードを増やす形・{"agg":"latest","entity":"settings",...} で読む形にしない。1 つのアプリで single の entity は 1 つにまとめる(field id は重ねない)。
- 読む: 式 **{"setting":"<fieldId>"}**。行が無い所(stat.value・progressBar.value・gauge.value・通知の when)でも、記録の entity の computed からでも書ける。例: 次の予定日 stat.value = {"op":"addDays","args":[{"agg":"latest","entity":"periods","field":"start_on","by":"start_on"},{"setting":"cycle_days"}]}、残り予算 = {"op":"sub","args":[{"setting":"budget"},{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"date","op":"thisMonth","value":null}}]}、達成率 progressBar.value = {"op":"div","args":[{"agg":"sum","entity":"steps","field":"count","filter":{"field":"date","op":"today","value":null}},{"setting":"daily_goal"}]}。
- 編集: single の entity のフォーム画面(screen.entity: settings・入力部品 + toolbar の saveRecord)を作り、記録の画面の toolbar か button の navigate(sheet・ラベル「周期の設定」「目標を変える」・icon gearshape)で開く。開くと今の値の「編集」になる(追加・削除は無い)。タブにするなら設定タブの根をそのフォームにしてよい。
- **使わない**(backend が拒否する): single の entity を list / table / timeline / calendar / chart / stat・progressBar・gauge の entity / aggregate に使う・式の agg・ref / refs の refEntity・deleteRecord・export・onItemTap の先。scope(shared / public)・retentionDays・hidden も付けない。記録(日々たまるもの・一覧で見るもの)や選ぶマスタ(カテゴリ・メンバー)は single にしない。
- 既存アプリへの「目標を○○にして」: 値を変えるのは利用者がフォームで行うので、defaultValue の replace だけにする(保存済みの値は変わらない、と summary に添える)。「周期を設定できるようにして」「予算を変えられるようにして」で古い形(設定の記録を latest で読む entity)があれば、その entity に "single": true を add し、その entity の list と onItemTap・deleteRecord を remove、{"agg":"latest","entity":<その entity>,"field":X,...} を {"setting":X} に replace する。
`)

// singleChatRule: the same capability for chat answers (できる?).
var singleChatRule = strings.TrimSpace(`
## 設定値(#267)
- 「周期を設定したい」「目標を変えられるようにしたい」「予算を決めておきたい」は**できる**(アプリに 1 つだけの値を「設定」の画面で編集し、計算や目標の表示に使う。記録のように追加していく形にはしない)。
`)

// noSingleChatRule replaces singleChatRule for builds without single.
const noSingleChatRule = "## 設定値(#267)\n- 周期・目標・予算のような値は、記録として入れた最新の値を使う(この版では「設定」の画面で 1 つの値を編集する形は使えない)。"

// prompt.go reads settings with latest; with single they are a setting.
const (
	settingsLatestLine  = "後から変える設定値(予算・目標)を entity に入れたら、その値は latest で読む(max だと下げた予算が反映されない)。"
	settingsSingleLine  = "後から変える設定値(予算・目標・周期)は single の entity に置き {\"setting\":\"<fieldId>\"} で読む(下の「設定値の置き場は single の entity」・latest で読む設定の記録は作らない)。"
	notifyLatestBudget  = `予算を記録で持つなら 50000 の代わりに {"agg":"latest","entity":"settings","field":"budget"}`
	notifySettingBudget = `予算を設定で持つなら 50000 の代わりに {"setting":"budget"}`
)

func init() {
	systemPrompt = strings.Replace(systemPrompt, settingsLatestLine, settingsSingleLine, 1)
	// notifyWhenRules itself changes too, so restrict.go's noWhen still finds it
	notify := strings.Replace(notifyWhenRules, notifyLatestBudget, notifySettingBudget, 1)
	systemPrompt = strings.Replace(systemPrompt, notifyWhenRules, notify, 1)
	notifyWhenRules = notify
	systemPrompt += "\n\n" + singleRules
	// right after the hand-written capabilities (the create prompt must keep
	// ending with the capability index); keeps chatCapabilities intact for
	// the other rules files' Replace.
	with := chatCapabilities + "\n\n" + singleChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, with, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, with, 1)
	// no dslTerm for "single" / "setting": ordinary English words in chat answers
}

// Older builds (restrict.go noSingle): the recipes that use a setting.
const (
	budgetSettingRecipe = `single の entity settings{budget:currency defaultValue 30000}(#267)を作り <予算> = {"setting":"budget"}(設定画面で編集するだけ・記録を追加させない)`
	budgetLatestRecipe  = `budget{amount:currency,date:date} entity を作り <予算> = {"agg":"latest","entity":"budget","field":"amount","by":"date"}(最後に設定した額。max だと下げた予算が反映されない)`
)

// restrictSingle drops Entity.single and the setting expression from the
// schema defs for builds without FeatureSingleEntity.
func restrictSingle(defs map[string]any) {
	if entity, _ := defs["Entity"].(map[string]any); entity != nil {
		props, _ := entity["properties"].(map[string]any)
		delete(props, "single")
	}
	delete(defs, "ExprSetting")
	expr, _ := defs["Expr"].(map[string]any)
	if expr == nil {
		return
	}
	if alts, ok := expr["anyOf"].([]any); ok {
		kept := make([]any, 0, len(alts))
		for _, v := range alts {
			if m, _ := v.(map[string]any); m != nil && m["$ref"] == "#/$defs/ExprSetting" {
				continue
			}
			kept = append(kept, v)
		}
		expr["anyOf"] = kept
	}
	if d, ok := expr["description"].(string); ok {
		expr["description"] = strings.Replace(d, "・{setting}(single の entity の設定値)", "", 1)
	}
	if rs, ok := expr["x-recipes"].([]any); ok {
		kept := make([]any, 0, len(rs))
		for _, v := range rs {
			s, _ := v.(string)
			s = strings.Replace(s, budgetSettingRecipe, budgetLatestRecipe, 1)
			if strings.Contains(s, `{"setting"`) {
				continue
			}
			kept = append(kept, s)
		}
		expr["x-recipes"] = kept
	}
}

// singleEntities lists entities with single in a DSL document (#267).
func singleEntities(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var app dsl.App
	if err := json.Unmarshal(raw, &app); err != nil {
		return nil
	}
	return app.SingleEntities()
}

// unsingle turns the single entities of a decoded DSL back into ordinary
// ones for builds without FeatureSingleEntity: "single" is dropped and every
// {"setting": f} becomes {"agg":"latest","entity":<its entity>,"field":f}
// (the newest record, which is what those builds wrote before). It returns
// how many entities changed.
func unsingle(root map[string]any) int {
	owner := map[string]string{} // field id -> single entity id
	n := 0
	ents, _ := root["entities"].([]any)
	for _, e := range ents {
		em, _ := e.(map[string]any)
		if em == nil || em["single"] != true {
			if em != nil {
				delete(em, "single") // false is the default
			}
			continue
		}
		delete(em, "single")
		n++
		id, _ := em["id"].(string)
		fs, _ := em["fields"].([]any)
		for _, f := range fs {
			if fm, _ := f.(map[string]any); fm != nil {
				if fid, _ := fm["id"].(string); fid != "" {
					if _, dup := owner[fid]; !dup {
						owner[fid] = id
					}
				}
			}
		}
	}
	if n == 0 {
		return 0
	}
	var fix func(v any) any
	fix = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			if f, ok := x["setting"].(string); ok && len(x) == 1 {
				if e := owner[f]; e != "" {
					return map[string]any{"agg": "latest", "entity": e, "field": f}
				}
			}
			for k, c := range x {
				x[k] = fix(c)
			}
		case []any:
			for i, c := range x {
				x[i] = fix(c)
			}
		}
		return v
	}
	fix(root)
	return n
}
