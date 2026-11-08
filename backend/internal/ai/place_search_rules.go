package ai

import "strings"

// placeSearchRules: "近くのカフェ / コワーキング / 映画館を探す" used to end in a
// hand-typed shop list (jphacks C_2214 / sp_2504 / tk_2419 / F_2202), and "今いる所
// を地名で" in raw "lat,lng" (KB_2308). placeSearch and locationButton.addressField
// ask Apple Maps from the device. Appended like routeRules so prompt.go stays
// untouched; the keys come from the schema catalog.
var placeSearchRules = strings.TrimSpace(`
## 近くの場所を探す(placeSearch)・現在地の住所(locationButton.addressField)
- 「近くの〇〇を探す」「周りのカフェ・飲食店・コワーキング・映画館・駅・病院・駐車場」「近くのお店から選んで記録」: 場所の entity(例 spot{name: string(required), place: location, address: string, memo?})のフォーム画面(screen.entity + saveRecord)に **placeSearch{nameField: name, locationField: place, addressField: address}** を A1:F7 に置き、下に入力欄と保存ボタン。施設の種類は category(cafe / restaurant / movieTheater / station / parking など)、種類に無い物(コワーキング・ラーメン・銭湯)は query に語を書く。自由に探すなら両方省略。ホームは spot の list と mapView{locationField: place, titleField: name}(行った・行きたい場所を地図で)。結果を地図でも見たいなら pins: true。
- placeSearch は近い順に名前・距離・住所を出し、「記録に追加」で入力中のフォームに入れるだけ(保存はユーザー)。探すのは現在地の近くだけ(使用中の位置)。営業時間・空席・値段・口コミ・上映時間・物件・電源や Wi-Fi の有無は**出ない**。求められたらメモの field(text)・rating・toggle で手で残す形にして summary で正直に伝える(「できません」で終わらせない)。
- 選んだ候補から抽選なら、記録した spot のホームに randomPicker{entity: spot, titleField: name} を足す(placeSearch 自体は選ぶだけ)。遠くの決まった場所を 1 つ選ぶなら locationPicker、そこへの案内は routeGuide。
- 「今いる場所を地名・住所で残す / 文章で伝える」: フォームの locationButton に addressField(string field)を付ける(例 locationButton{field: place, addressField: address})。共有して伝えるなら同じフォームに shareButton{text: "今ここにいます: {{address}}"}。
- 既存アプリへの「近くのお店を探せるようにして」は、その entity のフォーム画面に placeSearch を add する(location / string の field が無ければ add)。
`)

func init() { systemPrompt += "\n\n" + placeSearchRules }
