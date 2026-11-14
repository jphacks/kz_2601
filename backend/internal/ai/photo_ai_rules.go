package ai

import "strings"

// photoAIRules (#418): reading a photo (OCR / receipt amount / classify /
// describe / body-pose joint angle) is photoField.analyze, done on the device.
// Appended like randomRules so prompt.go stays untouched.
var photoAIRules = strings.TrimSpace(`
## 写真を AI で読み取る・骨格(#418)
- 「レシートを撮ったら金額を読み取って」「写真の文字を読んで」「写真から何か判定・分類」「写っているものをメモに」「筋トレ・ストレッチのフォームをチェック」「関節の角度を測る」は、フォームの **photoField に analyze を付ける**(端末内の AI で読み取り、同じレコードの別の項目に入れる。無料・写真は端末外に出ない)。aiButton や手入力だけで代替しない。
- analyze は [{kind, into}] の配列(最大 6)。into = 結果を入れる **同じ entity の保存される field**(photo field 自身・computed は不可・1 つの into に 1 つの kind)。読み取った値は入力欄で確認・修正できるよう、**into の入力欄(numberField / datePicker / picker / textField)を同じフォームに必ず置く**。
  - text: 写真の文字(OCR・日本語と英語)→ string / text。
  - amount: レシートの合計金額 → currency(unit 円)/ integer / number。
  - date: レシートの日付 → date / datetime。
  - classify: 候補から 1 つ → enum(options はその field の options)か、string / text + options(2〜12)。
  - describe: 写っているものの短い説明 → string / text。
  - jointAngle: 人の関節の角度(度)→ number(unit 度)/ integer。joint 必須: knee / elbow / hip / shoulder(左右のよく写っている方)か leftKnee / rightElbow など。写真に関節の点と線を重ねて表示する。
  - label: 写っている物・動植物・料理の名前 1 つ(「花の名前を記録」「何の料理か」)→ string / text。候補が決まっている分け方は classify。
  - labels: 写っている物の名前を最大 5 つ「、」区切り(「冷蔵庫の中身」「机の上の物」)→ string / text。
  - barcode: 写真のバーコード・QR の中身(本の ISBN・商品の JAN・URL)→ string / text。撮影した写真から読む。カメラで連続して読むのは qrScanner。
  - peopleCount: 写っている人の数(「集合写真の人数」「参加人数を写真で」)→ integer / number。誰かは見分けない。
  - color: 写真の中央の主な色の名前(「服の色」「花の色」)→ string / text。
- 形の例: 家計簿のレシート = expense{receipt: photo, amount: currency(required), date: date(defaultValue now), category: enum, memo: text} + フォームに photoField{field: receipt, source: camera, analyze: [{kind: amount, into: amount}, {kind: date, into: date}, {kind: classify, into: category}]}(全幅 5〜6 行・例 A1:F5)+ 金額・日付・分類の入力欄。筋トレのフォーム = check{photo: photo, knee_angle: number(unit 度), hip_angle: number, date} + photoField{field: photo, analyze: [{kind: jointAngle, joint: knee, into: knee_angle}, {kind: jointAngle, joint: hip, into: hip_angle}]} + 角度の numberField、ホームに角度の stat(avg)や list.trailing。
- 既存アプリへの「レシートを読み取れるようにして」「写真から金額を入れて」は、既存の photoField に analyze を add し、足りない into の field と入力欄だけ足す(写真の field が無ければ photo field と photoField も足す)。
- **できないこと**(求められたら summary で正直に伝える): 動画やカメラの映像を見ながらの回数カウント・リアルタイムのフォーム判定(写真 1 枚ずつの角度だけ)、レシートの品目ごとの明細を別々のレコードに分ける、顔で人を見分ける・人の名前を当てる、食べ物のカロリー・栄養の推定、植物や名所の詳しい種類・解説(label は一般的な名前まで)、2 枚の写真の見比べ。
`)

func init() { systemPrompt += "\n\n" + photoAIRules }
