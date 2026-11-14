# iOS のウィジェット・コントロール・ライブアクティビティ

`ios/MetamonWidget/`(WidgetKit の拡張)の中身、本体とのデータの受け渡し(App Group)、更新のきっかけ、審査用の版での扱い。仕組みの経緯は [architecture.md の 12](architecture.md#12-ホーム画面ウィジェットwidgetkit実装済みissue-39) と [12.1](architecture.md#121-ライブアクティビティactivitykitissue-423)、Mac のウィジェットは [Mac の OS との連携](../mac/system-integration.md#ウィジェット)。

## 目次

1. [種類](#種類)
2. [ファイル](#ファイル)
3. [データの受け渡し(App Group)](#データの受け渡しapp-group)
4. [更新のきっかけ](#更新のきっかけ)
5. [コントロール](#コントロール)
6. [ライブアクティビティ](#ライブアクティビティ)
7. [審査用の版での扱い](#審査用の版での扱い)
8. [テスト・確かめ方](#テスト確かめ方)
9. [決めること](#決めること)

## 種類

拡張の入口は `MetamonWidgetBundle`(`ios/MetamonWidget/MetamonWidget.swift`)。拡張は Renderer・DSL を import しない(WidgetKit のメモリ上限が小さいため、本体で集計した文字列を描くだけ)。

| 種類 | 型 | 出る所 | 最低の OS |
|---|---|---|---|
| アプリを開くコントロール | `OpenAppControl` | コントロールセンター・ロック画面・アクションボタン | iOS 18 |
| 記録するコントロール | `RecordControl` | コントロールセンター・ロック画面・アクションボタン | iOS 18 |
| ライブアクティビティ | `TailorLiveActivity` | ロック画面のバナー・Dynamic Island | iOS 17(配布の最低) |
| 数字のウィジェット(ホーム画面) | `MetamonStatWidget`(`systemSmall` / `systemMedium` / `systemLarge`) | ホーム画面 | iOS 17 |
| 数字のウィジェット(ロック画面) | `MetamonStatWidget`(`accessoryCircular` / `accessoryInline` / `accessoryRectangular`) | ロック画面 | iOS 17 |

- 押すと本体が開く(数字のウィジェットとライブアクティビティは `widgetURL(metamon://widget)`)。

## ファイル

| ファイル | 役割 |
|---|---|
| `ios/MetamonWidget/Controls/ControlEntities.swift` | コントロールの設定で選ぶもの(`ControlAppEntity`・記録先)。一覧は `ControlCatalog` から読む |
| `ios/MetamonWidget/Controls/ControlIntentBridge.swift` | 拡張側の口(何もしない)。本体側は `ios/Metamon/Features/Intents/ControlIntentBridge.swift` |
| `ios/MetamonWidget/Controls/OpenAppControls.swift` | 2 つのコントロールと設定の Intent・文言(`ControlCopy`) |
| `ios/MetamonWidget/LiveActivityWidget.swift` | ライブアクティビティの見た目 |
| `ios/MetamonWidget/Localizable.xcstrings` | 拡張の文言(iOS と Mac のウィジェットで共有) |
| `ios/MetamonWidget/MetamonWidget.swift` | 入口(`MetamonWidgetBundle`)・`StatProvider`(タイムライン)・`MetamonStatWidget` |
| `ios/MetamonWidget/Shared/ControlCatalog.swift` | 本体 → コントロールの受け渡し(選べるアプリと記録先の一覧) |
| `ios/MetamonWidget/Shared/FocusFilterState.swift` | 集中モードのフィルタで選んだ内容 |
| `ios/MetamonWidget/Shared/TailorActivityAttributes.swift` | ライブアクティビティの受け渡し(`ContentState`) |
| `ios/MetamonWidget/Shared/WidgetSnapshot.swift` | 本体 → 数字のウィジェットの受け渡し(App Group の ID もここ) |
| `ios/MetamonWidget/Views/AccessoryViews.swift` | ロック画面の 3 種(単色で描かれるので太さで区別) |
| `ios/MetamonWidget/Views/LargeStatsView.swift` | `systemLarge`(最近の記録・直近 7 日の棒グラフ) |
| `ios/MetamonWidget/Views/StatViews.swift` | 小・中・空の時・「集中モード中」の表示 |

- `Shared/` のファイルは本体と拡張の両方にコンパイルする(パッケージにしない・`ios/project.yml`)。
- コントロールの Intent(`ios/Metamon/Features/Intents/ControlIntents.swift`)も拡張にコンパイルする。本体にも同じ型があるので、押すと OS が本体のプロセスで動かす。

## データの受け渡し(App Group)

App Group は `group.jp.kz2601.metamon`(全構成で同じ・本体と拡張の両方の entitlements)。`UserDefaults(suiteName:)` に JSON を 1 件ずつ置く。

| キー | 型 | 書く側 | 読む側 |
|---|---|---|---|
| `metamon.control.catalog` | `ControlCatalog` | 本体の `ControlIntentBridge.writeCatalog`(アプリの一覧が変わった時・`IntentHost.appsMayHaveChanged`) | コントロールの設定と見出し |
| `metamon.focusFilter` | `FocusFilterState` | 本体の `TailorFocusFilter`(集中モードの切り替え) | 本体の一覧・数字のウィジェット |
| `metamon.widget.snapshot` | `WidgetSnapshot` | 本体の `WidgetSnapshotWriter.write`(stat の先頭 3 個を集計した文字列) | 数字のウィジェット |

- ライブアクティビティは App Group を使わない。本体が ActivityKit に `TailorActivityAttributes.ContentState` を渡す。
- 端末の中のデータを消す時(`wipeLocalData`・アカウントの削除など)は、App Group を丸ごと消してタイムラインとコントロールを読み直し、ライブアクティビティも全部終える(`ios/Metamon/App/AppRuntime+Account.swift`)。
- stg 版と prod 版を同じ端末に入れると App Group を共有する(後から書いた方が出る・[architecture.md](architecture.md#stg-版と-prod-版を同じ端末に入れるbundle-id-の末尾既定-off))。

## 更新のきっかけ

| きっかけ | すること |
|---|---|
| 30 分ごと | `StatProvider.getTimeline` の `.after(30 分後)`。値は App Group にある前回のまま |
| アプリの一覧が変わった(2 秒まとめて) | `ControlCatalog` を書き、変わっていれば `ControlCenter.shared.reloadAllControls()` |
| 記録が変わった(ライブアクティビティを出している時) | `refreshLiveActivityStats()` → 値が変わった時だけ `update` |
| 集中モードの切り替え | `FocusFilterState` を書き、`reloadAllTimelines()` |
| 端末の中のデータを消す(`wipeLocalData`) | App Group を消し、`reloadAllTimelines()`・`ControlIntentBridge.reloadControls()`・ライブアクティビティを全部終える |
| 本体で DSL を当てた・記録が変わった(`AppRuntime`) | `WidgetSnapshotWriter.write` → 中身が変わった時だけ保存と `reloadAllTimelines()`(WidgetKit の更新の予算を節約) |

- 本体を開かずに記録が増えても(別の端末など)、ウィジェットとライブアクティビティは古いまま。push での更新はしていない。

## コントロール

| 項目 | 内容 |
|---|---|
| 押した時 | `OpenAppFromControlIntent` / `OpenRecordFromControlIntent` が本体のプロセスで動き、`IntentHost` 越しに選んだアプリ(と記録の入力画面)を開く |
| 設定 | `promptsForUserConfiguration()`。置いた時に、どのアプリ(記録先)かを選ばせる |
| 設定の一覧 | 設定の画面は拡張のプロセスで動き、本体のサーバー・DB に届かないので、本体が書いた `ControlCatalog` から読む |
| 見出し | 選んだアプリ名(データなので訳さない)。未選択なら「アプリを選ぶ」/「記録先を選ぶ」 |
| まだ無いもの | 生成アプリのボタンの動作を直接動かすコントロール(DSL の `exposeAsControl` が決まってから・#1775) |

## ライブアクティビティ

| 項目 | 内容 |
|---|---|
| 出す・消す | DSL の `timerDisplay.liveActivity: true`・action `startActivity` / `endActivity`(表は [architecture.md の 12.1](architecture.md#121-ライブアクティビティactivitykitissue-423)) |
| 組み立て | `ios/Metamon/App/LiveActivityContent.swift`(純粋関数) |
| 出し入れ | `ios/Metamon/App/AppRuntime+LiveActivity.swift`(操作は 1 本の列で順に流す) |
| 時間の表示 | 日付で渡して OS が描く。アプリが止まっていても進む |
| 設定でオフの時 | `startActivity` は「ロック画面への表示が設定でオフになっています」を出す。タイマーは黙って出さない |
| 更新 | 端末の中だけ(push しない・push の entitlement は無い) |

## 審査用の版での扱い

審査用の版(`APP_STORE_REVIEW`・[review-build.md](review-build.md))では「アプリを作る」を避けて「記録帳」と言う(言葉の決まりは [docs/store/wording.md](../store/wording.md))。

| 物 | 審査用の版 |
|---|---|
| App Group | 残す(PrivacyInfo の理由 `1C8F.1`) |
| コントロール | 残す。文言を「記録帳を開く」「記録帳を選ぶ」などに替える(`#if APP_STORE_REVIEW`) |
| ライブアクティビティ | 残す(権限のダイアログが無く、ユーザーが押した時だけ出る) |
| 数字のウィジェット | 残す。説明を「用意した記録帳の集計…」に替える(`#if APP_STORE_REVIEW`) |

- 注意(推測): `ios/project.yml` で `APP_STORE_REVIEW` を付けているのは `Metamon`・`MetamonMac`・`MetamonMacWidget` の Review 構成で、iOS の `MetamonWidget` には付いていないように読める。そうなら、iOS のウィジェットとコントロールの `#if APP_STORE_REVIEW` は審査用の版でも効かず、通常の版の文言が出る。Review でビルドして確かめる(決めること)。

## テスト・確かめ方

| 何を | どこで |
|---|---|
| コントロールの一覧の読み書き | `ios/MetamonWidgetTests/ControlCatalogTests.swift` |
| 集中モード中の表示 | `ios/MetamonWidgetTests/FocusPausedWidgetTests.swift` |
| 大きいウィジェットの描画 | `ios/MetamonWidgetTests/LargeWidgetRenderTests.swift`(`TEST_RUNNER_WIDGET_SHOTS_DIR` で PNG を出せる) |
| ライブアクティビティの中身の組み立て | `ios/MetamonTests/LiveActivityContentTests.swift`(ActivityKit 自体はユニットテストで動かない) |
| 実機・シミュレータでの見た目 | ホーム画面を長押し → + →「Tailor It」。ロック画面は長押し → カスタマイズ。ライブアクティビティは `METAMON_INITIAL_EXAMPLE=ramen_timer` でタイマーを開始 |

- シミュレータは署名なし(`CODE_SIGNING_ALLOWED=NO`)でも App Group の UserDefaults が動く。実機は本体と拡張の 2 つの App ID に App Groups を付ける。
- `MetamonWidgetTests` は PR の ios-ci では回らない(develop / main への push の時だけ)。

## 決めること

- iOS の `MetamonWidget` の Review 構成に `APP_STORE_REVIEW` を足すか(上の注意を確かめてから)。
- 本体を開かない間の更新(push・バックグラウンドの更新)をするか。今は 30 分ごとの読み直しで、値は前回のまま。
- 生成アプリのボタンを直接動かすコントロール(#1775 の後)。
