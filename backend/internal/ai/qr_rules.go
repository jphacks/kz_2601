package ai

import "strings"

// qrRules (#422): which qrCode source to pick, and never invent the user's
// own URL / number. Appended like visualRules so prompt.go stays untouched.
var qrRules = strings.TrimSpace(`
## QR コードを作って見せる(#422)
- 「QR を出したい・見せたい」「QR にして」「会員証」「チケット」「自分の SNS・サイトの URL を QR で」は **qrCode**(作って表示する側)。カメラで読むのは qrScanner(逆にしない・両方要るなら両方置く)。写真への保存・共有は qrCode に入っているので shareButton を足さない。
- 値の出どころは text / field / value の**どれか 1 つ**: text = 生成時に分かる固定の値(お店の公式 URL 等)、field = 表示中のレコードの値(screen.entity の画面だけ)、value = 計算式(screen.entity の画面なら ref でそのレコード)。
- **本人の URL・アカウント・会員番号など生成時に分からない値は text に例(https://example.com 等)を書かない**。entity に入力させて field で出す。形: entity(例 card{name: string(required), number: string(required)} / profile{service: enum, url: string(required)})+ ホームに list(onItemTap でフォーム)+ 追加ボタン + フォーム画面(screen.entity・textField の下に qrCode{field, label: 「受付で見せてください」等}・saveRecord)。
- 「URL の前に決まった文字を付ける」は text の {{fieldId}}(例 https://x.com/{{account}})か value の concat。text の上限は 1000 文字(日本語は約 700 文字)。
`)

func init() { systemPrompt += "\n\n" + qrRules }
