# 生成アプリからリマインダーに足すアクションの案(下書き・決めない)

> 下書き。**決めない**。案を並べて、良い点・困る点・OS ごとに作れるかを表にするだけ。決めるのはオーナー(親 #1810・まとめ #1778)。
> 調べた日: 2026-11-13(`epic/god` で確認)。決定文書・`docs/dsl`・openapi・スキーマは変えていない。
> 書き分け: **事実**=コードで確かめた物 / **推測**=確かめていない物 / **決めること**=人が選ぶ物。

## 目次

1. [今の形(事実)](#今の形事実)
2. [やりたいこと](#やりたいこと)
3. [案](#案)
4. [案の比較](#案の比較)
5. [OS ごとにできるか](#os-ごとにできるか)
6. [決めること](#決めること)

## 今の形(事実)

- `addToCalendar` は今のレコードを OS の「新規イベント」画面に埋めて開き、ユーザーが「追加」を押す形。iOS 17 以降この画面は別プロセスで動くので**許可が要らない**([`docs/dsl/README.md`](../dsl/README.md) の「カレンダーへの追加」・[`AppRuntime+Calendar.swift`](../../ios/Metamon/App/AppRuntime+Calendar.swift))。
- 欄の対応は `calendarEvent{startField, endField?, durationMinutes?, titleField?, locationField?, notesField?, alertMinutes?}`。型・検証は [`backend/pkg/dsl/calendar_event.go`](../../backend/pkg/dsl/calendar_event.go)。
- Mac は同名の処理を持つ([`MacAppRuntime+Calendar.swift`](../../ios/MetamonMac/App/Runtime/MacAppRuntime+Calendar.swift))。Android は `Intent.ACTION_INSERT`(`CalendarContract`)で予定の追加画面を開く([`AndroidRuntimeHost.kt`](../../android/app/src/main/kotlin/jp/kz2601/metamon/runtime/AndroidRuntimeHost.kt))。デスクトップは `.ics` を一時フォルダに書いて既定のカレンダーで開く([`Actions.kt`](../../desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop/Actions.kt))。
- **リマインダーを書く口は無い**。リマインダーは**読む**口だけがある(`DeviceCalendarSource.reminders(...)`・[`DeviceCalendar.swift`](../../ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Support/DeviceCalendar.swift))。許可の文言は `NSRemindersFullAccessUsageDescription`(「読み取ります」と書いてある)で、審査用の版では外す(`ios/scripts/strip-review-plist.sh`)。
- 「通知」としてのリマインドは別の仕組み(`scheduleNotification`・`app.notifications`)で既にできる。今回はリマインダー**アプリ**に項目を足す話。

## やりたいこと

- 生成アプリの記録(「牛乳を買う」「提出」)を、ボタン 1 つでリマインダーアプリに足す(題・期限・リスト)。
- `addToCalendar` と同じ書き心地で、AI が 1 つのアクションで書ける。

## 案

### 案 A: 専用のアクション `addToReminders`(`calendarEvent` の兄弟として `reminder{titleField, dueField?, listName?, notesField?}`)

```json
{ "type": "addToReminders", "entity": "todo",
  "reminder": { "titleField": "title", "dueField": "due", "notesField": "memo", "listName": "買い物" } }
```

(形の例。名前は仮・**決めること**)

### 案 B: `addToCalendar` を広げる(`calendarEvent.kind: "event" | "reminder"`)

アクションは 1 つのまま、欄の対応の中に種類を持つ。

### 案 C: リマインダーは作らず、`.ics` の `VTODO`(やること)を渡す形にする

デスクトップが `.ics` を渡す今の形に揃え、OS 側が「やること」として扱えるかに任せる。

## 案の比較

| 案 | 良い点 | 困る点 | 互換・検証 |
|---|---|---|---|
| A: 専用アクション | 欄(期限・リスト)が `calendarEvent` と別なので意味がはっきりする。AI のカタログでも「リマインダーに入れたい」の語で選びやすい。古い iOS は「未対応のアクション」の toast で済む(`addToCalendar` と同じ) | アクションが 1 つ増える(閉じた一覧に 1 つ足す)。検証の関数も 1 つ増える | Go に `reminder` の検証を足す(titleField は string / text、dueField は date / datetime) |
| B: 既存を広げる | アクションの数が増えない | 同じアクションで必須の欄が変わる(`startField` が必須か不要か)ので検証が複雑になり、AI が取り違えやすい。`startField` 必須という既存の規則を崩す | 既存の `addToCalendar` の検証・AI の例に影響する |
| C: `.ics` の VTODO | 新しいアクションが要らない・各 OS で似た形に揃えられる | iOS・Mac のリマインダーが `.ics` の VTODO を取り込めるかは未確認(**推測**: 取り込めない可能性がある)。Android には標準のリマインダーが無い | 実機確認が先に要る |

## OS ごとにできるか

| OS | 案 A で作れるか | 許可 | 根拠・注意 |
|---|---|---|---|
| iOS | ○ 作れる(**推測**) | 要る(**推測**) | EventKit でリマインダーを作る。リマインダーを書く時は許可が要り、`addToCalendar` のような「システムの画面を開くだけ」の抜け道は無い(**推測・要確認**)。今ある `NSRemindersFullAccessUsageDescription` は読むための文言なので、書くことを足した文言への更新が要る。書き込みの口は今は無い。Issue の前提は「iOS の書き込みの口の上に載せる」 |
| Mac | ○ 作れる(**推測**) | 要る(**推測**) | EventKit は Mac にもある。`addToCalendar` と同じく Mac 側の処理を足す |
| Android | △ 代わりに予定の追加(カレンダー) | 要らない | 標準の「リマインダー」アプリが無い。代わりは (1) カレンダーの予定として `ACTION_INSERT`(今の `addToCalendar` と同じ)、(2) 通知の `scheduleNotification`。どちらにするかは **決めること** |
| デスクトップ | △ `.ics` を渡す(VTODO か VEVENT か) | 要らない | 今の `addToCalendar` の流れが使える。既定のアプリが VTODO を開けるかは機種次第(**推測**) |

## 決めること

- 案 A / B / C のどれか(あるいは別案)。
- iOS・Mac で、リマインダーに書く許可を取る形にするか(許可の文言・審査用の版でどうするか・プライバシーの表示の更新が要る=人の作業)。許可を取らず、カレンダーの予定として足す形に倒す手もある。
- Android・デスクトップの代わり(カレンダーの予定・通知・`.ics`)。「リマインダーに入れる」の AI の返事が、OS ごとに違う動きになってよいか。
- リスト(`listName`)が無い時の扱い(既定のリストか・作るか)。
- `supportedFeatures`(機能の申告)のキー名(#1775 の判断待ち)。
