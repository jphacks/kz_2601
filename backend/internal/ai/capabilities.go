package ai

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// chatCapabilityIndex is the "what can it do" list the chat answers from
// (#230). It is built from the canonical schema like catalog, so a part or op
// added to schema.json (with its description) is answerable as soon as it
// ships; the hand-written chatLimits only lists what the DSL truly cannot do.
// The create prompt carries this index (it has no catalog); the edit prompt
// carries the full catalog, which is built from the same schema.
var chatCapabilityIndex = mustCapabilityIndex(dsl.SchemaJSON())

func mustCapabilityIndex(raw []byte) string {
	out, err := BuildCapabilityIndex(raw)
	if err != nil {
		panic("ai: build capability index: " + err.Error())
	}
	return out
}

// capDescRunes caps each entry's description so the index stays a summary.
const capDescRunes = 64

// BuildCapabilityIndex renders one short line per container, leaf, field
// type, op and action from the schema's descriptions (first sentence only).
func BuildCapabilityIndex(schema []byte) (string, error) {
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil {
		return "", err
	}
	defs, _ := root["$defs"].(map[string]any)
	if defs == nil {
		return "", fmt.Errorf("schema has no $defs")
	}
	var b strings.Builder
	b.WriteString("## できることの索引(スキーマから自動生成。ここにある部品・計算・動作で作れることは「できます」と答える)\n")

	container, _ := defs["Container"].(map[string]any)
	var cs []string
	for _, t := range enumValues(container, "type") {
		cs = append(cs, t+"("+shortDesc(enumDesc(container, t))+")")
	}
	if len(cs) == 0 {
		return "", fmt.Errorf("schema has no $defs/Container type enum")
	}
	b.WriteString("- 器: " + strings.Join(cs, " / ") + "\n")

	leafNames := refNames(defs["Leaf"])
	if len(leafNames) == 0 {
		return "", fmt.Errorf("schema has no $defs/Leaf anyOf")
	}
	b.WriteString("- 部品:\n")
	for _, name := range leafNames {
		def, _ := defs[name].(map[string]any)
		t := constType(def)
		if t == "" {
			return "", fmt.Errorf("$defs/%s has no properties.type.const", name)
		}
		desc, _ := def["description"].(string)
		fmt.Fprintf(&b, "  - %s: %s\n", t, shortDesc(desc))
	}

	field, _ := defs["Field"].(map[string]any)
	if ft := enumValues(field, "type"); len(ft) > 0 {
		fmt.Fprintf(&b, "- 記録の項目(field)の型: %s", strings.Join(ft, " / "))
		if props, _ := field["properties"].(map[string]any); props != nil {
			if h, _ := props["health"].(map[string]any); h != nil {
				d, _ := h["description"].(string)
				fmt.Fprintf(&b, "。health: %s", shortDesc(d))
			}
		}
		b.WriteString("\n")
	}

	op, _ := defs["ExprOp"].(map[string]any)
	ops := enumValues(op, "op")
	if len(ops) == 0 {
		return "", fmt.Errorf("schema has no $defs/ExprOp.properties.op enum")
	}
	var plain, described []string
	for _, o := range ops {
		d := enumDescOf(op, "op", o)
		// Arithmetic / comparison ops are self-explanatory; spend tokens on
		// the ones users ask about (dates, streaks, ...).
		if plainOps[o] || d == "" {
			plain = append(plain, o)
			continue
		}
		described = append(described, o+" = "+shortDesc(opDesc(o, d)))
	}
	fmt.Fprintf(&b, "- 計算式(項目の計算・集計カード): agg(count / sum / avg / min / max / latest / earliest、期間の絞り込み)・if・op。op: %s\n", strings.Join(plain, ", "))
	for _, d := range described {
		b.WriteString("  - " + d + "\n")
	}

	action, _ := defs["Action"].(map[string]any)
	var as []string
	for _, t := range enumValues(action, "type") {
		as = append(as, t+"("+shortDesc(enumDesc(action, t))+")")
	}
	if len(as) == 0 {
		return "", fmt.Errorf("schema has no $defs/Action type enum")
	}
	b.WriteString("- ボタンの動作: " + strings.Join(as, " / ") + "\n")

	entity, _ := defs["Entity"].(map[string]any)
	var ss []string
	for _, v := range enumValues(entity, "scope") {
		ss = append(ss, v+"("+shortDesc(enumDescOf(entity, "scope", v))+")")
	}
	if len(ss) > 0 {
		b.WriteString("- 記録の共有(scope): " + strings.Join(ss, " / ") + "\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// plainOps are self-explanatory ops listed by name only.
var plainOps = map[string]bool{
	"add": true, "sub": true, "mul": true, "div": true, "mod": true, "neg": true, "abs": true,
	"sqrt": true, "pow": true, "round": true, "floor": true, "ceil": true, "min": true, "max": true,
	"clamp": true, "eq": true, "neq": true, "gt": true, "gte": true, "lt": true, "lte": true,
	"and": true, "or": true, "not": true, "concat": true, "len": true, "now": true, "today": true,
	"year": true, "month": true, "day": true, "weekday": true, "toNumber": true, "toString": true,
	"coalesce": true,
}

// shortDesc keeps the leading sentences of a description that fit in
// capDescRunes runes (the first one is cut with "…" when it alone is longer).
func shortDesc(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	var out string
	for s != "" {
		sent := s
		if i := sentenceEnd(s); i > 0 {
			sent, s = s[:i], strings.TrimSpace(s[i:])
		} else {
			s = ""
		}
		sent = strings.TrimRight(strings.TrimSpace(sent), "。.")
		switch {
		case out == "" && utf8.RuneCountInString(sent) > capDescRunes:
			return string([]rune(sent)[:capDescRunes]) + "…"
		case out == "":
			out = sent
		case utf8.RuneCountInString(out)+1+utf8.RuneCountInString(sent) <= capDescRunes:
			out += "。" + sent
		case capDescRunes-utf8.RuneCountInString(out)-1 >= 20:
			// Room for the start of the next sentence (its key words).
			return out + "。" + string([]rune(sent)[:capDescRunes-utf8.RuneCountInString(out)-1]) + "…"
		default:
			return out
		}
	}
	return out
}

// sentenceEnd is the byte index just past the first "。" or ". " in s, or -1.
func sentenceEnd(s string) int {
	best := -1
	if i := strings.Index(s, "。"); i >= 0 {
		best = i + len("。")
	}
	if i := strings.Index(s, ". "); i >= 0 && (best < 0 || i+1 < best) {
		best = i + 1
	}
	return best
}

// opDesc drops the "name(args) = " signature some op descriptions start with.
func opDesc(name, d string) string {
	if strings.HasPrefix(d, name+"(") {
		if i := strings.Index(d, ") = "); i > 0 {
			return d[i+len(") = "):]
		}
	}
	return d
}

// chatLimit is one thing the DSL cannot do, with the closest alternative.
// Keep it to real limits: TestChatLimits_NoConflictWithSchema fails when an
// entry names something the schema can build.
type chatLimit struct{ What, Instead string }

var chatLimits = []chatLimit{
	{"外部サービスの自動取得(天気・株価・ニュースの API)", "webView でそのサイトを表示"},
	{"ログイン・課金・決済", ""},
	{"サーバーからのプッシュ通知", "時刻指定のローカル通知"},
	{"ウィジェット・Apple Watch", ""},
	{"順位番号", "合計の降順の表"},
	{"一括削除などの一括操作", "スワイプ削除"},
	{"タイマーの自動切り替え", ""},
	{"ファイル書き出し", "共有ボタン"},
	{"車線の案内・渋滞の回避・電車やバスの乗り換え", "車・徒歩の道案内"}, // #411: turn-by-turn itself is routeGuide
	{"投稿者名の表示", "入力フォームに「使った人」の選択項目"},
	{"1 つの項目で選択肢を複数選ぶ入力", "選択肢ごとのオンオフ"},
	{"フォントのポイント数・好きなフォント・アニメーション・部品の重ね合わせ", "文字の 4 段の大きさと枠の大きさ"},
}

// chatLimitsText renders chatLimits as one prompt bullet.
func chatLimitsText() string {
	parts := make([]string, 0, len(chatLimits))
	for _, l := range chatLimits {
		if l.Instead == "" {
			parts = append(parts, l.What)
			continue
		}
		parts = append(parts, l.What+"(代わり → "+l.Instead+")")
	}
	return "- できないこと(聞かれたら正直に「できません」と言い、近い代わりを添える。**ここに無く、索引や部品カタログにあるものは「できます」**): " + strings.Join(parts, "、") + "。\n" + shareChatRule + "\n" + publicPIIChatRule
}
