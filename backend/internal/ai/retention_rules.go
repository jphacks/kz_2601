package ai

import "strings"

// retentionRules (#482): "1週間たった記録は消して" is a per-entity retention
// period (entity.retentionDays) that iOS enforces, not a bulk delete (which
// chatLimits says the app cannot do). Appended like musicRules so prompt.go /
// chat_first.go stay untouched.
var retentionRules = strings.TrimSpace(`
## 記録の自動削除は entity.retentionDays(#482)
- 「1週間たった記録は消して」「30日で消えるようにして」「古い記録は自動で消したい」「3か月より前のは要らない」「1日で消える日記」: 記録の entity に **"retentionDays": 日数** を書く(1日=1・3日=3・1週間=7・2週間=14・1か月=30・3か月=90・半年=180・1年=365)。作ってから日数を過ぎた記録は、端末が開いた時・前面に戻った時・保存の後に自動で消す。**「一括削除はできません」「消せません」と断らない**(一括削除・選んで消すのとは別の機能)。消すためのボタン・filter・computed・別 entity・通知は作らない。
- 書く entity: 日々たまる記録(支出・日記・投稿・体温・ログ・メッセージなど)。記録の entity が複数なら全部、「メモだけ」のように名指しされたらその entity だけ。カテゴリ・メンバー・お店のように ref で選ばれる一覧(マスター)と設定には書かない(消えると参照が切れる)。
- 期間は記録を作った日時($createdAt)から数える(日付の項目の値ではない)。「今日の 0 時に消す」のような時刻指定はできないので、日数で近いもの(1)にする。
- 共有(shared / public)の entity では、各自が自分で作った記録だけが消える(ほかの人の記録は消さない)。共有の entity に書いた時は summary にそう添える。
- 編集ターン: 「1週間で消して」は add /entities/<i>/retentionDays、期間の変更(「2週間にして」)は replace、「自動で消すのをやめて」「ずっと残して」「自動削除を解除」は remove /entities/<i>/retentionDays。画面・fields は変えない。
- 消えた記録は戻せない。summary は「30日たった記録は自動で消えます(消えた記録は戻せません)」のように期間を言う(設定した時、端末が確認を出す)。
`)

// retentionChatRule: the same capability for chat answers (できる?).
var retentionChatRule = strings.TrimSpace(`
## 記録の自動削除(#482)
- 「1週間たった記録を自動で消す」「30日で消えるようにする」は**できる**(記録の保存期間: 作ってから日数を過ぎた記録を自動で消す。設定する時に確認が出る。消えた記録は戻せない。「自動で消すのをやめて」で外せる)。一括削除の制限とは別なので「できません」と言わない。共有の記録は各自が自分で作った分だけ消える。
`)

// noRetentionChatRule replaces retentionChatRule for builds that cannot purge
// (iOS before #482 sends allowedComponents but not supportedFeatures).
const noRetentionChatRule = "## 記録の自動削除(#482)\n- このアプリの版では、期間を過ぎた記録を自動で消すことはできない。頼まれたら「この版では記録の自動削除はできません。最新版に更新すると使えます」と答える。"

func init() {
	systemPrompt += "\n\n" + retentionRules
	// right after the hand-written capabilities, like sensorChatRule (the
	// create prompt must keep ending with the capability index)
	withRetention := chatCapabilities + "\n\n" + retentionChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, withRetention, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, withRetention, 1)
	t := dslTerm{"retentionDays", "記録の保存期間", "auto-delete period"}
	dslTerms = append(dslTerms, t)
	termIndex[strings.ToLower(t.term)] = t
}
