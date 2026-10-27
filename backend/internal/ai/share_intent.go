package ai

import (
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Share intent (#259 / #215 / #254): one word table drives the create prompt,
// the edit prompt, the chat prompt and the server-side check, so the same
// request gets the same entity.scope whether it creates or edits an app.

type shareIntent int

const (
	shareNone     shareIntent = iota
	shareEveryone             // other people use the same records → public
	shareLimited              // family / circle / invited people only → shared (#388: link members)
	sharePrivate              // back to "only me"
)

var (
	shareEveryoneWords = []string{
		"みんなで", "みんなが見", "みんなが入力", "みんなと共有", "友達のスマホ", "友達の端末",
		"友達も", "ほかの人も", "他の人も", "ほかの人と", "他の人と", "一緒に使", "一緒に入力", "複数人で使",
		"公開して", "公開に", "共有して", "共有でき", "で共有", "と共有",
		"share with", "with my friends", "my friends' phones", "everyone can", "other people",
	}
	shareLimitedWords = []string{
		"家族だけ", "家族のみ", "家族限定", "身内だけ", "招待した人", "招待制", "メンバーだけ", "仲間だけ",
		"自分たちだけ", "二人だけ", "2人だけ", "夫婦だけ", "参加した人だけ", "限定公開", "限定で共有",
		"家族で共有", "家族と共有", "家族で使", "家族と使", "家族で見", "家族で記録", "家族でつけ", "家族みんな", "家族全員",
		"家族のスマホ", "家族の端末", "家族も見", "家族も入力", "家族も使", "夫婦で", "サークルのみんな", "サークルで", "サークルのメンバー",
		"部活のみんな", "部員", "チームで", "チームのみんな", "チームのメンバー", "メンバーが各自", "メンバーで共有",
		"only my family", "family only", "just my family", "with my family", "only people i invite", "invite only", "invite-only",
		"my team", "club members",
	}
	// An explicit "no sharing" (incl. the free-version instruction, paid.go)
	// wins over every other word.
	shareOffWords = []string{
		"無料版で作る", "共有しない", "共有はしない", "共有せず", "共有はせず", "共有は不要", "共有不要",
		"free version", "without sharing", "not shared",
	}
	sharePrivateWords = []string{
		"自分だけ", "非公開", "公開をやめ", "共有をやめ", "公開しない",
		"only me", "make it private", "stop sharing",
	}
	// Sending a copy (shareButton) is not a scope change.
	shareSendWords = []string{
		"LINE", "Slack", "Discord", "メールで", "共有ボタン", "シェアボタン", "送って", "送れ", "送り", "書き出",
		"line", "slack", "discord", "share button", "send",
	}
)

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// classifyShareIntent reads who the user wants to see the records.
func classifyShareIntent(transcript string) shareIntent {
	t := strings.ToLower(strings.TrimSpace(transcript))
	switch {
	case containsAny(t, shareOffWords):
		return sharePrivate
	case containsAny(t, shareLimitedWords):
		return shareLimited
	case containsAny(t, sharePrivateWords):
		return sharePrivate
	case containsAny(t, shareSendWords):
		return shareNone
	case containsAny(t, shareEveryoneWords):
		return shareEveryone
	}
	return shareNone
}

func quoteWords(ws []string) string {
	var ja []string
	for _, w := range ws {
		if !isASCII(w) {
			ja = append(ja, "「"+w+"」")
		}
	}
	return strings.Join(ja, "")
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// shareRules is the prompt section for both create and edit turns.
var shareRules = "## 共有の範囲(作成と編集で同じ規則・#259 #215 #254)\n" +
	"- 決まった人だけで使う記録: " + quoteWords(shareLimitedWords) + "など(en: only my family / my team / invite only)。家族・夫婦・サークル・部活・チームの記録は scope:\"shared\"(作った人と、シェアリンクで招待した人だけが見る・入力する)。**作成でも編集でも同じく** shared にし、public にしない(誰でも見られるようにしない)。上の entity.scope の行より優先する。\n" +
	"- 不特定の他の人と同じ記録を使う依頼: " + quoteWords(shareEveryoneWords) + "など(en: share with / everyone / other people)。ほかの人が見る・入力する記録の entity を scope:\"public\" にする(どれか言われていなければ、ほかの人も入力する記録の entity 全部・割り勘ならメンバーと支出)。掲示板・ランキング・みんなで投稿も public。\n" +
	"- 編集で共有を頼まれたら「できない」と答えない・test だけの patch にしない。レシピ: scope の無い entity は {\"op\":\"add\",\"path\":\"/entities/<i>/scope\",\"value\":\"shared\"}(不特定の人なら \"public\")、scope があれば replace。shared / public の entity に author / authorName の field id があれば別の id に変える(backend が付ける)。\n" +
	"- 「自分だけ」「非公開」「共有をやめて」は scope を remove(private に戻す)。「LINE で送る」「共有ボタン」は scope ではなく shareButton。\n" +
	"- 誰が見られるか(リンクで招待した人だけ / 誰でも)の一文はサーバーが summary に足すので、summary は何を作った・変えたかだけでよい。"

// shareChatRule is appended to the chat prompts (chatLimitsText).
const shareChatRule = "- 共有の範囲(#259 #215 #254): 「家族だけ」「家族で」「サークルのみんなで」「招待した人だけ」のように決まった人だけで同じ記録を使うことは**できる**(作った人と、共有シートのリンクで招待した人だけが見られる・今あるアプリにも後から足せる)。「友達のスマホからも」「みんなで」のように他の人と同じ記録を使うことも**できる**(アプリを開いた人なら誰でも見られる「みんなに公開」)。家族だけの共有を「できない」と答えず、公開になるか確かめる質問もしない。"

// newlyShared returns the shared/public entities of after whose scope
// differs from before (before nil = create), so public → shared counts too.
func newlyShared(before, after *dsl.App) []*dsl.Entity {
	was := map[string]dsl.EntityScope{}
	if before != nil {
		for i := range before.Entities {
			was[before.Entities[i].ID] = before.Entities[i].EffectiveScope()
		}
	}
	var out []*dsl.Entity
	for i := range after.Entities {
		e := &after.Entities[i]
		if e.IsShared() && was[e.ID] != e.EffectiveScope() {
			out = append(out, e)
		}
	}
	return out
}

func hasScope(a *dsl.App, s dsl.EntityScope) bool {
	for i := range a.Entities {
		if a.Entities[i].EffectiveScope() == s {
			return true
		}
	}
	return false
}

// wantScope is the scope a share request asks for ("" = no scope request).
func wantScope(in shareIntent) dsl.EntityScope {
	switch in {
	case shareEveryone:
		return dsl.ScopePublic
	case shareLimited:
		return dsl.ScopeShared
	}
	return ""
}

func anyShared(a *dsl.App) bool {
	for i := range a.Entities {
		if a.Entities[i].IsShared() {
			return true
		}
	}
	return false
}

// shareEditProblem returns a retry problem when an edit asked to share the
// records but the patch did not give them the asked scope (#259: "できない
// ためそのままにしました"; #254: "家族だけ" made public).
func shareEditProblem(transcript string, before, after []byte) string {
	want := wantScope(classifyShareIntent(transcript))
	if want == "" {
		return ""
	}
	b, err := dsl.Parse(before)
	if err != nil {
		return ""
	}
	a, err := dsl.Parse(after)
	if err != nil || hasScope(a, want) {
		return ""
	}
	if want == dsl.ScopePublic && anyShared(a) {
		return "" // already shared with someone: don't widen it
	}
	ids := make([]string, 0, len(b.Entities))
	for i := range b.Entities {
		ids = append(ids, fmt.Sprintf("%d=%s", i, b.Entities[i].ID))
	}
	return lintPrefix + shareProblemHead(want) + "ほかの人が見る・入力する entity に {\"op\":\"add\",\"path\":\"/entities/<i>/scope\",\"value\":\"" + string(want) +
		"\"} を足す(scope があれば replace。entities: " + strings.Join(ids, ", ") + ")。「できない」と答えない。"
}

// shareCreateProblem: a new app for "家族だけ" / "サークルで" with no shared
// entity is regenerated once (#254). Public requests keep the model's choice.
func shareCreateProblem(transcript string, raw []byte) string {
	if classifyShareIntent(transcript) != shareLimited {
		return ""
	}
	a, err := dsl.Parse(raw)
	if err != nil || hasScope(a, dsl.ScopeShared) {
		return ""
	}
	return lintPrefix + shareProblemHead(dsl.ScopeShared) + "ほかの人が見る・入力する entity を scope:\"shared\" にする(public にしない)。"
}

func shareProblemHead(want dsl.EntityScope) string {
	if want == dsl.ScopeShared {
		return "依頼は家族・サークルなど決まった人だけで同じ記録を使うこと(共有)で、scope:\"shared\"(作った人とリンクで招待した人だけ)でできる。"
	}
	return "依頼はほかの人と同じ記録を使うこと(共有)で、DSL でできる。新規作成と同じく scope:\"public\" にする。"
}

// withShareNote appends the spoken disclosure for entities whose scope
// became shared ("リンクで招待した人だけ", #254) or public (D-05 wording). A
// limited-share request that left every record private says so too (#215).
// Each note is skipped when the summary already says it.
func withShareNote(summary, transcript string, before, after []byte, loc Locale) string {
	a, err := dsl.Parse(after)
	if err != nil {
		return summary
	}
	var b *dsl.App
	if len(before) > 0 {
		if b, err = dsl.Parse(before); err != nil {
			return summary
		}
	}
	var shared, public []*dsl.Entity
	for _, e := range newlyShared(b, a) {
		if e.EffectiveScope() == dsl.ScopeShared {
			shared = append(shared, e)
		} else {
			public = append(public, e)
		}
	}
	if len(shared)+len(public) == 0 {
		if classifyShareIntent(transcript) == shareLimited && !anyShared(a) {
			return withPrivateNote(summary, loc)
		}
		return summary
	}
	out := strings.TrimSpace(summary)
	if loc == LocaleEN {
		l := strings.ToLower(out)
		if len(shared) > 0 && !strings.Contains(l, "invite") {
			out = strings.TrimSpace(out + " " + fmt.Sprintf("Only you and people you invite with the share link can see %s.", joinEN(entityNamesOf(shared, true), "and")))
		}
		if len(public) > 0 && (!strings.Contains(l, "anyone") || !strings.Contains(l, "see")) {
			out = strings.TrimSpace(out + " " + fmt.Sprintf("Anyone who opens this app can see %s.", joinEN(entityNamesOf(public, true), "and")))
		}
		return out
	}
	if len(shared) > 0 && !strings.Contains(summary, "招待した人だけ") {
		out += joinJa(entityNamesOf(shared, false)) + "は、" + sharedNoteJa
	}
	if len(public) > 0 && !strings.Contains(summary, "誰でも見られ") {
		out += joinJa(entityNamesOf(public, false)) + "は、このアプリを開いた人なら誰でも見られます(みんなに公開)。"
	}
	return out
}

// sharedNoteJa follows "○○は、" (#254 / #388: creator + people who joined by link).
const sharedNoteJa = "作った人とリンクで招待した人だけが見られます(共有シートからリンクを送って招待)。"

// withPrivateNote: a limited-share request that left every record private
// says so too (#215: no silent result either way).
func withPrivateNote(summary string, loc Locale) string {
	if loc == LocaleEN {
		l := strings.ToLower(summary)
		if strings.Contains(l, "shar") || strings.Contains(l, "private") {
			return summary
		}
		return strings.TrimSpace(strings.TrimSpace(summary) + " Sharing wasn't turned on, so the records stay private to you.")
	}
	if strings.Contains(summary, "共有") || strings.Contains(summary, "自分だけ") {
		return summary
	}
	return strings.TrimSpace(summary) + "共有の設定は変えていないため、記録は自分だけのままです。"
}

func entityNamesOf(es []*dsl.Entity, lower bool) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		n := e.Name
		if lower {
			n = strings.ToLower(n)
		}
		out = append(out, n)
	}
	return out
}
