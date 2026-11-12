# iOS の OS との連携(集中モード・Spotlight・電池など)

iOS 版が iOS の仕組み(集中モードのフィルタ・Spotlight・電池の状態など)とどうつながるかの決まりです。`epic/ios` に入っているものだけを書き、作業中のものは最後の [開いている PR](#開いている-pr) に並べます。

- どれも **生成したアプリの DSL・スキーマ・AI の決まりは変えません**。アプリの殻(`ios/Metamon/`)と `ios/Packages/MetamonServices/` だけで作っています。
- どれも値を **サーバーに送りません**。
- ウィジェット・ライブアクティビティ・Siri / ショートカット・プッシュ通知などは [architecture.md](architecture.md) の各章(§5.1・§12・§15)にあります。Mac で違う所は [Mac の OS との連携](../mac/system-integration.md) にあります。

## 目次

- [集中モードのフィルタ](#集中モードのフィルタ)
- [Spotlight で生成アプリを探す](#spotlight-で生成アプリを探す)
- [電池の残り・充電中・省電力モード(DevicePowerSource)](#電池の残り充電中省電力モードdevicepowersource)
- [開いている PR](#開いている-pr)

## 集中モードのフィルタ

設定 › 集中モード › フィルタを追加 に「Tailor It」を出し、集中モードの間に変身一覧へ出す生成アプリを選べます(#1991・`ios/Metamon/Features/Intents/TailorFocusFilter.swift`)。

| 項目 | 決まり |
| --- | --- |
| Intent | `TailorFocusFilter: SetFocusFilterIntent`。パラメータは「出すアプリ」(`[TailorAppEntity]`・§15 の Entity を共用)と「通知を止める」(Bool) |
| 保存 | `perform()` が `FocusFilterState` を JSON にして App Group の UserDefaults(`WidgetSnapshot.defaults`・キー `metamon.focusFilter`)に置く。集中モードが切れると空の値で呼ばれるので、空なら消す |
| 絞る所 | 変身一覧(`AppSwitcherSheet`)の表示だけ。`rt.visibleApps` は変えないので、Siri の照合・並べ替え・シェアは今のまま |
| 絞り方 | 選んだアプリを一覧の並びのまま残す。選んだアプリが 1 つも残っていない(消した等)時は全部出す(空の一覧にしない) |
| 一覧の表示 | アプリが隠れている間だけ、カードの上に「集中モード中: 選んだアプリだけを表示しています」と「すべて表示」のトグル。トグルは保存せず、シートを閉じるまで |
| 通知を止める | 今は保存だけ。通知とウィジェットを絞るのは [開いている PR](#開いている-pr) の #2030 |
| 未設定 | 保存データが無い(nil)ので一覧は今と同じ |
| 消す | サインアウト・アカウント削除で App Group を空にする時に一緒に消える |
| 審査用の版 | 文言を「記録帳」に言い換える(`APP_STORE_REVIEW`)。機能は外さない |
| Mac | 入れていない(iOS の target だけ) |
| テスト | `MetamonTests/FocusFilterStateTests` |

## Spotlight で生成アプリを探す

生成アプリの名前を Spotlight で探し、タップでそのアプリを開きます(#1996・`ios/Metamon/Features/Spotlight/`)。記録の中身はまだ入れません。

| 項目 | 決まり |
| --- | --- |
| 部品 | `SpotlightIndex` protocol(`index(apps:)`・`remove(appIds:)`・`removeAll()`)。実体は `CoreSpotlightIndex`(`CSSearchableIndex.default()`)。本物は `MetamonApp` だけが入れ、既定(ユニットテストなど)は何もしない `NoopSpotlightIndex` |
| 入れるもの | 生成アプリの名前と、DSL の `app.icon` の SF Symbols のサムネだけ。archived と未生成(version 1)は入れない。`GET /v1/apps` は自分のアプリだけ返すので、他の人が持ち主の共有アプリは入らない |
| ID | domain は `apps`、項目 id は `apps/<appId>`。記録を足す時用に domain `records.<appId>` を用意してあり、アプリを消す時はこの domain も消す |
| iOS 18 | `TailorAppEntity` を `IndexedEntity` にし、項目に `associateAppEntity` でつなぐ(開く Intent `OpenTailorAppIntent` と同じ Entity)。iOS 17 は `CSSearchableItem` だけ |
| 開く | `SpotlightOpenReceiver`(`CSSearchableItemActionType`)が appId を取り出し、`IntentHost.request(.open(appId:))`。Siri の「開く」と同じ道なので、サインイン・bootstrap の前に届いても準備の後に開く |
| 合わせる時 | `SpotlightAppIndexer.sync`: 一覧の取得が成功した時・起動時(live のみ・失敗しても黙る)・名前の変更・複製の後。一覧に無い id は消し、名前か版が変わったものだけ入れ直す |
| 差分の取り方 | 索引の中身は読めないので、入れた id → 「版\|名前」を UserDefaults(`metamon.spotlight.indexedApps`)に覚える |
| 消す | アプリを消した時は `remove`、サインアウト・アカウント削除は `removeAll`(覚えたキーも `LocalWipe` で消える) |
| Mac | まだ入れていない(同じ API で後から足す) |
| テスト | `MetamonTests/SpotlightIndexTests`(偽物の索引を使う) |

## 電池の残り・充電中・省電力モード(DevicePowerSource)

電池の状態を読む口だけを置いています(#1989・`ios/Packages/MetamonServices/Sources/MetamonServices/Native/DevicePowerSource.swift`)。**式・スキーマ・AI の決まり・`supportedFeatures`・きっかけにはまだ出しません**(#1775 の決定待ち。式に出すのは #1835)。

| 項目 | 決まり |
| --- | --- |
| 値 | `DevicePowerState`: 残り(0〜1・不明は nil)・充電の状態(unknown / unplugged / charging / full)・省電力モード。`isCharging` は満充電(full)も含む |
| 口 | `DevicePowerSource`(`@MainActor`): `current`・`observe(_:) -> DevicePowerObservation`・AsyncStream 版の `updates()`。値が変わった時だけ知らせる |
| 実体 | `LiveDevicePowerSource`: `UIDevice` と `ProcessInfo` から読む。`isBatteryMonitoringEnabled` は見ている間だけ有効にし、最後の監視が止まったら元の値に戻す。省電力モードの通知は main に寄せてから知らせる |
| 不明にする時 | シミュレータ・監視無効の `-1`、範囲外の値、充電の状態が unknown の時は残りを nil にする |
| Mac | `#if os(iOS)` の外では電池は不明・省電力モードだけ読む |
| 偽物 | `FakeDevicePowerSource`。`METAMON_MOCK=1` なら `DevicePowerSources.make()` が返す。初期値は `METAMON_BATTERY_LEVEL`・`METAMON_BATTERY_STATE`・`METAMON_LOW_POWER=1` |
| テスト | `MetamonServicesTests/DevicePowerSourceTests` |

## 開いている PR

`epic/ios` にまだ入っていないものです。入ったら上に章を足し、ここから消します。

- #2010 設定に「サインインの方法」を出し、今のアカウントに Apple をつなげる(#361)
- #2011 ヘルスケアから睡眠・心拍・距離・消費カロリー・運動・マインドフルネスを読む口(部品はまだ)(#1986)
- #2014 端末のカレンダーの予定とリマインダーを読む口(EventKit・部品はまだ)(#1987)
- #2017 アシスティブアクセスで開けるようにし、作る機能を隠して生成アプリだけ出す(#1997)
- #2019 ショートカットで生成アプリの数字(stat)を読む App Intent(#1994)
- #2030 集中モードのフィルタ中は、選んでいないアプリの通知とウィジェットを絞る(#1992)
- #2033 iOS 18 のコントロール(コントロールセンター・ロック画面・アクションボタン)から生成アプリを開く(#1993)
- #1995(Issue・作業中)今ある Intent でのオートメーションの案内
