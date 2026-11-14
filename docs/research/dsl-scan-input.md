# 書類のスキャンと文字の読み取りを photoField / textField の props にする案(下書き・決めない)

> 下書きです。**どの案にするかは決めません**(決めるのは人)。事実と、推測・決めることを分けて書きます。
> 親: #1826 #1827(まとめ #1782)・この下書き: #3032。正典の [`../dsl/dsl.schema.json`](../dsl/dsl.schema.json)・[`../dsl/README.md`](../dsl/README.md) は変えていません。

## 目次

- [1. 何を決めたいか](#1-何を決めたいか)
- [2. 今あるもの(事実)](#2-今あるもの事実)
- [3. 案](#3-案)
- [4. 案の比べ表](#4-案の比べ表)
- [5. OS ごとの口と、使えない OS の代わり](#5-os-ごとの口と使えない-os-の代わり)
- [6. 決めること](#6-決めること)

## 1. 何を決めたいか

各 OS が持つ「書類をスキャンする」「写っている文字を読む」口を、入力の部品の props(例 `source: scan`)として生成アプリから使えるようにするか。使えない OS ではどうするか。

## 2. 今あるもの(事実)

- `photoField`([`dsl.schema.json`](../dsl/dsl.schema.json) の `PhotoField`)
  - `source` は `camera` / `library` / `both` の 3 つだけ(`scan` は無い)。
  - `analyze` で、写真を撮った・選んだ直後に端末内で読み取り、同じ記録の他の項目に入れる。`kind: text` が文字(OCR)・`amount` / `date` がレシートの金額・日付・`barcode` が QR・バーコードの中身。読み取れなかった項目は今の値のまま。写真は端末の外に出さない・クラウドに送らない(`PhotoVision.swift`・`MlKitPhotoReader.kt` の注記)。
- `textField`(`TextField`)に、カメラで文字を取り込む props は無い(`field` / `label` / `placeholder` / `multiline` だけ)。
- iOS
  - 文字・金額・日付の読み取りは `ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Support/PhotoVision.swift`(Vision)。
  - 書類をスキャンする専用の画面(`VNDocumentCameraViewController`)は、今のコードに見当たらない(確認した範囲)。
- Mac: photoField に「iPhone から写真を撮る / 書類をスキャン」のボタンがある(#1852・`ContinuityCameraImport.swift`)。近くの iPhone を選ぶメニューが出て、受けるのは画像だけ。`analyze` があればすぐ読み取る。カメラの無い Mac でも隠さない([`../mac/system-integration.md`](../mac/system-integration.md#連係カメラiphone-で撮る書類をスキャン))。
- Android
  - 文字認識は ML Kit の日本語(`MlKitPhotoReader.kt`・PDF の画像ページは `PdfTextReader.kt`)。QR は Google Code Scanner(`play-services-code-scanner`・カメラの許可が要らない)。
  - ML Kit の書類スキャナ(Document Scanner)は今の依存に見当たらない(確認した範囲。推測: 足せば作れる)。
- デスクトップ: カメラは持たず、写真はファイル選択・貼り付け・ドロップで取り込む(`desktop/README.md`)。

## 3. 案

### 案 A: `photoField.source` に `scan` を足す

```json
{"type": "photoField", "field": "receipt", "source": "scan",
 "analyze": [{"kind": "text", "into": "memo"}, {"kind": "amount", "into": "price"}]}
```

撮る代わりに、その OS の書類スキャンの口(傾きの補正・ふちの検出がある画面)を開く。結果は今と同じ photo の項目に入り、`analyze` も今と同じに働く。使えない OS は `camera` / `library` と同じ動きに落とす。

### 案 B: `textField` に、写真を残さず文字だけ取り込む props を足す

```json
{"type": "textField", "field": "memo", "multiline": true, "scan": "text"}
```

文字の欄の横に「カメラで読み取る」を出す。撮った画像は保存せず、読み取った文字だけを欄に入れる(確認して直せる)。

### 案 C: DSL は変えず、今の `source: both` + `analyze: text` を AI に勧める

OS ごとの「スキャン」の見た目の違いは OS に任せる(例: Mac は今の連係カメラ・iOS は写真選択)。AI の説明に「書類は photoField + analyze text」と書くだけ。

## 4. 案の比べ表

| 案 | 良い点 | 困る点 | iOS | Android | Mac | デスクトップ |
|---|---|---|---|---|---|---|
| A: `source: scan` | 今の部品と `analyze` の流れをそのまま使える。書類と普通の写真を AI が言い分けられる | `source` の値が増え、古い版は未知の値になる(`supportedFeatures` で見分ける・[README §5.3](../dsl/README.md#53-backend-が古い-ios-に新しい部品を出さない仕組み))。書類の画像が記録に残る(個人の書類の扱いは決めること) | 作れる見込み(推測: `VNDocumentCameraViewController`。今のコードに無い) | 作れる見込み(推測: ML Kit の Document Scanner を足す) | 作れる(連係カメラの「書類をスキャン」が既にある) | 作れない(カメラ・スキャナが無い。ファイル選択に落とす) |
| B: `textField` の `scan` | 画像を残さないので、書類の中身を記録に残さず文字だけ使える | 部品の props が増え、OCR の結果の質が低い時に直す手間が出る。`analyze` と読み取りの形が 2 通りになる | 作れる(Vision) | 作れる(ML Kit) | 作れる(連係カメラ + Vision。受け取りの形が違う) | 作れない(画像ファイルを選んで読む形に落とす) |
| C: 変えない | スキーマ・AI の変更が最小 | 書類スキャンの専用の画面を AI が選べない。Mac だけ「書類をスキャン」が出る差が残る | 今のまま | 今のまま | 今のまま | 今のまま |

## 5. OS ごとの口と、使えない OS の代わり

| OS | スキャン(傾き補正つき) | 文字の読み取り | 今の状態 | 使えない・足りない時の代わり(案) |
|---|---|---|---|---|
| iOS | `VNDocumentCameraViewController`(推測・未実装) | Vision(`PhotoVision.swift`・実装済み) | 文字は可・スキャン画面は無し | カメラで撮る(`camera`)に落とす |
| Android | ML Kit Document Scanner(推測・未実装) | ML Kit 日本語(実装済み) | 文字は可・スキャン画面は無し | カメラ / 写真の選択に落とす |
| Mac | 連係カメラの「書類をスキャン」(実装済み) | Vision(`analyze` で実装済み) | 可(近くに iPhone がある時) | iPhone が無い時はファイル選択・貼り付け(今と同じ) |
| デスクトップ(Windows・Linux) | なし | 画像ファイルの文字読み取りは見当たらない(確認した範囲) | `analyze` の読み取りも見当たらない | 画像のファイル選択・貼り付け・ドロップ(今と同じ)。読み取りが無いと、`analyze` の項目は手入力のまま |

- 使えない OS で部品を隠さず、**別の入力に落とす**案が今の方針(Mac の連係カメラは「カメラの無い Mac でも隠さない」)。`source: scan` を知らない・使えない環境は `both` と同じに扱う案。
- 申告は `supportedFeatures` / `disabledFeatures`([`../dsl/platforms.md`](../dsl/platforms.md))。Android・デスクトップの `analyze` の対応は申告の列がまだ空(`platforms.md` の説明)。

## 6. 決めること

1. DSL に出すか(案 C のみ)。出すなら A / B のどちらか(併用も)。
2. 書類の画像を記録に残すか、文字だけにするか(個人の書類・個人の情報の扱い。共有・公開のアプリで使ってよいか)。
3. iOS・Android のスキャン画面を先に作るか(今は無い)。
4. 使えない OS の代わりの表示と、AI への教え方。
5. デスクトップで画像の文字を読むか(読まないなら `analyze` を使うアプリは手入力になることを教える)。
6. 複数ページのスキャンを 1 つの項目にどう入れるか(今の `photo` は 1 枚。推測: 最初の 1 ページだけにする案もある)。
