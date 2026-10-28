package ai

import (
	"regexp"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Silent omissions (#257): parts of a request the DSL cannot do (or the model
// dropped) and the spoken summary never mentioned. Each rule pairs a request
// pattern with a check on the generated app; an omission counts only when
// the model also wrote no honest sentence about it ("○○は今はできません。" /
// "○○は△△で代替しました。"). Create turns re-prompt once with the problem
// (with AI_LINT_RETRY, like dsl.Lint); whatever is still omitted at the end
// gets a fixed sentence appended to the summary, so nothing is dropped quietly.

type omissionRule struct {
	id string
	// asked: the request contains this part.
	asked func(t string) bool
	// omitted: the app does not really do it.
	omitted func(t string, a *dsl.App) bool
	// topic: words an honest sentence about this part contains.
	topic *regexp.Regexp
	// needsLogic: decided on computed fields, which the UI pass of the logic
	// pipeline leaves out (skip it there).
	needsLogic bool
	problem    string
	noteJA     func(t string) string
	noteEN     func(t string) string
}

var (
	notifyAskRe  = regexp.MustCompile(`通知|知らせ|教えて|リマインド|アラート|忘れないよう|(?i:notif|remind|alert|let me know|tell me when)`)
	notifyOffRe  = regexp.MustCompile(`(通知|リマインド|アラート)(を|は|も)?(消|止|やめ|オフ|切|いら|要ら|なくし|外)|(?i:(turn off|remove|stop|disable|delete|no more) (the |all )?(notif|remind|alert))`)
	notifyCondRe = regexp.MustCompile(`たら|れば|下回|切れ|超え|(?i:\bwhen\b|\bif\b|below|runs? (low|out))`)

	othersRoleRe = regexp.MustCompile(`店長|管理者|先生|リーダー|コーチ|家族|他の人|ほかの人|他の(メンバー|家族|スタッフ)|誰でも|(?i:manager|admin|teacher|family|anyone|others)`)
	othersVerbRe = regexp.MustCompile(`確定|承認|チェック|完了|終わったら|済み|解決|採用|(?i:confirm|approve|check|done|complete|resolve)`)
	ownOnlyRe    = regexp.MustCompile(`各自|自分の|(?i:\bown\b)`)

	quizRe  = regexp.MustCompile(`クイズ|テスト|問題|(?i:quiz|\btest\b)`)
	scoreRe = regexp.MustCompile(`採点|点数|正解|正誤|スコア|競|(?i:score|grade|correct|compete)`)

	favoriteRe = regexp.MustCompile(`お気に入り|気に入った|ブックマーク|(?i:favou?rite|bookmark)`)

	honestRe = regexp.MustCompile(`代替|できません|できない|出来ません|省きました|入れていません|押せません|(?i:replaced|can't|cannot|can not|not possible|isn't possible|isn't supported|not supported|left out)`)
)

var omissionRules = []omissionRule{
	{
		id:    "notify",
		asked: func(t string) bool { return notifyAskRe.MatchString(t) && !notifyOffRe.MatchString(t) },
		omitted: func(_ string, a *dsl.App) bool {
			return len(a.App.Notifications) == 0 && !hasAction(a, dsl.ActionScheduleNotif)
		},
		topic:   regexp.MustCompile(`通知|知らせ|教え|リマインド|アラート|(?i:notif|remind|alert|let you know)`),
		problem: `依頼に「知らせて / 通知 / 教えて」があるのに app.notifications も scheduleNotification も無い。決まった時刻・レコードの時刻の通知で満たせるなら足す。条件で知らせる通知(在庫が減ったら等)はできないので、近いもの(不足の一覧など)で代替し、app.description の末尾に「○○の通知は今はできないため、△△で代替しました。」と書く`,
		noteJA: func(t string) string {
			if notifyCondRe.MatchString(t) {
				return "条件で知らせる通知は今はできないため、通知は入れていません。"
			}
			return "通知は入れていません。「毎朝8時に通知して」のように時刻を言えば足せます。"
		},
		noteEN: func(t string) string {
			if notifyCondRe.MatchString(t) {
				return "Alerts triggered by a condition aren't possible yet, so no notification was added."
			}
			return `No notification was added. Say "remind me every morning at 8" to add one.`
		},
	},
	{
		id: "shared_update",
		asked: func(t string) bool {
			return othersRoleRe.MatchString(t) && othersVerbRe.MatchString(t) && !ownOnlyRe.MatchString(t)
		},
		omitted: func(_ string, a *dsl.App) bool { return sharedCheckable(a) },
		topic:   regexp.MustCompile(`作った人|作成者|投稿した人|本人|確定|承認|チェック|完了|(?i:creator|created|author|who added|confirm|approve|check)`),
		problem: `みんなで使う記録(scope public / shared)は作った人だけが変更・削除できる。依頼は他の人(店長・家族など)がチェック・確定する形だが、その boolean / toggle は作った人しか押せない。確定を別の記録にする(店長が「確定」の記録を追加する)か、app.description の末尾に「みんなで使う記録は作った人だけが変更できるため、○○は作った人しか押せません。」と書く`,
		noteJA: func(string) string {
			return "みんなで使う記録は作った人だけが変更できるため、ほかの人はチェックや確定ができません。"
		},
		noteEN: func(string) string {
			return "Shared records can only be changed by the person who created them, so others can't check or confirm them."
		},
	},
	{
		id:         "quiz_score",
		asked:      func(t string) bool { return quizRe.MatchString(t) && scoreRe.MatchString(t) },
		omitted:    func(_ string, a *dsl.App) bool { return !hasComputed(a) },
		needsLogic: true,
		topic:      regexp.MustCompile(`採点|点数|正解|正誤|スコア|(?i:score|grade|correct)`),
		problem:    `依頼に採点・点数があるのに、正解と比べる計算が無い(点数を手で入力する形)。クイズの自動採点(#262)の形で正誤を計算する。それでも出せないなら、app.description の末尾に「自動の採点を組めなかったため、点数は自分で入力する形で代替しました。」と書く`,
		noteJA: func(string) string {
			return "自動の採点を組めなかったため、点数は自分で入力する形です。"
		},
		noteEN: func(string) string {
			return "Automatic scoring couldn't be built, so scores are entered by hand."
		},
	},
	{
		id:      "save_copy",
		asked:   favoriteRe.MatchString,
		omitted: func(_ string, a *dsl.App) bool { return savesSharedByCopy(a) },
		topic:   regexp.MustCompile(`保存|お気に入り|気に入|コピー|ブックマーク|(?i:save|favou?rite|bookmark|copy)`),
		problem: `依頼は他の人の投稿(みんなで使う記録)をお気に入り・保存する形だが、その記録を指す自分用の記録(type ref + refEntity)が無い。自分用の entity(scope 省略)に ref で選ぶ項目を作るか、できないなら app.description の末尾に「ほかの人の投稿をそのまま保存する機能は今はできないため、○○で代替しました。」と書く。全項目を打ち直すフォームで黙って代用しない`,
		noteJA: func(string) string {
			return "ほかの人の投稿をそのまま自分用に保存する機能は今はできないため、内容は自分で入力する形です。"
		},
		noteEN: func(string) string {
			return "Saving someone else's post as-is isn't possible yet, so you enter the details yourself."
		},
	},
}

// omissions returns the ids of the rules the app omits silently. honest is
// the text the model wrote for the user (create: app.description, edit: the
// patch summary). logic=false skips rules decided on computed fields.
func omissions(transcript string, a *dsl.App, honest string, logic bool) []omissionRule {
	var out []omissionRule
	for _, r := range omissionRules {
		if r.needsLogic && !logic {
			continue
		}
		if !r.asked(transcript) || !r.omitted(transcript, a) || mentionsHonestly(honest, r.topic) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// omissionProblems renders the rules as lint problems for one re-prompt.
func omissionProblems(rs []omissionRule) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = "omission " + r.id + ": " + r.problem
	}
	return out
}

// omissionNote is the fixed sentence(s) appended to the summary.
func omissionNote(rs []omissionRule, transcript string, loc Locale) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		if loc == LocaleEN {
			parts = append(parts, r.noteEN(transcript))
		} else {
			parts = append(parts, r.noteJA(transcript))
		}
	}
	if loc == LocaleEN {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts, "")
}

