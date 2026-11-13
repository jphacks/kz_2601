# 通知のボタンと返信で記録する DSL の案(下書き・決めない)

> 下書き。**決めない**。案を並べて、良い点・困る点・OS ごとに作れるかを表にするだけ。決めるのはオーナー(親 #1802・まとめ #1777)。
> 調べた日: 2026-11-13(`epic/god` で確認)。決定文書・`docs/dsl`・openapi・スキーマは変えていない。
> 書き分け: **事実**=コードで確かめた物 / **推測**=確かめていない物 / **決めること**=人が選ぶ物。

## 目次

1. [今の形(事実)](#今の形事実)
2. [やりたいこと](#やりたいこと)
3. [案](#案)
4. [案の比較](#案の比較)
5. [OS ごとにできるか](#os-ごとにできるか)
6. [押した時の確認](#押した時の確認)
7. [決めること](#決めること)

## 今の形(事実)

- 通知は 2 通り。`app.notifications[]`(宣言・定期やレコードの時刻から)と、アクション `scheduleNotification`(その場で予約)。どちらも同じ `Notification` 型(`id`・`title`・`body`・`repeat`・`time`・`entity`・`field`・`when`・`near` など)。型は [`backend/pkg/dsl/schema.json`](../../backend/pkg/dsl/schema.json) の `Notification`、Go は [`backend/pkg/dsl/dsl.go`](../../backend/pkg/dsl/dsl.go)。
- `Notification` に**ボタンも返信の欄も無い**。通知を押すとアプリが開くだけ。
- コードに通知のボタン(`UNNotificationAction`・`UNTextInputNotificationAction`・`NotificationCompat.Action`・`RemoteInput`)は **iOS・Android・デスクトップのどれにも無い**(`epic/god` を grep)。iOS・Mac の土台は Issue #2231(iOS)・#2267(Mac)が「カテゴリの登録と応答を 1 つの型にして本体に渡す」までを担当する予定。受けた後の動きは未定。
- アクションは閉じた一覧(`ActionType`・16 種)で、`saveRecord`(フォームの値を保存)・`navigate` などがある。通知から呼べるアクションを書く欄は、`Notification` には無い。
- Android の通知は AlarmManager → `NotificationReceiver` で出す([`LocalNotifications.kt`](../../android/app/src/main/kotlin/jp/kz2601/metamon/notifications/LocalNotifications.kt))。デスクトップは「トレイの通知(起動中だけ・時刻は次の 1 回)」([`Actions.kt`](../../desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop/Actions.kt))。

## やりたいこと

- 通知の上に「飲んだ」「あとで」のようなボタンを出し、押したら**アプリを開かずに**記録を 1 件足す。
- 「今日の気分は?」のように通知から文字で返信して、その文字を記録の欄に入れる。
- 生成アプリの DSL に、AI が 1 か所に書けば足りる形で入れられること(自由な文字列の `feature` を作らせない・[`docs/dsl/README.md`](../dsl/README.md) の決定表 17 の考え方)。

## 案

### 案 A: `Notification` に `buttons[]` と `reply` を足す

通知の宣言の中に、ボタンの一覧と返信の欄を持たせる。

```json
{ "id": "med", "title": "薬", "body": "飲みましたか?", "repeat": "daily", "time": "08:00",
  "buttons": [ { "id": "done", "label": "飲んだ", "record": { "entity": "med_log", "set": { "taken": true } } },
               { "id": "later", "label": "あとで", "action": { "type": "scheduleNotification", "...": "10 分後" } } ],
  "reply": { "label": "メモ", "record": { "entity": "med_log", "field": "note" } } }
```

(上は形の例。名前は仮・**決めること**)

### 案 B: 通知は今のまま。ボタンは「アクションの列」を別の置き場に書く

`app.notificationActions[]`(通知の id と、ボタンごとのアクション)を `app` の直下に置き、`Notification` は変えない。

### 案 C: ボタンを押したら「画面を開いて、その画面で記録」だけにする(記録はアプリ内)

ボタンは `navigate` 相当の「この画面を開く」だけ。通知の返信は持たない。アプリを開かずに記録する機能は作らない。

## 案の比較

| 案 | 良い点 | 困る点 | 互換・検証 |
|---|---|---|---|
| A: `Notification` に欄を足す | 通知とボタンが 1 か所に書かれる。AI が書きやすい。`repeat`・`near`・`when` の通知すべてで同じ書き方になる | `Notification` が太る。`scheduleNotification`(アクション)の中の通知にも欄が付くので、入れ子(アクション → 通知 → アクション)の深さを止める規則が要る | 未知キーは iOS が無視するので古いビルドは落ちない(推測)。Go の検証に「ボタンの記録先 entity・field が在るか」を足す |
| B: 別の置き場 | `Notification` は変えない(既存の検証・AI の例に影響が少ない)。同じボタンを複数の通知で使い回せる | 通知とボタンが 2 か所に分かれて AI が整合を崩しやすい(id の取り違え)。`scheduleNotification` で作った通知にはボタンを付けにくい | 参照 id の整合を Go で検証する必要 |
| C: 開くだけ | 作りやすい(Android・デスクトップでも同じ動き)。入力内容を通知の上で扱わないので、間違いの記録が出にくい | 「開かずに記録」ができない。ボタンの利点がほぼ無い | 既存の `navigate` で足りる(スキーマ変更ほぼ無し) |

## OS ごとにできるか

| OS | ボタン | 文字の返信 | アプリを開かずに記録 | 根拠・注意 |
|---|---|---|---|---|
| iOS | ○ 作れる | ○ 作れる | ○ 条件つき(**推測**) | `UNNotificationAction`・`UNTextInputNotificationAction`。土台は #2231(未マージ・ログと DEBUG のトーストまで)。アプリが起きていない時に記録を書く経路は未確認 |
| Mac | ○ 作れる | ○ 作れる | ○ 条件つき(**推測**) | macOS にも同じ API がある。土台は #2267(iOS の上に載せる前提) |
| Android | ○ 作れる(**推測**) | ○ 作れる(**推測**) | ○ 条件つき(**推測**) | `NotificationCompat.Action`・`RemoteInput`・`BroadcastReceiver` で受ける。今は `NotificationReceiver` に無い。ボタンの数・文字の返信の出方は機種・Android の版で違う(推測) |
| デスクトップ | △ 案 C 相当だけ(**推測**) | × | × | 今の通知は Java のトレイの通知で、ボタンや返信の API が無い(推測)。起動中しか出ないので「開かずに記録」は成り立たない |

## 押した時の確認

押した時にどうするかも案の一部。**決めること**。

| 選び方 | 内容 | 良い点 | 困る点 |
|---|---|---|---|
| 確認なしで記録 | 押したらすぐ 1 件足す | 一番速い | 押し間違いで記録が増える(取り消しの手段が要る) |
| 記録してから「取り消す」を出す | 記録後に通知の置き換えか、アプリ内に「取り消す」 | 速さと間違い直しを両立 | 取り消しの置き場の設計が要る |
| 確認を挟む | 押すとアプリを開いて確認画面 | 間違えにくい | 「開かずに記録」の利点が消える |

文字の返信は、入力した文字をそのまま 1 つの欄に入れる(長さの上限・空文字は入れない)ことを前提にした案。個人情報の扱い(通知の本文を画面ロック中に見せるか)は **決めること**で、ここでは決めない。

## 決めること

- 案 A / B / C のどれか(あるいは別案)。
- 押した時の確認(上の表)。
- 通知から呼べるアクションを、`saveRecord` 系に限るか、閉じた一覧のどこまでにするか。
- 古い版・Android・デスクトップでボタンが出ない時の見え方(押せない代わりに、アプリを開く形でよいか)。
- `supportedFeatures`(機能の申告)に足すキーの名前(#1775 の判断待ち)。
