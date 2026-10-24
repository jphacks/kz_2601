# チャット基本化 eval(#185 / 親 #184)— 2026-10-25

`POST /v1/chat` に edit・answer・proposal を足した版を、本物の Claude API で回した結果。

- モデル: chat = `claude-sonnet-5-5` / effort low / thinking 既定(ChatConfig の既定のまま)。generate = `claude-sonnet-5-5` low
- ケース: `backend/internal/ai/eval/chat_cases.json`(既存の create 11 本 + 今回足した 13 本)
- 実行: `go run ./cmd/ai-eval -chat -capabilities answer,proposal -env-file ../.env -tag chat-first-rN`
  - `-capabilities` を付けると、既存の create のケースも新しい経路(chat-first の create)で走る。既存の振る舞いが壊れていないかの確認を兼ねる
  - 旧 iOS の経路(mode も capabilities も無いリクエスト)はコードを変えていない。単体テスト `TestChatFirst_BadModeAndLegacyUntouched` と既存の chat テストで確認
- API 実行: 2 周(r1 → プロンプト修正 → r2)。raw: `raw/2026-10-25-chat-claude-sonnet-5-5-low-chat-first-r{1,2}.json`

## 結果

| 周 | ケース | ステップ | 失敗 |
|---|---|---|---|
| r1 | 20 / 24 | 27 / 31 | `kakeibo_short` `just_build` `kono_mama`(1 語なのに ready)、`edit_bigger_q`(question になった) |
| r2 | **24 / 24** | **31 / 31** | なし |

r1 → r2 で直したこと(`chat_first.go` のプロンプトだけ):
- create: 「アプリの種類だけの 1〜2 語(家計簿・買い物リスト…)は今までどおり question で 1 問聞く」を足した。「できること」の説明を足したら、1 語でも常識で埋めて ready にするようになったため
- edit: 「『〜できる?』『〜も可能?』は対象があいまいでも question にせず answer で答え、対象ごとの指示を suggestions に」を足した

### edit の対話(家計簿 `examples/kakeibo.json` を base、r2)

| ケース | 発話 | 期待 | 結果 | 答え / 案(要点) | 正しいか |
|---|---|---|---|---|---|
| edit_webview | これ現状は WebView でやってるんですか? | answer | answer | 「WebView ではありません。SwiftUI でネイティブに描画…外部サイトを表示する時だけその枠が WebView」 | ○(アプリに webView 部品は無い) |
| edit_chart_q | グラフも出せる? | answer | answer | 「すでに集計タブに『カテゴリ別(今月)』の円グラフと『月ごとの支出』の棒グラフがあります」+ 指示 3 つ | ○(DSL と一致) |
| edit_bigger_q | もうちょっと大きくすることも可能? | answer | answer | 「できます。今月の支出のカードや一覧の枠を広げたり文字を大きくしたり…ポイント数の指定はできません」 | ○(割り切りも正直) |
| edit_notify_q | 今このアプリって通知出る? | answer | answer | 「いまは通知は出ません。毎日の時刻通知なら追加できます」 | ○(通知なし) |
| edit_route_q | 地図で経路案内もできる? | answer | answer | 「経路案内はできません。代わりに場所をピンで表示・地図アプリを開くボタン」 | ○ |
| edit_en_chart | Can this app show a pie chart of my spending? | answer | answer | "Yes, it already does. The Stats tab has a pie chart…" | ○(ラベルは英訳して言っている) |
| edit_q_then_do | ホームの上にある数字って何? → じゃあそれを大きくして | answer → ready | answer → ready | 「今月の支出の合計と今月の件数です」→ transcript「ホーム画面の上の『今月の支出』と『今月の件数』の数字を大きく…」 | ○(会話が続く) |
| edit_mieyasuku | もっと見やすくして | proposal | proposal 3 案 | 数字を大きく / 予算の進捗バー / グラフをホームに | ○ 案 1 を generate → patch で通った(1.5 秒) |
| edit_family | 家族でも使いたい | proposal | proposal 3 案 | 「家族だけに限る設定はまだありませんが」+ みんなで共有 / 使った人を記録 / 人別グラフ | △ 案 1 を generate → 通ったが「投稿者名を一覧に表示」の部分は generate が「出せないので省いた」(下の懸念) |
| edit_amount_big | 金額を大きくして | ready | ready | transcript「ホーム画面の上の『今月の支出』の数字を大きく目立たせ、枠を広げて…」 | ○ generate → patch で通った(2.1 秒) |
| edit_delete_memo | メモ欄を消して | ready | ready | transcript「支出を記録画面の『メモ』入力欄を消して、一覧ではメモの代わりにカテゴリを行のタイトルに」 | ○(メモは一覧のタイトルなので代わりまで書いている) |

### create の新しい kind(r2)

| ケース | 発話 | 結果 |
|---|---|---|
| create_what | 何が作れるの? | answer「家計簿・筋トレ記録・日記・買い物リストなどの記録アプリが作れます。合計やグラフ、写真、通知、地図、共有も…」+ 例 3 つ |
| create_style | 家計簿を作りたいけど、どんな感じがいいかな? | proposal 3 案(シンプル / カテゴリ別グラフ付き / レシート写真つき)。instruction は transcript と同じ一人称の依頼文 |

既存の create のケース(1 語は question・具体的なら ready・「もう作って」「このまま作って」は shortcut・2 ラリー目まで曖昧なら 3 ラリー目で ready)は、r2 では 17 / 17 ステップが期待どおり。

## レイテンシ(iOS が受け取るまでの時間・r2)

answer / question / proposal は本文(kind・text・suggestions・options)がそろった時点で返す(#159 と同じ)。

| mode / kind | 件数 | p50 | 最大 |
|---|---|---|---|
| edit / answer | 7 | **1.7 秒** | 2.7 秒 |
| edit / ready | 3 | 2.0 秒 | 2.9 秒 |
| edit / proposal | 2 | 3.7 秒 | 3.8 秒 |
| create / question | 9 | 1.7 秒 | 2.1 秒 |
| create / ready | 6 | 2.6 秒 | 4.2 秒 |
| create / answer | 1 | 2.1 秒 | - |
| create / proposal | 1 | 3.7 秒 | - |
| 全体(Claude を呼んだステップ) | 29 | **2.1 秒** | p95 3.8 秒 |

- 目標の p50 3 秒以内は全体・answer・ready で達成。**proposal だけ 3.7 秒**(3 案 × 指示文で出力が長い)
- edit の入力は約 16.6K トークン(部品カタログ込みの system が大半。system は静的なのでプロンプトキャッシュに乗る)

## 懸念・次にやるなら

- proposal が 3 秒を超える。速くするなら (a) 案を 2 個に減らす (b) options を 1 案ずつストリームで返す (c) instruction を短くする
- 案の instruction に generate でできない要素が混じることがある(`edit_family` の「投稿者名を一覧に表示」)。generate は通るが一部が落ちる。「このアプリの仕組み」に投稿者名の扱いを明記すれば減らせる(iOS の表示仕様を確認してから)
- en の answer で日本語ラベルを英訳して言う(「Category (this month)」)。アプリの実際の表示と違うので、ラベルはそのまま引用させる方が正確
