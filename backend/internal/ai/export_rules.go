package ai

import "strings"

// exportRules (#421): "CSV / PDF にしたい" used to be answered with a
// shareButton (text only). Appended like randomRules so prompt.go stays
// untouched.
var exportRules = strings.TrimSpace(`
## 書き出し(CSV / PDF・#421)
- 「CSV で出したい」「Excel で開きたい」「表計算に貼りたい」は **button{action: export{entity, format: csv}}**、「PDF にしたい」「月の報告書」「印刷したい」「報告をメールで送りたい」は **export{format: pdf}**。押すと共有シート(ファイル・メール・AirDrop)が開く。**shareButton(文面だけ)や「できません」で代替しない**。
- 置き場所: 一覧・表・グラフのある画面の下部の半幅(A11:C12・追加ボタン D11:F12 の隣)か toolbar{label: 「書き出す」, icon: square.and.arrow.up}。CSV と PDF を両方求められたらボタンを 2 つ(hstack か半幅ずつ)。
- 範囲: 「今月分」「今年の」は export.filter{field: 日付の field, op: thisMonth / thisYear, value: null}(list.filter と同じ書き方)。画面の list / table に filter があれば同じ filter を付ける。
- 列: 省略で全項目(計算項目を含む・写真と動画は除く)。「日付と金額だけ」のように言われた時だけ fields: [field id…]。title はファイル名と PDF の見出し(例「今月の家計簿」)。
- PDF には同じ画面にある同じ entity の chart が表の上に入る。「グラフ付きの報告」と言われたら、その画面に chart を置いてから export{format: pdf} を同じ画面に置く。
- 既存アプリへの「CSV で出せるようにして」は、記録の一覧がある画面に export のボタンを add する(entity・list は作り直さない)。
`)

func init() { systemPrompt += "\n\n" + exportRules }
