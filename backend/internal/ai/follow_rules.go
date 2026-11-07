package ai

import "strings"

// followRules (#270): "posts of the people I follow" used to be declined (the
// timeline showed everyone). Follows are now kept by appconfig per app; the
// DSL reads them with filter {"myFollows":true} and follow / unfollow
// buttons. Appended like marksRules so prompt.go / catalog.go stay untouched.
var followRules = strings.TrimSpace(`
## フォローとタイムライン(#270)
- 「フォローした人の投稿だけ」「フォロー中のタイムライン」「好きな人の投稿だけ流れる」は、投稿の entity を public(不特定多数の SNS)か shared(仲間内)にし、list の filter {"field":"$author","op":"eq","value":{"myFollows":true}} で作る。item.subtitle は "$author"、sort は {"field":"$createdAt","direction":"desc"}。フォローしていない人の投稿だけ(おすすめ・見つける)は op "neq"。
- フォロー / フォロー解除は、一覧の onItemTap で開く投稿の画面(その entity のフォーム)に button を 2 つ: {"type":"follow","entity":"<投稿の entity>"}(label「フォローする」)と {"type":"unfollow","entity":"<投稿の entity>"}(label「フォロー中」・style secondary・confirmMessage)。アプリが今の状態に合う方だけを出し、自分の投稿では両方隠す。
- **フォロー用の entity・相手を選ぶ ref / picker・フォロー数の computed・「フォローする」フォームを作らない**(フォローの関係はアプリ本体が持つ。誰でも他人をフォロー済みにできてしまう)。フォローはアプリごと・本人だけが見られる(相手に知らせず、フォロワーの一覧・人数は無い)。
- タブの定番: 「フォロー中」(上の filter の list)・「みんな」(filter なしの新着)・「自分」(filter {"field":"$author","op":"eq","value":{"me":true}})。いいねは marks。
- プロフィール = 投稿者の表示名($author。各自がアプリの設定で決める)。プロフィールの写真・自己紹介の入力欄や、人ごとのプロフィール画面は作らない(今はできない)。
- private の entity(自分だけの記録)には myFollows も follow も使わない。
- 編集ターン(「フォローできるようにして」「フォロー中のタブを足して」): 投稿の画面に button を add、/screens/- と /app/tabs/- にフォロー中のタブを add。entity が private なら /entities/<i>/scope も add。
`)

// followChatRule: what the chat answers about follows (できる?).
var followChatRule = strings.TrimSpace(`
## フォロー(#270)
- 「フォローした人の投稿だけが流れるタイムライン」(SNS)は**できる**。投稿をみんなに公開(または参加した人だけ)にし、投稿を開いた画面の「フォローする」ボタンで付け外しする。フォローはアプリごと・本人だけが見られ、相手に通知は届かず、フォロワーの一覧・人数は出せない。プロフィールは投稿者の表示名だけ(写真・自己紹介・人ごとのページはできない)。
`)

func init() {
	systemPrompt += "\n\n" + followRules
	// right after the hand-written capabilities, like matchingChatRule
	withFollow := chatCapabilities + "\n\n" + followChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, withFollow, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, withFollow, 1)
}
