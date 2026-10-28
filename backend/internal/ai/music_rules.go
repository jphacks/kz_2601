package ai

import "strings"

// musicRules (#412): music apps and the honest limits (Apple Music only,
// Spotify can only be opened, no audio relay between devices). Appended like
// visualRules so prompt.go / chat_first.go stay untouched.
var musicRules = strings.TrimSpace(`
## 音楽を流す(#412)
- 「音楽を流す」「ドライブの BGM」「曲を再生」「プレイリスト」「カラオケの曲を流す」は **musicPlayer**(Apple Music の曲を検索して再生待ちに入れ、再生・一時停止・次へ・前へ。イヤホンのボタン・ロック画面でも操作できる)。曲の entity・フォーム・list は作らない(再生待ちは端末のプレーヤーが持つ)。1 アプリに 1 つだけ。
- 形: ホーム(またはタブ)に musicPlayer{title?} を A1:F12 で 1 つ。メモ等を並べるなら musicPlayer を A1:F8 にして下に置く。
- 「友だち・同乗者のスマホから曲を入れたい / リクエストしたい」は allowRequests: true(近くの端末から曲名を送ってもらい、この端末の再生待ちに入れる)。「運転中で確認できない」「勝手に入れていい」なら autoAccept: true も付ける。
- 再生元は **Apple Music だけ**(Apple Music の契約が要る)。**Spotify・YouTube Music の再生・操作はできない**: Spotify を頼まれたら musicPlayer で作り、summary(新規作成なら app.description の末尾)に「Spotify は曲名から Spotify アプリを開くことだけできます。再生と操作は Apple Music で行います。」と正直に書く。
- 「Bluetooth で友だちの端末をつないで音を流す」「友だちのスマホの音をこっちで鳴らす」は iOS ではできない(端末どうしで音声を中継できない)。allowRequests(友だちが曲をリクエスト・再生はこの端末)で代え、そのことを summary に書く。
`)

// musicChatRule: the same limits for the chat answers (できる? / Spotify で流したい).
var musicChatRule = strings.TrimSpace(`
## 音楽(#412)
- Apple Music の曲を検索して再生・一時停止・次へ・再生待ちに入れる音楽プレーヤーを**作れる**(Apple Music の契約が要る。イヤホンのボタン・ロック画面でも操作できる)。近くにいる友だちのスマホから曲をリクエストして、この端末の再生待ちに入れることも**できる**。
- **Spotify は曲名から Spotify アプリを開くことだけできる**。Spotify の曲をこのアプリで再生・一時停止・曲送りすることは**できない**(Spotify 側の開発者登録・Premium・審査が要るため)。Spotify で流したいと言われたら「Spotify の操作はできませんが、Apple Music なら再生と操作ができ、Spotify はアプリで開けます」と正直に答え、Apple Music のプレーヤーを案にする。
- Bluetooth で友だちの端末をつないで音を流す・端末どうしで音声を中継することは**できない**(代わり → 友だちが曲をリクエストし、この端末で再生)。
`)

// musicNoRequestsNote: builds without nearby (App Store review build, #245)
// also lack the friend requests, which use the same peer-to-peer link.
const musicNoRequestsNote = "- musicPlayer の allowRequests / autoAccept は使わない(近くの端末とのやりとりはこの版ではできない。友だちからの曲のリクエストを「できます」と言わない。再生は使える)。\n"

func init() {
	systemPrompt += "\n\n" + musicRules
	chatEditSystemPrompt += "\n\n" + musicChatRule
	// the create prompt keeps ending with the capability index (restrict.go swaps it)
	chatCreateSystemPrompt = strings.TrimSuffix(chatCreateSystemPrompt, chatCapabilityIndex) + musicChatRule + "\n\n" + chatCapabilityIndex
}
