package ai

import "strings"

// feedRules (#348): weather / holidays / allowlisted notices come from the
// feed leaf (read-only, fetched by appconfig from fixed public sources).
// Every line contains "」は feed", so a build without feed (App Store review
// build, older iOS) drops all of them in buildRestrictedPrompt.
var feedRules = strings.TrimSpace(`
- 「天気予報」「明日の天気」「週間天気」「〇〇の天気」は feed{provider: weather, region: 府県予報区のコード, title: 〇〇の天気, limit: 3〜7}(全幅・例 A1:F5)。地域は県・地方の単位(東京都 130000・大阪府 270000・愛知県 230000・石川県 170000・福岡県 400000・札幌は 016000・那覇は 471000)。市町村や現在地の天気・1 時間ごとの予報・雨雲レーダーはできないので、県の天気で作り summary で正直に伝える。地域を言われなければ 130000 にして「地域は言えば変えられます」と添える。entity・フォーム・保存ボタンは作らない。
- 「次の祝日」「祝日の一覧」「連休はいつ」は feed{provider: holidays, title: 次の祝日, limit: 5}。カレンダーに祝日を重ねる・祝日を条件に通知する、はできない。
- 「政府のお知らせ」「デジタル庁の新着」「消防庁の災害情報」は feed{provider: rss, feedId: digital-news / fdma-news / fdma-disaster}。ほかのサイト・ニュース・ブログの RSS や URL は指定できない(feedId はこの 3 つだけ)。
- 「天気を記録に保存したい」「祝日を計算・グラフに使いたい」は feed ではできない(読むだけ)。株価・為替・任意の API・外部への送信も無い。頼まれたら「天気・祝日・許可されたお知らせを読んで表示することだけできます」と伝える。
`)

func init() { systemPrompt += "\n\n" + feedRules }