func omissionIDs(rs []omissionRule) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.id
	}
	return out
}

// mentionsHonestly: some sentence of text is an honest "cannot / replaced"
// line about the topic.
func mentionsHonestly(text string, topic *regexp.Regexp) bool {
	for _, s := range splitSentences(text) {
		if honestRe.MatchString(s) && topic.MatchString(s) {
			return true
		}
	}
	return false
}

func splitSentences(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return r == '。' || r == '.' || r == '!' || r == '！' || r == '\n' })
}

// honestNote extracts the sentences of description that tell the user what
// was replaced or is not possible (read out in the create summary).
func honestNote(description string, sep string) []string {
	var out []string
	for _, sentence := range strings.SplitAfter(description, sep) {
		if s := strings.TrimSpace(sentence); s != "" && honestRe.MatchString(s) {
			out = append(out, s)
		}
	}
	return out
}

func hasAction(a *dsl.App, t dsl.ActionType) bool {
	found := false
	visitComponents(a, func(c *dsl.Component) {
		for _, act := range []*dsl.Action{c.Action, c.OnItemTap} {
			if act != nil && act.Type == t {
				found = true
			}
		}
	})
	for _, s := range a.Screens {
		for _, tb := range s.Toolbar {
			if tb.Action.Type == t {
				found = true
			}
		}
	}
	return found
}

