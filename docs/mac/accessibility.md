# Mac の Accessibility Inspector の点検

#1206 の最後の作業「Accessibility Inspector で主な画面を点検し、結果を残す」の記録です(#2855)。Mac 版の主な 3 画面(作る → 記録を入れる → 一覧)で Inspector の監査(Audit)を回し、出た警告と対応を表に残します。

まだ実機で点検していません。表の「状態」はすべて **未点検** です。点検した人が下の手順で回し、行を書き替えます。

## 目次

- [点検する画面](#点検する画面)
- [点検の手順](#点検の手順)
- [結果の表](#結果の表)
- [状態の書き方](#状態の書き方)
- [コードで入れてある対応](#コードで入れてある対応)

## 点検する画面

| 画面 | 開き方 | 主な部品 |
| --- | --- | --- |
| 作る | ファイル メニューの「文字で頼む」(⌘K) / 「声で頼む」(⇧⌘M)で会話のシートを開く | 頼む文の入力欄・マイク・送る・頼み方のコツ・案のカード |
| 記録を入れる | 作ったアプリ(モックの「家計簿」)で追加のボタンを押し、フォームのシートを開く | 入力欄・選ぶ欄・日付・保存のバー・閉じる |
| 一覧 | 同じアプリの一覧の画面(中央の列) | 一覧の行・見出し・サイドバー・ツールバー |

## 点検の手順

1. モックで起動する(サーバー・課金に触れない)。Xcode の scheme `MetamonMac` の Run の環境変数に `METAMON_MOCK=1`・`METAMON_MOCK_STORE=1`・`METAMON_AI_CONSENT=granted`・`METAMON_BETA_WELCOME=seen` を入れる(`ios/MetamonMacUITests/KeyboardShellUITests.swift` と同じ)。
2. Xcode のメニュー > Open Developer Tool > Accessibility Inspector を開き、左上の対象に「Tailor It」を選ぶ。
3. 上の 3 画面を 1 つずつ前面に出し、Inspector の Audit(三角の警告のアイコン)> Run Audit を押す。
4. 出た警告ごとに、表の 1 行にする(画面・部品・警告の種類・状態・メモ)。警告が 0 件の画面も「警告なし」の行を 1 つ残す。
5. 同じ画面を、システム設定 > アクセシビリティ > ディスプレイ の「透明度を下げる」「コントラストを上げる」を入れた状態でも回す。表示メニューの「拡大」(⌘+)を上限まで上げた状態でも回す(文字の切れ・重なり)。
6. VoiceOver(⌘F5)で各画面を一通り読み、Audit に出ない問題(読む順・名前の無いボタン)があれば同じ表に足す。
7. 直した行は直した PR の番号を、残す行は理由をメモに書く。

Inspector の警告の種類(書く時の名前):

- Element has no description(名前が無い)
- Contrast nearly passed / Contrast failed(文字と背景の差が足りない)
- Hit area is too small(押せる所が小さい)
- Dynamic Type font sizes are unsupported(文字の大きさが変わらない)
- Element has no accessibility role など、そのほか Inspector が出す文言

## 結果の表

| 画面 | 部品 | 警告の種類 | 状態 | メモ |
| --- | --- | --- | --- | --- |
| 作る | 画面全体の Audit | — | 未点検 | 実機の点検がまだ |
| 作る | 透明度を下げる・コントラストを上げる | — | 未点検 | 実機の点検がまだ |
| 作る | 文字の拡大(上限) | — | 未点検 | 会話のシートは中央のアプリではないので ⌘+ の対象外。OS の設定の文字の大きさで見る |
| 作る | VoiceOver で読む順 | — | 未点検 | 実機の点検がまだ |
| 記録を入れる | 画面全体の Audit | — | 未点検 | 実機の点検がまだ |
| 記録を入れる | 透明度を下げる・コントラストを上げる | — | 未点検 | 実機の点検がまだ |
| 記録を入れる | 文字の拡大(上限) | — | 未点検 | ⌘+ でフォームのシートの文字も上がる(`AppCanvas` の `dynamicTypeSize`) |
| 記録を入れる | VoiceOver で読む順 | — | 未点検 | 実機の点検がまだ |
| 一覧 | 画面全体の Audit | — | 未点検 | 実機の点検がまだ |
| 一覧 | 透明度を下げる・コントラストを上げる | — | 未点検 | 実機の点検がまだ |
| 一覧 | 文字の拡大(上限) | — | 未点検 | サイドバーは OS の設定のまま。中央の列だけ上がるのが正しい |
| 一覧 | VoiceOver で読む順 | — | 未点検 | 実機の点検がまだ |

## 状態の書き方

| 状態 | 意味 |
| --- | --- |
| 未点検 | まだ Inspector を回していない |
| 警告なし | 回して警告が出なかった |
| 直した | 警告を直した。メモに PR の番号 |
| 理由があって残す | 直さない。メモに理由(例: OS の部品で手が出せない・飾りの画像で読ませない) |
| 直す予定 | Issue にした。メモに Issue の番号 |

点検した行は、警告 1 つにつき 1 行に分けます(「画面全体の Audit」の行を、出た警告の行に置き換える)。

## コードで入れてある対応

点検の前から入っているもの(#937・#1206)。点検で警告が出た時に、どこを直せばよいかの手がかりです。

| 対応 | 場所 | テスト |
| --- | --- | --- |
| 透明度を下げる時は不透明な面・コントラストを上げる時は濃く太い枠と本文の色の補足 | `ios/MetamonMac/App/Theme/MacAccessibility.swift`(`MacSurfaceStyle`) | `MacAccessibilityStyleTests` |
| 中央のアプリの文字の拡大 ⌘+ / 縮小 ⌘- / 実際のサイズ ⌘0(上限 accessibility3) | `MacTextZoom`・`ios/MetamonMac/App/Shell/MacCommands.swift`・`AppCanvas.swift` | `MacAccessibilityStyleTests` |
| 視差効果を減らす時は動かさずに変える | `MacMotion` | `MacVoiceOverTests` |
| フルキーボードアクセスの時だけ広い面のフォーカスの輪 | `MacFocusRing` | `MacVoiceOverTests` |
| 一言を出すのと同時に VoiceOver で読む | `ios/MetamonMac/App/Shell/MacVoiceOver.swift` | `MacVoiceOverTests` |
| carousel・disclosure・見出しの名前と値 | 生成アプリの部品(Renderer) | `MetamonMacUITests/AccessibilityTreeUITests`(手元だけ) |
