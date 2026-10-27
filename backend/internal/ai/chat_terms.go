package ai

import (
	"regexp"
	"strings"
)

// Plain words in the chat (#258): the chat prompt carries the DSL catalog, so
// replies and chips leaked its English names ("public の記録で", "支出を public
// にして", "乱数の計算式(op)は無い", "折りたたみ(disclosure)で"). The prompt now
// forbids them; plainTerms is the server-side safety net on every text the
// user sees: reply text, chips, idea titles and details, and also the idea
// instruction / ready transcript (shown on the generating screen) and the brief card
// (the generator reads the plain wording just as well).

// dslTerm is one internal name and its user-facing wording per locale
// (en "" = leave the word alone: an ordinary English word there).
type dslTerm struct {
	term string
	ja   string
	en   string
}

// dslTerms: scope names, Expr / schema words and component type names that
// the catalog spells out. Longer names first so "datePicker" wins over a
// shorter overlap.
var dslTerms = []dslTerm{
	{"scope", "公開範囲", "sharing setting"},
	{"public", "みんなに公開", ""},
	{"private", "自分だけ", ""},
	{"shared", "参加した人だけ", ""},
	{"entity", "記録の種類", "record type"},
	{"entities", "記録の種類", "record types"},
	{"field", "項目", ""},
	{"fields", "項目", ""},
	{"computed", "自動計算", "calculated value"},
	{"op", "計算", "calculation"},
	{"expr", "計算式", "formula"},
	{"aggregate", "集計", ""},
	{"agg", "集計", "total"},
	{"ref", "ほかの記録を選ぶ項目", "link to another record"},
	{"refs", "ほかの記録を複数選ぶ項目", "links to other records"},
	{"refEntity", "選ぶ先の記録", "linked record type"},
	{"enum", "選択肢", "choice list"},
	{"boolean", "はい/いいえ", "yes/no"},
	{"disclosure", "折りたたみ", "collapsible section"},
	{"stat", "数字のカード", "number card"},
	{"progressBar", "進み具合のバー", "progress bar"},
	{"gridTable", "時間割の表", "timetable grid"},
	{"textField", "文字の入力欄", "text box"},
	{"numberField", "数字の入力欄", "number box"},
	{"datePicker", "日付の入力", "date picker"},
	{"timePicker", "時刻の入力", "time picker"},
	{"photoField", "写真の欄", "photo box"},
	{"videoField", "動画の欄", "video box"},
	{"locationPicker", "場所の検索", "place search"},
	{"locationTracker", "道のりの記録", "route tracker"},
	{"dashcam", "ドラレコ", ""},
	{"healthStat", "ヘルスケアの数字", "Health data card"},
	{"hstack", "横並び", "row"},
	{"vstack", "縦並び", "column"},
	{"settlement", "精算", "settle-up list"},
	{"streak", "連続日数", "streak"},
	{"doneInDays", "直近の日数", "days done"},
	{"swipeToDelete", "スワイプで削除", "swipe to delete"},
	{"saveRecord", "保存ボタン", "save button"},
	{"deleteRecord", "削除ボタン", "delete button"},
	{"scheduleNotification", "通知ボタン", "reminder button"},
	{"onItemTap", "タップした時の動き", "tap action"},
	{"navigate", "画面の移動", "go to screen"},
	{"DSL", "設計図", "app blueprint"},
	// ordinary English words: replaced in ja text only
	{"list", "一覧", ""},
	{"table", "表", ""},
	{"chart", "グラフ", ""},
	{"toggle", "チェック", ""},
	{"picker", "選ぶ入力", ""},
	{"section", "区切り", ""},
	{"notifications", "通知", ""},
	{"notification", "通知", ""},
	{"AppDSL", "設計図", "app blueprint"},
}

