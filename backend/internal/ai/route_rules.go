package ai

import "strings"

// routeRules (#411): navigation requests used to become a mapView of pins or
// an openUrlButton to Maps. Appended like randomRules so prompt.go stays untouched.
var routeRules = strings.TrimSpace(`
## 道案内・ナビ(#411)
- 「ナビ」「道案内」「目的地まで案内」「ドライブで行き先まで連れて行って」「行き方・曲がる所を教えて」「音声で案内」は **routeGuide**(MapKit の経路を地図に線で出し、次に曲がる所・距離・残りの距離・到着の予想時刻を現在地に合わせて進める・音声案内のオン/オフ・車/徒歩の切り替え・道を外れたら引き直す)。**mapView のピンだけ・openUrlButton で地図アプリへ飛ばすだけで代替しない**。
- 目的地の決め方(locationField と address はどちらか一方):
  - 行き先を登録して選ぶ(ドライブの行き先・行きたいお店へ案内): entity 例 spot{name: string(required), place: location(required), memo?}、ホームに routeGuide{entity: spot, locationField: place, titleField: name}(A1:F7・部品の中で行き先を選ぶ)+ spot の list(A8:F10・onItemTap はフォーム)+ 追加ボタン(D11:F12)、フォーム画面(screen.entity spot + saveRecord)に textField name・locationPicker{field: place, nameField: name}。フォーム/詳細画面にも routeGuide{locationField: place, titleField: name}(entity は書かない = 開いているレコードの場所へ案内)を置いてよい。
  - 既存アプリの location field へ案内(「登録したお店まで案内して」): その entity の詳細/フォーム画面に routeGuide{locationField, titleField} を add するか、ホームに routeGuide{entity, locationField, titleField} を add する(entity・list は作り直さない)。
  - 行き先が決まっている(「会社まで」「金沢駅まで」): routeGuide{address: "金沢駅"}(住所・施設名をそのまま。entity は作らない)。
- 移動手段: 運転・ドライブ・車は mode automobile(既定)、散歩・歩いて・徒歩は mode walking。自転車・電車・バスの案内は**できない**(MapKit は車と徒歩だけ)。
- できないこと(求められたら summary で正直に伝える): 車線の案内・渋滞の回避・有料道路を避ける・寄り道(経由地)・アプリを閉じた後(画面を消した後)の案内・CarPlay。案内中の画面は消えないようにしてある。
`)

func init() { systemPrompt += "\n\n" + routeRules }
