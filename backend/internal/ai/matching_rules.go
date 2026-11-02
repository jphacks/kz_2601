package ai

import (
	"regexp"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Matching apps (#507). Two parts:
//   - matchingRules / matchingChatRule: how to build a peer-finding app with
//     Entity.match, list.item.interest and matchList. Only for builds that send supportedFeatures "matching"
//     (restrict.go drops them otherwise, like retentionRules).
//   - datingRules / datingChatRule: romance and dating are never built, for
//     every build. Introducing strangers for romance falls under Japan's
//     出会い系サイト規制法 (registration, 18+ verification) and App Store's
//     dating treatment; docs/backend/features/matching.md.

// FeatureMatching is the supportedFeatures value of an iOS build that renders
// matching (interest / approve / matches UI, #507 phase 2).
const FeatureMatching = "matching"

var matchingRules = strings.TrimSpace(`
## 仲間探し(相互承認でつながる)は entity.match(#507)
- 「同じ趣味の人を探して、お互いに OK したら連絡できるアプリ」「勉強仲間を探したい」「イベントの参加者同士でつながりたい」: 次の 2 つの entity を作る。
  - プロフィール: "match":"profile"、scope は public(「同じ大学の人だけ」「サークルの中で」のように決まった人だけなら shared)。field は趣味・やりたいこと(string か enum)・地域(enum。都道府県や「オンライン」までの大まかな地域)・ひとこと(text)。名前の入力欄は作らず一覧に $author を出す。**写真・位置(location)・住所・市区町村・学校名・年齢・生年月日・性別・連絡先は入れない**(知らない人が見るため)。
  - 連絡先: "match":"matched"、scope はプロフィールと同じ。field は連絡方法(string。SNS の ID など)とひとこと。書いた本人と、お互いに承認してつながった相手だけが読める。
- 画面(タブ 3 つが定石):
  - 「さがす」: プロフィールの list に **item.interest: true**(title に趣味、subtitle に地域、trailing に $author)。他の人の行に「つながりたい」ボタンが出る(自分の行には出ない・送ると「送りました」)。自分のプロフィールを書くフォームへの追加ボタンも置く。
  - 「つながり」: **matchList**{entity: プロフィールの entity, show: "incoming"}(届いた人。行で承認・見送る)と matchList{show: "matches"}(つながった人。行のメニューで解除・通報・ブロック)を縦に並べる。送った人の matchList{show: "outgoing"}(取り下げ)は任意。item はプロフィールの list と同じ。
  - 「連絡先」: 連絡先の list(つながった人の分と自分の分だけ出る)とフォーム。
- 「つながりたい」を送る・承認する・見送る・取り下げる・解除する・通報・ブロック・18 歳以上の確認はアプリ本体が持つ(item.interest と matchList の中で完結)。**いいね用の entity・承認の boolean・相手を選ぶ ref・承認用のボタンや画面は作らない**。edit は書かない。
- summary に「お互いに「つながりたい」を送った人とだけ、連絡先が見られます。」を入れる。
`)

// matchingChatRule: the capability for chat answers (できる?).
var matchingChatRule = strings.TrimSpace(`
## 仲間探し(#507)
- 趣味・勉強・イベントの仲間をプロフィールから探し、**お互いに承認した人とだけ連絡先が見られる**アプリは**できる**(プロフィールの一覧の「つながりたい」ボタン・届いた人とつながった人の一覧で承認・見送る・解除・通報・ブロックはアプリ本体が持つ。初めて送る時に 18 歳以上かを確かめる)。地域は都道府県まで・写真や位置は載せない。
`)

// noMatchingChatRule replaces matchingChatRule for builds without the UI.
const noMatchingChatRule = "## 仲間探し(#507)\n- このアプリの版では、お互いに承認してつながる仕組みはまだ使えない。頼まれたら「この版ではお互いに承認してつながる仕組みは使えません。最新版に更新すると使えます」と答え、みんなで見るプロフィールの掲示板(連絡先・写真・位置は載せない)を代わりの案にする。"

var datingRules = strings.TrimSpace(`
## 恋愛・出会い目的は作らない(#507)
- 恋人・結婚相手・デートの相手・異性との出会いを探すアプリ(恋愛マッチング・婚活・出会い系・お見合い)は**作らない**。プロフィールの一覧・つながりたい・entity.match・連絡先の交換のように知らない人同士を引き合わせる部分は入れない。依頼に恋愛・出会いと関係ない部分(予定・日記などの自分の記録)があればそこだけ作り、全部が恋愛・出会いなら自分だけの「やりたいことメモ」(private の entity 1 つ)の小さなアプリにする。どちらでも app.description の末尾(編集は summary)に「` + datingNoteJA + `」と書く(en: "` + datingNoteEN + `")。
- 恋人・夫婦・家族の間で使う記録(デートの記録・記念日・二人の家計簿)は出会いではないので普通に作る(断らない)。
- 中学生・高校生の仲間探しのように 18 歳未満の人を知らない人と引き合わせる依頼は entity.match を使わず、知っている人だけの shared(シェアリンクで招待)の記録にする。
`)

// datingChatRule: chat declines before anything is generated.
var datingChatRule = strings.TrimSpace(`
## 恋愛・出会い目的(#507)
- 恋人探し・婚活・出会い系・デートの相手探しのアプリは**作れない**(出会い系サイト規制法の届出や年齢確認が必要になるため)。kind は ready にせず「` + datingNoteJA + `」と答え、suggestions に「趣味の仲間探し」「勉強仲間を探す」を入れる。恋人・家族の間で使う記録(デートの記録・記念日)は作れる。
`)

// The decline sentence, shared by the prompts and the server's fallback note.
const (
	datingNoteJA = "恋愛や出会いを目的に知らない人同士を引き合わせることはできません(出会い系サイト規制法の届出や年齢確認が必要になるため)。趣味・勉強・イベントの仲間探しなら作れます。"
	datingNoteEN = "The app can't introduce strangers for dating or romance (Japanese law would require registration and age verification). It can help people find peers for hobbies, study or events."
)

var (
	// datingAskRe: looking for a romantic partner. A couple's own records
	// (デートの記録・記念日) and 恋愛小説 do not match.
	datingAskRe = regexp.MustCompile(`恋人(を|が)?(探|ほし|欲し|募集|候補|づくり|作り|が見つか)|恋愛(目的|相手|対象|マッチング|の相手|できる相手)|婚活|出会い系|出会い(を求|の場|目的|アプリ|を探|がほし|が欲し)|異性(と|を|の)(出会|知り合|探|紹介|マッチ)|デート(相手|の相手|できる相手)|彼氏(を|が)?(探|ほし|欲し|募集)|彼女(を|が)?(探|ほし|欲し|募集)|結婚相手(を|が)?(探|募集|紹介)|お見合い|合コン(相手|の相手)|(?i:\bdating\b|\bdate partners?\b|\b(find|meet|looking for) (a |my )?(boyfriend|girlfriend|lover|soulmate|romantic partner|spouse)\b|\bromantic (match|matching|partners?)\b|\bhook ?ups?\b)`)
	// datingNotRe: the request rules romance out (「恋愛目的じゃない趣味仲間」).
	datingNotRe = regexp.MustCompile(`(恋愛|出会い|デート|婚活)(目的)?(じゃない|ではない|でない|じゃなく|ではなく|抜き|以外|禁止|NG|なし)|(?i:not (for )?(dating|romance)|no (dating|romance))`)
)

func datingAsked(t string) bool { return datingAskRe.MatchString(t) && !datingNotRe.MatchString(t) }

// datingOmission backs the rule up: the decline sentence is always said.
var datingOmission = omissionRule{
	id:      "dating",
	asked:   datingAsked,
	omitted: func(string, *dsl.App) bool { return true },
	topic:   regexp.MustCompile(`恋愛|出会い|(?i:dating|romance)`),
	problem: "依頼は恋愛・出会い目的のマッチングだが、作れない(出会い系サイト規制法・App Store)。entity.match・プロフィールの一覧・連絡先の交換は作らず、app.description の末尾に「" + datingNoteJA + "」と書く",
	noteJA:  func(string) string { return datingNoteJA },
	noteEN:  func(string) string { return datingNoteEN },
}

func init() {
	systemPrompt += "\n\n" + datingRules + "\n\n" + matchingRules
	// right after the hand-written capabilities, like retentionChatRule
	withMatching := chatCapabilities + "\n\n" + datingChatRule + "\n\n" + matchingChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, withMatching, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, withMatching, 1)
	omissionRules = append(omissionRules, datingOmission)
	for _, t := range []dslTerm{
		{"match", "仲間探し", ""}, // an ordinary English word: ja text only
		{"matchList", "つながりの一覧", "connections list"},
	} {
		dslTerms = append(dslTerms, t)
		termIndex[strings.ToLower(t.term)] = t
	}
}