var (
	termIndex = func() map[string]dslTerm {
		m := make(map[string]dslTerm, len(dslTerms))
		for _, t := range dslTerms {
			m[strings.ToLower(t.term)] = t
		}
		return m
	}()
	// an ASCII word with optional quotes / emphasis and one space around it
	termWordRe = regexp.MustCompile(` ?[*_` + "`" + `"]*\b[A-Za-z][A-Za-z]*\b[*_` + "`" + `"]* ?`)
	// 「折りたたみ(disclosure)」: a parenthesised DSL name after the plain word
	termParenRe   = regexp.MustCompile(`\s*[(（]\s*[*_` + "`" + `"]*([A-Za-z][A-Za-z ,/・、]*?)[*_` + "`" + `"]*\s*[)）]`)
	scopePhraseRe = regexp.MustCompile(`(?i)\bscope\s*(?:を|は|が|:|=)?\s*[*_` + "`" + `"]*(public|private|shared)\b[*_` + "`" + `"]*`)
	// 「public にして」→「みんなに公開して」
	publicVerbRe = regexp.MustCompile(`(?i) ?[*_` + "`" + `"]*\bpublic\b[*_` + "`" + `"]*\s*に\s*(して|する|します|しました|すれば|すると|したら)`)
)

// plainTerms rewrites DSL names in text the user reads into plain words.
func plainTerms(s string, loc Locale) string {
	if s == "" {
		return s
	}
	en := loc == LocaleEN
	// drop "(op)" / "(disclosure)" glosses when every word inside is a DSL name
	s = termParenRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := termParenRe.FindStringSubmatch(m)[1]
		words := strings.FieldsFunc(inner, func(r rune) bool { return strings.ContainsRune(" ,/・、", r) })
		for _, w := range words {
			if _, ok := termIndex[strings.ToLower(w)]; !ok {
				return m
			}
		}
		if len(words) == 0 {
			return m
		}
		return ""
	})
	// "scope public" / "scope: \"public\"" -> just the scope name, worded below
	s = scopePhraseRe.ReplaceAllString(s, "$1")
	if !en {
		s = publicVerbRe.ReplaceAllString(s, "みんなに公開$1")
	}
	s = termWordRe.ReplaceAllStringFunc(s, func(m string) string {
		word := strings.Trim(m, " *_`\"")
		t, ok := termIndex[strings.ToLower(word)]
		if !ok || (en && !isTermSpelling(word, t.term)) {
			return m
		}
		if en {
			if t.en == "" {
				return m
			}
			// keep the spaces around the word
			return m[:len(m)-len(strings.TrimLeft(m, " "))] + t.en + m[len(strings.TrimRight(m, " ")):]
		}
		// 「public の記録」→「みんなに公開の記録」: Japanese needs no spaces around it
		return t.ja
	})
	return s
}

// isTermSpelling (en only): the word is written like the DSL name
// (lowercase or its exact camelCase), not a capitalised sentence word.
func isTermSpelling(word, term string) bool {
	return word == term || word == strings.ToLower(term)
}

// plainChatResult applies plainTerms to everything of res the user reads.
func plainChatResult(res *ChatResult) {
	res.Text = plainTerms(res.Text, res.Locale)
	for i := range res.Suggestions {
		res.Suggestions[i] = plainTerms(res.Suggestions[i], res.Locale)
	}
	for i := range res.Options {
		res.Options[i].Title = plainTerms(res.Options[i].Title, res.Locale)
		res.Options[i].Detail = plainTerms(res.Options[i].Detail, res.Locale)
		res.Options[i].Instruction = plainTerms(res.Options[i].Instruction, res.Locale)
	}
	res.Transcript = plainTerms(res.Transcript, res.Locale)
	if b := res.Brief; b != nil { // the 「この内容で作る」 card
		b.Name, b.Purpose = plainTerms(b.Name, res.Locale), plainTerms(b.Purpose, res.Locale)
		for i := range b.Entities {
			b.Entities[i] = plainTerms(b.Entities[i], res.Locale)
		}
		for i := range b.Features {
			b.Features[i] = plainTerms(b.Features[i], res.Locale)
		}
	}
}

// LeakedTerms lists the DSL names left in Japanese text s (tests and ai-eval).
func LeakedTerms(s string) []string {
	var out []string
	for _, m := range termWordRe.FindAllString(s, -1) {
		word := strings.Trim(m, " *_`\"")
		if _, ok := termIndex[strings.ToLower(word)]; ok {
			out = append(out, word)
		}
	}
	return out
}
