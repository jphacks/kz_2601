package ai

import "strings"

// aiPhotoInputRules: when the generator should set aiButton inputPhoto (the
// cloud AI reads one photo of the form, text_image.go). Appended like
// aiLongRules so prompt.go and catalog.go stay untouched.
var aiPhotoInputRules = strings.TrimSpace(`
## アプリの中の AI に写真を見せる(aiButton inputPhoto)
- 「写真を AI に読ませて説明・まとめ・相談」(処方箋や薬の袋を撮って薬の名前と飲み方の書いてあることをまとめる、ラベルや成分表を読んで説明、ホワイトボードや手書きのメモを清書、料理・植物・部屋の写真に助言)は、フォーム(screen.entity)に photo field と photoField を置き、aiButton に inputPhoto: <その photo field> を付ける。結果は output(text field)に入れる。
  - 形: 薬の記録 = medicine{photo: photo, name: string, note: text, date: date(defaultValue now)}。フォームに photoField{field: photo, source: camera}・aiButton{task: extract, inputPhoto: photo, output: note, prompt: 「写真の処方箋・薬の袋から、薬の名前・1 回の量・飲む時間を書いてある通りに」, label: 写真を AI で読む}・textField(note)。ホームに medicine の list。
  - 写真だけで材料になる(input は任意)。文字の field も一緒に見せたい時は input も付ける。長い説明は length: long も付けてよい。
- 文字・金額・日付・分類を端末の中で読むだけで足りるなら photoField の analyze(無料・写真は端末外に出ない)を先に使う。inputPhoto は analyze ではできない説明・要約・助言の時だけ。
- inputPhoto は端末内 AI では動かず、いつも Tailor It Pro のオンライン AI で動く(写真は縮小して送る)。1 つのボタンで写真は 1 枚。薬・健康の写真の答えは書いてあることの読み取りまでで、量や飲み方の判断はしない(summary で正直に伝える)。summary で料金に触れなくてよい。
`)

func init() { systemPrompt += "\n\n" + aiPhotoInputRules }
