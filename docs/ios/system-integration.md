# iOS の OS との連携(集中モード・Spotlight・ショートカット・電池・カレンダー・翻訳など)

iOS 版が iOS の仕組み(集中モードのフィルタ・Spotlight・ショートカット・電池の状態・カレンダー・端末の中の翻訳など)とどうつながるかの決まりです。`epic/ios` に入っているものだけを書き、作業中のものは最後の [開いている PR](#開いている-pr) に並べます。

- どれも **生成したアプリの DSL・スキーマ・AI の決まりは変えません**。アプリの殻(`ios/Metamon/`)と `ios/Packages/MetamonServices/`・`ios/Packages/MetamonRenderer/` の `Support/` だけで作っています。
- どれも値を **サーバーに送りません**。
- ウィジェット・ライブアクティビティ・Siri / ショートカット・プッシュ通知などは [architecture.md](architecture.md) の各章(§5.1・§12・§15)にあります。Mac で違う所は [Mac の OS との連携](../mac/system-integration.md) にあります。

## 目次

- [集中モードのフィルタ](#集中モードのフィルタ)
- [Spotlight で生成アプリを探す](#spotlight-で生成アプリを探す)
- [ショートカットで数字(stat)を読む](#ショートカットで数字statを読む)
- [電池の残り・充電中・省電力モード(DevicePowerSource)](#電池の残り充電中省電力モードdevicepowersource)
- [カレンダーの予定とリマインダーを読む(DeviceCalendarSource)](#カレンダーの予定とリマインダーを読むdevicecalendarsource)
- [端末の中で文字を訳す(OnDeviceTranslator・iOS 18)](#端末の中で文字を訳すondevicetranslatorios-18)
- [連絡先から 1 人選ぶ口(ContactPick)](#連絡先から-1-人選ぶ口contactpick)
- [画面を消さない(ScreenAwake)](#画面を消さないscreenawake)
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
| 通知を止める | 下の「通知とウィジェットを絞る」 |
| 未設定 | 保存データが無い(nil)ので一覧は今と同じ |
| 消す | サインアウト・アカウント削除で App Group を空にする時に一緒に消える |
| 審査用の版 | 文言を「記録帳」に言い換える(`APP_STORE_REVIEW`)。機能は外さない |
| Mac | 入れていない(iOS の target だけ) |
| テスト | `MetamonTests/FocusFilterStateTests` |

### 通知とウィジェットを絞る

フィルタの選択に合わせて、生成アプリのローカル通知とウィジェットの数字を絞ります(#1992)。`FocusFilterState` は本体とウィジェットの両方にコンパイルする `ios/MetamonWidget/Shared/FocusFilterState.swift` に置き、出すかどうかの判定(`allowsNotification`・`notificationFilterPredicate`・`allowsWidget`・`pausesWidget`)をここにまとめています。一覧の絞り込み(`filter`)は本体の extension です。

| 項目 | 決まり |
| --- | --- |
| 通知の目印 | 生成アプリのローカル通知(予約分・ボタンで予約した分・条件で出す分)の `filterCriteria` に生成アプリの id を入れる(`UserNotificationScheduler`)。予約の識別子から id を引くのは `NotificationRegistry.owners()`(id に `.` が入っていても引ける) |
| 通知の絞り方 | `TailorFocusFilter.appContext` が `notificationFilterPredicate`(`SELF IN 選んだ id`)を OS に渡し、OS が選んでいないアプリの通知を出さない。アプリ側で予約は消さないので、集中モードが切れればそのまま出る |
| 通知を止めない時 | 「通知を止める」がオフなら述語は nil(OS は絞らない) |
| 1 つも選んでいない時 | 「通知を止める」がオンでアプリを 1 つも選んでいなければ、生成アプリの通知をすべて止める |
| 対象外の通知 | AlarmKit のアラーム・タイマーの通知・サーバーのプッシュは `filterCriteria` を付けないので絞らない |
| ウィジェットの目印 | iOS でもスナップショット(`WidgetSnapshot`)に `appId` を書く |
| ウィジェットの絞り方 | 一覧を絞っている(出すアプリを選んでいる)間、選んでいないアプリの数字の代わりに「集中モード中」(月のアイコン・アプリ名は出さない)を出す。ホーム画面(小・中・大)とロック画面(accessory 3 種)の両方。`appId` の無い古いスナップショットも絞り込み中は出さない |
| 通知だけ止める時 | 出すアプリを選ばず「通知を止める」だけの時は、ウィジェットはそのまま |
| 切り替えた時 | `perform()` で `WidgetCenter.reloadAllTimelines()` を呼ぶ |
| 文言 | ウィジェットの xcstrings に「集中モード中」(英語: Focus is on) |
| Mac | 入れていない |
| テスト | `MetamonTests/FocusFilterStateTests`・`MetamonWidgetTests/FocusPausedWidgetTests`・`MetamonServicesTests/FocusNotificationCriteriaTests` |

## Spotlight で生成アプリを探す

生成アプリの名前を Spotlight で探し、タップでそのアプリを開きます(#1996・`ios/Metamon/Features/Spotlight/`)。記録の中身はまだ入れません。

| 項目 | 決まり |
| --- | --- |
| 部品 | `SpotlightIndex` protocol(`index(apps:)`・`remove(appIds:)`・`removeAll()`)。実体は `CoreSpotlightIndex`(`CSSearchableIndex.default()`)。本物は `MetamonApp` だけが入れ、既定(ユニットテストなど)は何もしない `NoopSpotlightIndex` |
| 入れるもの | 生成アプリの名前と、DSL の `app.icon` の SF Symbols のサムネだけ。archived と未生成(version 1)は入れない。`GET /v1/apps` は自分のアプリだけ返すので、他の人が持ち主の共有アプリは入らない |
| ID | domain は `apps`、項目 id は `apps/<appId>`。記録を足す時用に domain `records.<appId>` を用意してあり、アプリを消す時はこの domain も消す |
| iOS 18 | `TailorAppEntity` を `IndexedEntity` にし、項目に `associateAppEntity` でつなぐ(開く Intent `OpenTailorAppIntent` と同じ Entity)。iOS 17 は `CSSearchableItem` だけ |
| 開く | `SpotlightOpenReceiver`(`CSSearchableItemActionType`)が appId を取り出し、`IntentHost.request(.open(appId:))`。Siri の「開く」と同じ道なので、サインイン・bootstrap の前に届いても準備の後に開く |
| 合わせる時 | `SpotlightAppIndexer.sync`: 一覧の取得(`appsList()`)が成功した時・起動時にすでに取っている一覧(`resumeLatestAppIfEmpty()`)・名前の変更・複製の後。Spotlight のためだけに一覧を取りには行かない。一覧に無い id は消し、名前か版が変わったものだけ入れ直す |
| 新しく作った時 | 生成(`send`)で新しいアプリができたら、一覧を取り直さずにその 1 件だけ入れる(`indexCreatedApp` → `SpotlightAppIndexer.upsert`・アイコンは DSL の `app.icon`)。生成の結果なので版に関係なく入れる。既存アプリの編集では入れ直さず、名前・版の変化は次の一覧の同期で拾う |
| 使わない一覧 | `restoreFromServer` は今のアプリの DSL だけなので使わない(共有されたアプリが次の同期でまた消えるため) |
| 他の端末で消した時 | 次に `appsList()` が走った時(変身一覧を開いた時など)に消える |
| サインアウトとの順番 | `SpotlightAppIndexer.generation` を `removeAll()` で 1 つ進める。`sync(_:since:)`・`upsert` は一覧の取得や索引の await の後に generation が変わっていたら入れず、`metamon.spotlight.indexedApps` も書かない。入れている最中に変わった時は、入った分を消し直す |
| 差分の取り方 | 索引の中身は読めないので、入れた id → 「版\|名前」を UserDefaults(`metamon.spotlight.indexedApps`)に覚える |
| 消す | アプリを消した時は `remove`、サインアウト・アカウント削除は `removeAll`(覚えたキーも `LocalWipe` で消える) |
| Mac | まだ入れていない(同じ API で後から足す) |
| テスト | `MetamonTests/SpotlightIndexTests`(偽物の索引を使う) |

## ショートカットで数字(stat)を読む

ショートカット.app と Siri から、生成アプリの数字(DSL の `stat`)を Tailor It を開かずに読みます(#1994・`ios/Metamon/Features/Intents/StatIntents.swift`)。数え方はウィジェットと同じです。

| 項目 | 決まり |
| --- | --- |
| Intent | `GetStatValueIntent`「数字を読む」: `ReturnsValue<String>`(stat と同じ書式の文字列)。Siri は「今月の支出は¥12,340です」。`GetStatNumberIntent`「数字を読む(数値)」: `ReturnsValue<Double>`(計算・比較用)。1 つの Intent は 1 つの型しか返せないので 2 つに分けた |
| 共通の設定 | どちらも `openAppWhenRun = false`・`authenticationPolicy = .requiresAuthentication`(端末がロック中なら、ロックを解いてから動く) |
| フレーズ | 「Tailor It で数字を読む」「Tailor It の数字を読む」(`TailorItIntents.swift` の `#if os(iOS)`) |
| Entity | `StatEntity`(アプリ × stat・表示は「家計簿 › 今月の支出」)と `StatQuery`(候補・名前照合・id の引き直し)。作りは `RecordTargetEntity` と同じで、DSL はキャッシュ優先、無いアプリだけ `GET /dsl` |
| stat の見分け方 | id は `<appId>/<statKey>`。`statKey` は中身から作る key(`WidgetSnapshotWriter.statKey`: entity\|aggregate\|field\|label)。id を書かない stat は読むたびに `Component.id` が変わるため。ウィジェットの前回値の引き継ぎと同じ key で、label を変えた stat は選び直しが要る |
| 候補 | ウィジェット・一覧のひと目の数字と同じ並び(`AppGlance.choices`) |
| 数え方 | 文字列はウィジェットと同じ `WidgetSnapshotWriter.statText`。数値は同じ絞り込み・同じ `Aggregator.aggregate` で、書式を掛ける前の値 |
| 記録の読み方 | `IntentHost.statRecords`: 表示中のアプリは画面の記録、ほかはローカル DB(`recordSync.cached`)。記録の API は呼ばない |
| 記録が端末に無い時 | 一覧のひと目の数字(`AppGlanceStore`)の値を返す(`statKey` で同じ stat か突き合わせる)。それも無ければ「まだ読み込んでいません」。「0」とは言い切らない |
| 暗号化した field の stat | 記録を読まずに「ロック中です。Tailor It を開いて確認してください」を返す(一覧のひと目の数字の 🔒 と同じ判定・`AppGlance.isEncrypted`)。数値版はこの文で失敗する |
| 数値で取れない時 | latest / earliest・文字の式・未読み込みは、数値版では分かる文で失敗する(文字列版は値を返す) |
| 消えた時 | アプリや stat を消した後は `StatQuery` が「見つからない数字」として返し、実行時に「その数字が見つかりませんでした。Tailor It でアプリを確認して、選び直してください」で失敗する |
| 審査用の版 | 文言を「記録帳」に言い換える(`APP_STORE_REVIEW`) |
| 文言 | 日本語・英語(`Localizable.xcstrings`・`AppShortcuts.xcstrings`) |
| Mac | 入れていない(Mac の target に `Metamon/Features` は入らない) |
| 変えないもの | `RunButtonIntent`・DSL の `exposeAsShortcut`・スキーマ・AI の決まり |
| テスト | `MetamonTests/StatIntentsTests`・`IntentEnglishCopyTests`・`ReviewWordingIntentsTests` |

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

## カレンダーの予定とリマインダーを読む(DeviceCalendarSource)

端末のカレンダーの予定とリマインダーを読む口だけを置いています(#1987・`ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Support/DeviceCalendar.swift`)。**読む部品・アクション・スキーマ・AI の決まりはまだありません**(決定待ち)。書き込み(#1810)も入れていません。記録を OS の画面からカレンダーに入れる `addToCalendar` は別の仕組みです([architecture.md §12.2](architecture.md#122-端末のカレンダーへの追加eventkituiaddtocalendar))。

| 項目 | 決まり |
| --- | --- |
| 口 | `DeviceCalendarSource`: `events(from:to:calendarIds:)`・`reminders(includeCompleted:dueBefore:listIds:)`・`calendars()`・`reminderLists()`(名前順)・`authorization(for:requesting:)`・`changes()`(`EKEventStoreChanged` を知らせるだけで中身は渡さない)。形は `HealthDataSource` と同じ(protocol + 実体 + 偽物) |
| 置き場 | `DeviceCalendar.source`。アプリ起動時に 1 回だけ差し替える(`METAMON_MOCK=1` は `FakeDeviceCalendar.sample`・`MetamonApp.swift`) |
| 実体 | `LiveDeviceCalendar`(`EKEventStore` を 1 つだけ持つ) |
| 許可 | 状態は自前の `DeviceCalendarAccess`(notDetermined / denied / restricted / writeOnly / fullAccess)。許可を求めるのは「未決定」の時だけで、iOS 17 のフルアクセス(`requestFullAccessToEvents` / `requestFullAccessToReminders`)。フルアクセスが無い時、読む関数は落ちずに `DeviceCalendarError.notAuthorized(状態)` を投げる(書き込みだけ許可の時は「追加だけ許可されています」と添える) |
| 説明文 | `Info.plist` の `NSCalendarsFullAccessUsageDescription`・`NSRemindersFullAccessUsageDescription`。英語は `Metamon/Resources/InfoPlist.xcstrings` |
| 読む範囲 | 予定は今日 0:00 の 31 日前から、31 日先の日の終わりまで。1 回 200 件まで。超えた分はエラーにせず切る(`DeviceCalendarLimits`・早い方を残す) |
| 並び | 予定は始まりの早い順(同じ時刻なら終日が先・次に題名順)。リマインダーは期限の早い順で、期限なしは最後 |
| 終日の予定 | 終わりを最終日の翌日 0:00 にそろえる(EventKit は 23:59:59 を返す) |
| リマインダーの絞り込み | `dueBefore` を渡すと期限がそれより前のものだけ(期限なしは除く)。済んだものは直近 31 日に済ませたものだけ |
| 題名・場所 | 予定とリマインダーの `description` / `debugDescription` に題名・場所を出さない。ログ・クラッシュ報告・サーバーにも出さない |
| 審査用の版 | 2 つの説明文を `strip-review-plist.sh` で外す(使う部品がまだ無いため)。説明文の無いビルドでは `LiveDeviceCalendar` は許可を求めずに `.restricted` を返す([review-build.md](review-build.md)) |
| Mac | 実体は作っていない(パッケージは macOS 向けにもコンパイルできる) |
| テスト | `MetamonRendererTests/DeviceCalendarTests` |

## 端末の中で文字を訳す(OnDeviceTranslator・iOS 18)

Apple の Translation framework で、文字を端末の中だけで訳す口を置いています(#1999・`ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Support/OnDeviceTranslator.swift`)。**アクション `translate`・スキーマ・AI の決まり・`supportedFeatures`・DeepL との切り替えはまだありません**(#1775 の決定待ち)。アプリの root に橋渡しの見えない View を付けるのも、使う側を作る時に回しています。

| 項目 | 決まり |
| --- | --- |
| 口 | `OnDeviceTranslator`: `availability(from:to:)`・`prepare(from:to:)`・`translate(_:from:to:)`。言語は `Locale.Language` |
| 状態 | `OnDeviceTranslationStatus`: `.installed`(機内モードでも訳せる)/ `.supported`(ダウンロードが要る・「翻訳する言語をダウンロードしてください」)/ `.unsupported`(「この端末ではこの言語を翻訳できません」) |
| 失敗 | `OnDeviceTranslationError`: `unsupported` / `notInstalled`(先に `prepare`)/ `hostMissing`(見えない View が付いていない)/ `failed`(ダウンロードの画面を閉じた・OS の翻訳が失敗した)。後ろ 2 つの文言は「翻訳できませんでした」 |
| 本物 | `SystemOnDeviceTranslator`(iOS 18 以上)。状態は `LanguageAvailability().status(from:to:)`。iOS 18〜25 は `TranslationSession` を `.translationTask` の中でしか作れないので、見えない View `.onDeviceTranslationHost(OnDeviceTranslationHost)` に載せて依頼を 1 件ずつ流す。iOS 26 以上で言語が入っていれば `TranslationSession(installedSource:target:)` で直接作る。ダウンロードの画面(`prepareTranslation`)は橋渡しから出す |
| 待ち続けない | 見えない View が 1 つも付いていなければ待たずに `hostMissing`。View が外れた時に待っていた依頼も失敗で返す。流してから 10 秒以内に `.translationTask` が動かなければ `failed` |
| 量の上限 | `OnDeviceTranslationLimit.clip`: 1 回 50 件・1 件 2000 文字・合計 10000 文字まで。合計に届いたら残りの件は落とすので、返す数は切った後の数 |
| 選び方 | `OnDeviceTranslators.make(mock:host:)`: mock なら偽物、iOS 18 以上で橋渡しがあれば本物、それ以外は `UnsupportedOnDeviceTranslator`(常に unsupported) |
| 偽物 | `FakeOnDeviceTranslator`(テストと `METAMON_MOCK=1` 用)。言語の組ごとに状態を決められ、ダウンロードの成功・失敗も切り替えられる。訳は `[<target>] 原文` |
| 置き場の理由 | 橋渡しに SwiftUI が要り、MetamonServices は SwiftUI を import しないため MetamonRenderer に置く |
| iOS 17・Mac | 本物は `os(iOS)` の時だけ作る。iOS 17 と macOS はビルドは通るが unsupported を返す |
| 文言 | 日本語・英語(MetamonRenderer の `Localizable.xcstrings`) |
| テスト | `MetamonRendererTests/OnDeviceTranslatorTests` |

## 連絡先から 1 人選ぶ口(ContactPick)

連絡先アプリの画面で 1 人を選んでもらい、その 1 人の名前・電話・メール・誕生日だけを受け取る口です(#2217・`ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Support/ContactPick.swift`)。**部品・アクションはまだありません**(決定待ち)。使っているのは、開発用メニューの試す口(`DebugContactPickSection`・#2079)だけです。

| 項目 | 決まり |
|---|---|
| 許可 | **要らない**。`CNContactPickerViewController` は、選んだ 1 人だけがアプリに渡る仕組みなので、許可のダイアログも `NSContactsUsageDescription` も要らない |
| やらないこと | `CNContactStore` を使わない(store で一覧を読むと許可のダイアログが出る。この口の肝) |
| 出す画面 | `ContactPickerSheet`(`.sheet` に入れる。`os(iOS)` だけ)。選ぶか取り消すと閉じる |
| 取り出す項目 | `ContactItem`(`name`・`phone`・`email`・`birthday`)。頼まれた項目だけを `ContactPick.read` で取り出す(既定は全部) |
| 返す値 | `PickedContact`(項目 → 文字)。無い項目・空の項目は入らない。`isEmpty` で空かを見る |
| 名前 | 地域の並び順の氏名。取れなければ「姓 名」を並べる |
| 電話・メール | 複数ある時は最初の 1 つ |
| 誕生日 | `YYYY-MM-DD`(date の field と同じ形)。年が無い誕生日は入れない(日付として表せない) |
| 見せない | 値はログ・クラッシュ報告・サーバーに出さない。`description` も項目名だけ(`PickedContact(items: [...])`) |
| 入れる先 | `ContactPick.prefill(_:fields:)` が、項目 → field の id の対応で値を作り、次に開く新規フォームへ `FormPrefill` で積む。値が無ければ何もしない(前に積んだ値を消さない)。`FormPrefill` は 3 秒で失効する |
| テスト | `MetamonRendererTests/ContactPickTests` |

- 決めること: 生成アプリの部品・アクションにするか、DSL の名前(決定待ち。#1775 の範囲との関係は推測)。
- 推測: Mac では `ContactPickerSheet` が無い(`os(iOS)` の中)ので、Mac で使うなら別の口が要る。

## 画面を消さない(ScreenAwake)

画面が勝手に暗くならないようにする口です(`ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Support/ScreenAwake.swift`)。使う所ごとに持ち、**持っている所が 1 つでもあれば消さない**作りです。直接 `isIdleTimerDisabled = false` にすると、他の使い手の分まで戻してしまうためです。

| 項目 | 決まり |
|---|---|
| 持つ | `ScreenAwake.shared.hold()` が `Hold` を返す。最初の 1 つで画面を消さない状態にする |
| 戻す | `Hold.release()`。最後の 1 つが戻した時だけ元に戻す。`release()` は何度呼んでもよく、2 回目は他の持ち主の分を減らさない |
| 効かせる先 | iOS は `UIApplication.isIdleTimerDisabled`。Mac はディスプレイのスリープを止める電源の assertion(`IOPMAssertion`)。いずれも `PlatformSystem.setIdleTimerDisabled` |
| テスト | `MetamonRendererTests/ScreenAwakeTests`(持ち主が残る間は戻さない・`release` を重ねても減らない) |

どの画面で効かせて、いつ戻すか(いまの使い手は 3 つ)。

| 使い手 | 効かせる時 | 戻す時 | コード |
|---|---|---|---|
| ドラレコ(`dashcam`) | 録画を始めた時(`start`) | 止めた時(`stop`) | `DashcamRecorder` |
| 道案内 | 案内が始まり、経路を計算できた後(`guiding` に入る時) | 案内をやめた時(`stop`)・着いた時(`arrived`) | `RouteGuideSession` |
| 録音ファイルの文字起こし | 文字にしている間 | 終わった・失敗・取り消した時(`defer`) | `FileImportLeaf` |

- 同じ使い手が 2 回持たないよう、持っている間は `Hold` を覚えて、`nil` の時だけ `hold()` します(ドラレコ・道案内)。
- `BackgroundGrace`(背面に回った時に、今の処理を区切りよく止めるまでの猶予)も同じファイルにあります。画面を消さないこととは別の物で、文字起こしが `begin` / `end` を一緒に呼びます。Mac は背面でも止められないので何もしません。
- 決めること: 新しく使う所を足す時は、必ず `ScreenAwake.shared.hold()` を使い、直接 `isIdleTimerDisabled` を触らない。

## 開いている PR

`epic/ios` にまだ入っていない、OS との連携のものです。入ったら上に章を足し、ここから消します。

- #2010 設定に「サインインの方法」を出し、今のアカウントに Apple をつなげる(#361)
- #2011 ヘルスケアから睡眠・心拍・距離・消費カロリー・運動・マインドフルネスを読む口(部品はまだ)(#1986・#2014 の上に積んである)
- #2017 アシスティブアクセスで開けるようにし、作る機能を隠して生成アプリだけ出す(#1997)
- #2033 iOS 18 のコントロール(コントロールセンター・ロック画面・アクションボタン)から生成アプリを開く(#1993)
- #2038 「ショートカットで自動にする」の案内(今ある Intent でのオートメーションの例)(#1995)
- #2042 Apple Music の最近聞いた曲を読む口(MusicKit・部品はまだ)(#1988)
- #2044 iOS 18 の記録の文字を長押しで「翻訳」(OS の翻訳の表示だけ・記録は変えない)(#2000)
- #2046 集中モード中かを読む口と許可の流れ(`INFocusStatusCenter`・式にはまだ出さない)(#1990)
