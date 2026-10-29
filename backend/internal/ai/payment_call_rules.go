package ai

import (
	"regexp"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// paymentCallRules (#413 案 B・#424 案 A): generated apps never take payments
// that unlock paid content (App Store 3.1.1) and never make in-app voice /
// video calls. The request is declined in one honest sentence and the allowed
// alternative is built instead: an https payment link the author made
// (openUrlButton) plus amount / paid records, or tel: and shared records.
// No line has the 「…」は <type> form, so a review build (#245) keeps the
// decline even when openUrlButton is not allowed.
var paymentCallRules = strings.TrimSpace(`
## お金のやり取り・通話(#413・#424)
- **アプリの中でお金は動かさない**。「払ったら有料の記事が読める」「課金したら機能が使える」「有料会員だけ見られる」のように、支払いで中身・機能を解放する仕組みは作らない(払ったかどうかでロックを外す boolean や合言葉の入力でも作らない)。支払いを受け付ける画面も作らない。解放したい中身は普通の画面としてそのまま作り、app.description の末尾(編集は summary)に「` + paywallNoteJA + `」と書く(en: "` + paywallNoteEN + `")。
- 実物の商品・サービスの代金・駐車場代・ガソリン代・割り勘の支払いは、作者が言った **https の支払いリンク**を openUrlButton{label: 「PayPay で払う」等, url} で開く形にしてよい。金額・支払い済み(boolean)・支払った日は普通の entity の記録にする。**リンクの URL は作らない**(言われていなければボタンは置かず、記録だけ作って app.description に「支払いページのリンク(https)を教えてもらえれば、開くボタンを置けます。」と書く)。paypay:// などの独自スキームは使わない(https のリンクだけ)。
- **アプリの中での通話・ビデオ通話・リアルタイムの音声/映像のやり取りはできない**(WebSocket も無い)。「ビデオ通話したい」「アプリで通話」と言われたら作らず、app.description の末尾(編集は summary)に「` + callNoteJA + `」と書く(en: "` + callNoteEN + `")。電話番号を言われていれば openUrlButton{url: tel:番号} を置き、連絡・状況・位置の共有はみんなで使う記録(scope public / shared)の一覧にする。番号が無ければボタンは置かない(番号を作らない)。
`)

// The user-facing decline sentences, shared by the prompt and the server's
// fallback note (omissionNote) so both say the same thing.
const (
	paywallNoteJA = "アプリの中での支払いや、払ったら有料の中身が開く仕組みはできません。代わりに、ご自身で用意した支払いページ(PayPay の請求リンクやお店の EC ページ)を開くボタンと、金額・支払い済みの記録なら作れます。"
	paywallNoteEN = "The app can't take payments or unlock paid content. Instead, the app can have a button that opens a payment page you prepare (such as a PayPay request link or your shop page) and keep records of amounts and whether they're paid."
	callNoteJA    = "アプリの中での通話・ビデオ通話はできません。代わりに、電話をかけるボタンや、みんなで見られる記録での連絡・共有なら作れます。"
	callNoteEN    = "The app can't make voice or video calls. Instead, the app can have a button that places a phone call, or share updates through records everyone can see."
)

var (
	// paywallAskRe: paying unlocks content or features (not plain bills, 割り勘, 有料駐車場).
	paywallAskRe = regexp.MustCompile(`(払ったら|払うと|払えば|支払ったら|購入したら|買ったら|課金したら|課金すると|課金で).{0,20}(読め|見られ|見れ|開|使え|解放|解除)|有料(の|な)?(記事|機能|コンテンツ|会員|プラン|版|部分|ページ|動画)|(?i:paywall|unlock|premium (content|feature)|paid (content|article|feature|member))`)
	// callAskRe: an in-app voice / video call (「電話をかける」 is tel: and allowed).
	callAskRe = regexp.MustCompile(`ビデオ通話|音声通話|テレビ電話|通話(機能|したい|でき|する)|(?i:video ?call|voice call|facetime|call (each other|in the app))`)
)

// paymentCallOmissions back the rules up: a paywall or an in-app call is never
// built, so the request counts as omitted until an honest sentence names it.
var paymentCallOmissions = []omissionRule{
	{
		id:      "paywall",
		asked:   paywallAskRe.MatchString,
		omitted: func(string, *dsl.App) bool { return true },
		topic:   regexp.MustCompile(`支払|払|課金|決済|有料|(?i:pay|paid|unlock)`),
		problem: "依頼は支払いで中身・機能を解放する形だが、アプリの中での支払い・有料の中身の解放は作れない(App Store 3.1.1)。中身は普通の画面にし、app.description の末尾に「" + paywallNoteJA + "」と書く",
		noteJA:  func(string) string { return paywallNoteJA },
		noteEN:  func(string) string { return paywallNoteEN },
	},
	{
		id:      "call",
		asked:   callAskRe.MatchString,
		omitted: func(string, *dsl.App) bool { return true },
		topic:   regexp.MustCompile(`通話|電話|(?i:call)`),
		problem: "依頼にアプリの中での通話・ビデオ通話があるが、通話は作れない。電話番号があれば openUrlButton の tel: を置き、連絡・共有はみんなで使う記録にして、app.description の末尾に「" + callNoteJA + "」と書く",
		noteJA:  func(string) string { return callNoteJA },
		noteEN:  func(string) string { return callNoteEN },
	},
}

func init() {
	systemPrompt += "\n\n" + paymentCallRules
	omissionRules = append(omissionRules, paymentCallOmissions...)
}
