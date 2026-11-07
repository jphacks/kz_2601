package ai

import "strings"

// bookRules: "ISBN から本の情報を入れたい" used to end in hand-typed titles
// (jphacks NG_2312 LenBow). lookupBook asks appconfig (internal/books: one
// key-less source) and fills the form. Appended like calendarRules so
// prompt.go stays untouched; the keys themselves come from the schema catalog.
var bookRules = strings.TrimSpace(`
## ISBN から本の情報を入れる(lookupBook)
- 本の貸し借り・蔵書・読書記録・積読・本の販売の記録で「ISBN から書名を入れたい」「バーコードで本を登録」「本の情報を自動で」: 本の entity に isbn(string)・書名(string)・著者(string)・出版社(string)・出版日(date か string)を作り、その記録フォーム(screen.entity)に **qrScanner{field: isbn}(本のバーコードも読める)+ textField{field: isbn} + button{label: 「ISBN から入力」, action: {type: lookupBook, bookLookup:{isbnField, titleField, authorsField, publisherField, publishedDateField}}}** を保存ボタンの上に置く。bookLookup の field はその entity にある分だけ。押すと値が入り、保存は saveRecord(押しただけでは保存しない)。
- 書影(表紙の画像)・あらすじ・値段・ページ数は入らない。入るのは書名・著者・出版社・出版日だけ。著者は「山田玲子／著」の形の 1 つの文字なので、著者の entity や ref に分けない。
- 書名は手でも直せる。見つからない本・つながらない時は手入力に戻るので、書名の field を required にしてよい。本以外(商品の JAN コード・雑誌・論文の DOI)は引けない。できないものは「できません」とせず、手入力の形にして summary で正直に伝える。
- 既存アプリへの「ISBN から入れられるようにして」は、本の entity に isbn の string field が無ければ add し、そのフォーム画面に上のボタン(と qrScanner)を add する。
`)

func init() { systemPrompt += "\n\n" + bookRules }
