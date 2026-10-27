package ai

import "strings"

// parentChildRules (#256): comments / answers / RSVPs came out as forms that
// make the user pick the parent again, and a quiz list's onItemTap opened the
// answer form in edit mode with the question's id. pkg/dsl now rejects item
// taps into another entity's form; this teaches navigate.prefillRef.
//
// Older iOS builds ignore prefillRef: the child form opens new with an empty
// ref, which is today's behaviour, and the picker Validate requires on that
// form lets the user still choose the parent and save. So no feature flag.
var parentChildRules = strings.TrimSpace(`
## 親から子を作る(#256・コメント・回答・出欠・いいね)
- **一覧の行タップ(list / mapView / gridTable の onItemTap、calendar の onDateTap)の遷移先は同じ entity のフォーム**。タップした行の id を渡して編集モードで開くので、別 entity のフォームへ向けると該当なしの編集になる(検証エラー)。
- 親のレコードに付く記録(投稿へのコメント・いいね、練習・イベントへの出欠、候補日への回答、問題への解答、お店へのレビュー)は、子の entity に親を指す field {"id":"<親>","type":"ref","refEntity":"<親の entity>","required":true} を置き、**親のフォーム画面**(一覧の onItemTap で開く詳細)に button「コメントを書く」「出欠を答える」などを置いて {"type":"navigate","screen":"<子のフォーム>","prefillRef":"<その ref field id>"} で開く(押した親が入った新規フォームになり、親を選び直させない)。
  - 子のフォームにもその ref の picker を残す(必須。親が見えて選び直せる)。子のフォームを開く toolbar の「追加」をホームに別に置かない(親が入らない。上の「追加フォームを toolbar から開ける」の例外で、親のフォームのボタンが追加の入口)。子の一覧は item.subtitle にその ref field を出す。
  - 親の一覧の行から直接子を作る時だけ(「問題をタップして答える」)onItemTap に prefillRef を付ける。その場合、親を直すフォームは別の一覧か親のフォームへのボタンから開けるようにする。
  - 親ごとの数(コメント数・参加人数・いいね数)は親の computed {"agg":"count","entity":"<子>","filter":{"field":"<ref field>","op":"eq","value":{"ref":"id"}}} を親の一覧の trailing に出す。
  - prefillRef を書けるのは navigate だけ。screen.entity の無い画面の button / toolbar と calendar / gridTable のタップには付けない(今のレコードが無い・自前で日付やマスを入れる)。
- 編集ターン(「コメントを書けるようにして」「出欠をつけられるように」): 子の entity(親への ref + 中身)を /entities/- に add、子のフォーム画面(picker + 入力 + saveRecord)と子の一覧画面を /screens/- に add、親のフォーム画面の components/- に prefillRef 付きの button を add、子の一覧はタブか親の一覧画面のボタンから辿れるようにする。親のフォーム画面が無ければ作り、親の一覧の onItemTap をそこへ向ける。
`)

func init() { systemPrompt += "\n\n" + parentChildRules }
