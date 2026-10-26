package ai

import (
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Share intent (#259 / #215): one word table drives the create prompt, the
// edit prompt, the chat prompt and the server-side check, so the same request
// gets the same entity.scope whether it creates or edits an app.

type shareIntent int

const (
	shareNone     shareIntent = iota
	shareEveryone             // other people use the same records → public
	shareLimited              // "family only": not in the DSL → public + said aloud
	sharePrivate              // back to "only me"
)

var (
	shareEveryoneWords = []string{
		"みんなで", "みんなが見", "みんなが入力", "みんなと共有", "友達のスマホ", "家族のスマホ", "友達の端末", "家族の端末",
		"友達も", "家族も", "ほかの人も", "他の人も", "ほかの人と", "他の人と", "一緒に使", "一緒に入力", "複数人で使",
		"公開して", "公開に", "共有して", "共有でき", "で共有", "と共有",
		"share with", "with my friends", "with my family", "my friends' phones", "everyone can", "other people",
	}
	shareLimitedWords = []string{
		"家族だけ", "家族のみ", "家族限定", "身内だけ", "招待した人", "招待制", "メンバーだけ", "仲間だけ",
		"自分たちだけ", "二人だけ", "2人だけ", "夫婦だけ", "参加した人だけ", "限定公開", "限定で共有",
		"only my family", "family only", "just my family", "only people i invite", "invite only", "invite-only",
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
var shareRules = "## 共有の範囲(作成と編集で同じ規則・#259 #215)\n" +
	"- 他の人と同じ記録を使う依頼: " + quoteWords(shareEveryoneWords) + "など(en: share with / everyone / other people)。**作成でも編集でも同じく**、ほかの人が見る・入力する記録の entity を scope:\"public\" にする(どれか言われていなければ、ほかの人も入力する記録の entity 全部・割り勘ならメンバーと支出)。編集で「できない」と答えない・test だけの patch にしない。\n" +
	"- 編集ターンのレシピ: scope の無い entity は {\"op\":\"add\",\"path\":\"/entities/<i>/scope\",\"value\":\"public\"}、scope があれば replace。public の entity に author / authorName の field id があれば別の id に変える(backend が付ける)。\n" +
	"- 限られた人だけの共有: " + quoteWords(shareLimitedWords) + "など。招待制・メンバー限定は今の DSL に無い。上と同じく public にし、新規作成は app.description の末尾に「限られた人だけで共有することは今はできないため、アプリを開いた人なら誰でも見られる公開にしました。」と書く(黙って公開にしない)。\n" +
	"- 「自分だけ」「非公開」「共有をやめて」は scope を remove(private に戻す)。「LINE で送る」「共有ボタン」は scope ではなく shareButton。\n" +
	"- 公開範囲(誰でも見られること)の一文はサーバーが summary に足すので、編集の summary は何を変えたかだけでよい。"

// shareChatRule is appended to the chat prompts (chatLimitsText).
const shareChatRule = "- 共有の範囲(#259 #215): 「友達のスマホからも」「みんなで」のように他の人と同じ記録を使うことは**できる**(アプリを開いた人なら誰でも見られる「みんなに公開」になる・今あるアプリにも後から足せる)。「家族だけ」「招待した人だけ」に限る共有は**できない**。頼まれたら ready にする前に question で「家族だけには絞れず、アプリを開いた人なら誰でも見られる形になります。それでよいですか?」と確認し、答えを transcript に入れる。"

// newlyShared returns the shared/public entities of after that were not
// shared in before (before nil = create).
func newlyShared(before, after *dsl.App) []*dsl.Entity {
	was := map[string]bool{}
	if before != nil {
		for i := range before.Entities {
			if before.Entities[i].IsShared() {
				was[before.Entities[i].ID] = true
			}
		}
	}
	var out []*dsl.Entity
	for i := range after.Entities {
		e := &after.Entities[i]
		if e.IsShared() && !was[e.ID] {
			out = append(out, e)
		}
	}
	return out
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
// records but the patch left every entity private (#259: "できないため
// そのままにしました").
func shareEditProblem(transcript string, before, after []byte) string {
	in := classifyShareIntent(transcript)
	if in != shareEveryone && in != shareLimited {
		return ""
	}
	b, err := dsl.Parse(before)
	if err != nil || anyShared(b) {
		return ""
	}
	a, err := dsl.Parse(after)
	if err != nil || anyShared(a) {
		return ""
	}
	ids := make([]string, 0, len(b.Entities))
	for i := range b.Entities {
		ids = append(ids, fmt.Sprintf("%d=%s", i, b.Entities[i].ID))
	}
	return lintPrefix + "依頼はほかの人と同じ記録を使うこと(共有)で、DSL でできる。新規作成と同じく、ほかの人が見る・入力する entity に {\"op\":\"add\",\"path\":\"/entities/<i>/scope\",\"value\":\"public\"} を足す(entities: " +
		strings.Join(ids, ", ") + ")。「できない」と答えない。"
}

// withShareNote appends the spoken disclosure for entities that became
// shared (D-05 wording); a limited-share request also says it could not be
// limited (#215). The note is skipped when the summary already says it.
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
	added := newlyShared(b, a)
	limited := classifyShareIntent(transcript) == shareLimited
	if len(added) == 0 {
		if limited && !anyShared(a) {
			return withPrivateNote(summary, loc)
		}
		return summary
	}
	if loc == LocaleEN {
		if l := strings.ToLower(summary); strings.Contains(l, "anyone") && strings.Contains(l, "see") {
			return summary
		}
		names := joinEN(entityNamesOf(added, true), "and")
		note := fmt.Sprintf("Anyone who opens this app can see %s.", names)
		if limited {
			note = fmt.Sprintf("Sharing with only certain people isn't available yet, so anyone who opens this app can see %s.", names)
		}
		return strings.TrimSpace(strings.TrimSpace(summary) + " " + note)
	}
	if strings.Contains(summary, "誰でも見られ") {
		return summary
	}
	names := joinJa(entityNamesOf(added, false))
	note := names + "は、このアプリを開いた人なら誰でも見られます(みんなに公開)。"
	if limited {
		note = "限られた人だけで共有することは今はできないため、" + names + "はアプリを開いた人なら誰でも見られる公開にしました。"
	}
	return strings.TrimSpace(summary) + note
}

// withPrivateNote: a limited-share request that left every record private
// says so too (#215: no silent result either way).
func withPrivateNote(summary string, loc Locale) string {
	if loc == LocaleEN {
		l := strings.ToLower(summary)
		if strings.Contains(l, "shar") && (strings.Contains(l, "can't") || strings.Contains(l, "not available") || strings.Contains(l, "isn't available")) {
			return summary
		}
		return strings.TrimSpace(strings.TrimSpace(summary) + " Sharing with only certain people isn't available yet, so the records stay private to you.")
	}
	if strings.Contains(summary, "共有") && strings.Contains(summary, "できな") {
		return summary
	}
	return strings.TrimSpace(summary) + "限られた人だけで共有することは今はできないため、記録は自分だけのままです。"
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
