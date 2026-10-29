package ai

import "strings"

// nearbyPassbyRules (#425): passby exchange and UWB distance are a mode of
// nearbyShare. Appended like visualRules so prompt.go stays untouched.
var nearbyPassbyRules = strings.TrimSpace(`
## すれ違い通信・近くの人との距離(#425)
- 「すれ違った人とプロフィールを交換」「すれ違い通信」「近くにいる同じアプリの人と自動で交換」「会った人の記録」「近くの友だちとの距離・方向を表示」は nearbyShare の **mode:"passby"**(距離・方向は **distance:true** を足す。距離だけの別部品は無い)。ボタンで相手を選ぶ send(既定)ではない。
- 形: 自分のプロフィールの entity(例 profile{name: string(required), comment: string, hobby: enum(任意)}・**scope は private のまま**。受け取った相手のプロフィールも同じ entity に追加される)+ すれ違いの画面(タブ)にその部品{mode:"passby", label:「すれ違い通信」, entity:profile, fields:[送る field id], limit?:1, distance?:true}(A1:F8)+ 交換したプロフィールの list(A9:F12)+ 自分のプロフィールを書くフォーム画面(screen.entity + saveRecord)への導線。
- **fields には送ってよい項目だけを明示する**(名前・ひとこと・趣味など)。hidden・photo・video・ref・refs・computed は指定できない。電話番号・住所・メールなど個人の連絡先は、ユーザーが明示した時だけ入れる。
- できないこと: アプリを裏に回した・画面を閉じた後のすれ違い(iOS の制約。画面を開いている間だけ動く)/ 写真・動画の交換 / 距離・方向は対応端末(iPhone 11 以降・SE を除く)だけ。求められたら summary で正直に伝える。
`)

func init() { systemPrompt += "\n\n" + nearbyPassbyRules }
