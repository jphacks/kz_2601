package ai

import "strings"

// chatThreadRules (#260): chat apps came out as a list + an "add message"
// form sheet. Appended like visualRules so prompt.go stays untouched.
var chatThreadRules = strings.TrimSpace(`
## チャット・メッセージのアプリ(#260)
- 「チャット」「メッセージを送り合う」「1 対 1 / DM」「グループで会話」「連絡を取り合う」は **chatThread** を使う(list + 追加フォームで代用しない)。entity は本文の text field(required)と任意の photo field だけ + scope:"public"。**送信者の名前・送信日時の field は作らない**(chatThread が $author / $createdAt で自動表示)。
- 画面: チャット用のタブ(またはホーム)に chatThread{entity, textField, photoField?} を A1:F12 で 1 つだけ置き、同じ画面に他の部品を置かない。**フォーム画面・追加ボタン・メッセージの list は作らない**(下の入力欄で送ると保存される)。
- 新しいメッセージの通知(プッシュ)はできない。求められたら「新しいメッセージの通知は今はできないため、開いた時にすぐ届く形で代替しました。」と申告する。
`)

func init() { systemPrompt += "\n\n" + chatThreadRules }