func hasComputed(a *dsl.App) bool {
	for _, e := range a.Entities {
		for _, f := range e.Fields {
			if f.Computed != nil {
				return true
			}
		}
	}
	return false
}

// sharedCheckable: a shared/public entity has a boolean field or a list
// toggle, i.e. a check that only the record's author can actually press.
func sharedCheckable(a *dsl.App) bool {
	shared := map[string]bool{}
	for _, e := range a.Entities {
		if !e.IsShared() {
			continue
		}
		shared[e.ID] = true
		for _, f := range e.Fields {
			if f.Type == dsl.FieldBoolean && f.Computed == nil {
				return true
			}
		}
	}
	found := false
	visitComponents(a, func(c *dsl.Component) {
		if c.Item != nil && c.Item.Toggle != "" && shared[c.Entity] {
			found = true
		}
	})
	return found
}

// savesSharedByCopy: there are shared records but no private entity points at
// them, so "save it" can only mean typing it in again.
func savesSharedByCopy(a *dsl.App) bool {
	shared := map[string]bool{}
	for _, e := range a.Entities {
		if e.IsShared() {
			shared[e.ID] = true
		}
	}
	if len(shared) == 0 {
		return false
	}
	for _, e := range a.Entities {
		if e.IsShared() {
			continue
		}
		for _, f := range e.Fields {
			if (f.Type == dsl.FieldRef || f.Type == dsl.FieldRefs) && shared[f.RefEntity] {
				return false
			}
		}
	}
	return true
}

// visitComponents calls fn for every component, container children included.
func visitComponents(a *dsl.App, fn func(*dsl.Component)) {
	var walk func(cs []dsl.Component)
	walk = func(cs []dsl.Component) {
		for i := range cs {
			fn(&cs[i])
			walk(cs[i].Children)
		}
	}
	for i := range a.Screens {
		walk(a.Screens[i].Components)
	}
}
