# 部品・アクションの OS ごとの対応表

どの部品・アクション・機能がどの OS で使えるかの表です。部品と機能の Mac の列は、Mac が AI に申告する値(`allowedComponents` / `disabledFeatures`)と同じです。アクションの Mac の列は、Mac のランタイムがそのアクションをどう扱うかです。

## 目次

- [表の作り方](#表の作り方)
- [値の意味](#値の意味)
- [表](#表)

## 表の作り方

**表は手で書き換えません。** 次のスクリプトがコードから作ります。

```sh
python3 ios/scripts/platform-table.py           # 下の表を作り直す
python3 ios/scripts/platform-table.py --check   # 表が今のコードと合っているかを見る(古ければ exit 1)
python3 ios/scripts/platform-table.py --print   # 表だけを出す
```

読むファイル:

| 列 | もと |
|---|---|
| 部品の一覧 | `ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/Component.swift` の `ComponentType` |
| アクションの一覧 | `ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/Action.swift` の `ActionType` |
| 部品・機能の Mac・審査用の版 | `ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/ClientCapabilities.swift` の `macUnavailableComponents` / `macDisabledFeatures` / `reviewExcludedComponents` / `reviewDisabledFeatures` / `supportedFeatures` |
| アクションの Mac | `ios/MetamonMac/App/Runtime/MacAppRuntime.swift` の `perform` の `switch` |

部品・機能を足した・Mac の申告を変えた時は、スクリプトを流して表も同じ PR で直します。申告と Renderer の登録の一致は `ios/MetamonMacTests/MacPlatformDeclarationTests.swift` が確かめます。

Android とデスクトップ(Windows・Linux)の列は空けてあります。それぞれの申告ができたら、スクリプトに読むファイルを足して埋めます。

## 値の意味

| 値 | 部品・機能 | アクション |
|---|---|---|
| 使える | AI が作ってよく、描ける | 動く |
| 代わりの表示 | — | 動かさずに一言で知らせる(文言を表に書く) |
| 使えない(枠) | AI に作らせない。古い DSL で来た時は「この Mac では使えません」の枠で受ける | — |
| 使えない(一言) | — | 知らない種類として「このボタンは Mac ではまだ使えません」を出す |
| 使えない(`disabledFeatures`) | 機能を AI に使わせない(例: `fieldHealth` は Mac にヘルスケアが無い) | — |
| 審査用の版「外す」 | iOS・Mac の審査用の版(`APP_STORE_REVIEW`)でバイナリから外す。中身は [`../ios/review-build.md`](../ios/review-build.md) | — |
| 審査用の版「送らない」 | 審査用の版は `supportedFeatures` に入れない(AI がその機能を使わない) | — |

部品ごとの Mac での見た目・操作の違い(代わりのやり方)は [`../mac/interactions.md`](../mac/interactions.md) と [`../mac/system-integration.md`](../mac/system-integration.md) にあります。

## 表

<!-- platform-table:begin -->
### 部品

| 部品 | iOS | Mac | Android | デスクトップ | 審査用の版 |
|---|---|---|---|---|---|
| `vstack` | 使える | 使える |  |  |  |
| `hstack` | 使える | 使える |  |  |  |
| `section` | 使える | 使える |  |  |  |
| `text` | 使える | 使える |  |  |  |
| `button` | 使える | 使える |  |  |  |
| `textField` | 使える | 使える |  |  |  |
| `numberField` | 使える | 使える |  |  |  |
| `datePicker` | 使える | 使える |  |  |  |
| `picker` | 使える | 使える |  |  |  |
| `toggle` | 使える | 使える |  |  |  |
| `photoField` | 使える | 使える |  |  |  |
| `image` | 使える | 使える |  |  |  |
| `list` | 使える | 使える |  |  |  |
| `stat` | 使える | 使える |  |  |  |
| `chart` | 使える | 使える |  |  |  |
| `voiceInput` | 使える | 使える |  |  |  |
| `spacer` | 使える | 使える |  |  |  |
| `divider` | 使える | 使える |  |  |  |
| `videoField` | 使える | 使える |  |  |  |
| `videoPlayer` | 使える | 使える |  |  |  |
| `audioRecorder` | 使える | 使える |  |  | 外す |
| `transcribeField` | 使える | 使える |  |  |  |
| `speakButton` | 使える | 使える |  |  |  |
| `drawingField` | 使える | 使える |  |  |  |
| `card` | 使える | 使える |  |  |  |
| `disclosure` | 使える | 使える |  |  |  |
| `carousel` | 使える | 使える |  |  |  |
| `grid` | 使える | 使える |  |  |  |
| `searchBar` | 使える | 使える |  |  |  |
| `calendar` | 使える | 使える |  |  |  |
| `shareButton` | 使える | 使える |  |  |  |
| `openUrlButton` | 使える | 使える |  |  |  |
| `mapView` | 使える | 使える |  |  |  |
| `locationButton` | 使える | 使える |  |  |  |
| `locationPicker` | 使える | 使える |  |  |  |
| `qrScanner` | 使える | 使える |  |  |  |
| `timerDisplay` | 使える | 使える |  |  |  |
| `webView` | 使える | 使える |  |  | 外す |
| `qrCode` | 使える | 使える |  |  |  |
| `slider` | 使える | 使える |  |  |  |
| `stepper` | 使える | 使える |  |  |  |
| `textEditor` | 使える | 使える |  |  |  |
| `secureField` | 使える | 使える |  |  |  |
| `timePicker` | 使える | 使える |  |  |  |
| `colorPicker` | 使える | 使える |  |  |  |
| `rating` | 使える | 使える |  |  |  |
| `label` | 使える | 使える |  |  |  |
| `icon` | 使える | 使える |  |  |  |
| `progressBar` | 使える | 使える |  |  |  |
| `gauge` | 使える | 使える |  |  |  |
| `badge` | 使える | 使える |  |  |  |
| `link` | 使える | 使える |  |  |  |
| `markdown` | 使える | 使える |  |  |  |
| `avatar` | 使える | 使える |  |  |  |
| `table` | 使える | 使える |  |  |  |
| `timeline` | 使える | 使える |  |  |  |
| `emptyState` | 使える | 使える |  |  |  |
| `settlement` | 使える | 使える |  |  |  |
| `aiButton` | 使える | 使える |  |  |  |
| `nearbyShare` | 使える | 使える |  |  | 外す |
| `locationTracker` | 使える | 使えない(枠) |  |  | 外す |
| `chatThread` | 使える | 使える |  |  | 外す |
| `aiChat` | 使える | 使える |  |  | 外す |
| `healthStat` | 使える | 使えない(枠) |  |  | 外す |
| `gridTable` | 使える | 使える |  |  |  |
| `randomPicker` | 使える | 使える |  |  |  |
| `bracket` | 使える | 使える |  |  |  |
| `fileImport` | 使える | 使える |  |  |  |
| `routeGuide` | 使える | 使える |  |  |  |
| `placeSearch` | 使える | 使える |  |  |  |
| `dashcam` | 使える | 使えない(枠) |  |  | 外す |
| `sensorValue` | 使える | 使えない(枠) |  |  | 外す |
| `musicPlayer` | 使える | 使える |  |  |  |
| `matchList` | 使える | 使える |  |  | 外す |
| `flashcards` | 使える | 使える |  |  |  |
| `feed` | 使える | 使える |  |  | 外す |
| `callButton` | 使える | 使える |  |  | 外す |

### アクション

| アクション | iOS | Mac | Android | デスクトップ |
|---|---|---|---|---|
| `navigate` | 使える | 使える |  |  |
| `back` | 使える | 使える |  |  |
| `saveRecord` | 使える | 使える |  |  |
| `deleteRecord` | 使える | 使える |  |  |
| `openVoice` | 使える | 使える |  |  |
| `openUrl` | 使える | 使える |  |  |
| `resetForm` | 使える | 使える |  |  |
| `scheduleNotification` | 使える | 使える |  |  |
| `cancelNotification` | 使える | 使える |  |  |
| `follow` | 使える | 代わりの表示(一言「フォローは Mac では使えません」) |  |  |
| `unfollow` | 使える | 代わりの表示(一言「フォローは Mac では使えません」) |  |  |
| `export` | 使える | 使える |  |  |
| `startActivity` | 使える | 代わりの表示(一言「ライブアクティビティは iPhone だけで表示されます」) |  |  |
| `endActivity` | 使える | 使える(何もしない) |  |  |
| `addToCalendar` | 使える | 使える |  |  |
| `lookupBook` | 使える | 使える |  |  |

### 機能(`supportedFeatures` / `disabledFeatures`)

| 機能 | iOS | Mac | Android | デスクトップ | 審査用の版 |
|---|---|---|---|---|---|
| `recordRetention` | 使える | 使える |  |  |  |
| `matching` | 使える | 使える |  |  | 送らない |
| `notification.when` | 使える | 使える |  |  |  |
| `singleEntity` | 使える | 使える |  |  |  |
| `dataRoles` | 使える | 使える |  |  | 送らない |
| `addressedMessages` | 使える | 使える |  |  | 送らない |
| `notification.near` | 使える | 使える |  |  |  |
| `aiButton.long` | 使える | 使える |  |  | 送らない |
| `aiButton.inputPhoto` | 使える | 使える |  |  | 送らない |
| `aiButton.saveTo` | 使える | 使える |  |  |  |
| `aiButton.inputFeeds` | 使える | 使える |  |  |  |
| `aiButton.outputs` | 使える | 使える |  |  |  |
| `action.lookupBook` | 使える | 使える |  |  |  |
| `text.highlight` | 使える | 使える |  |  |  |
| `feed.v2` | 使える | 使える |  |  |  |
| `expr.addMinutes` | 使える | 使える |  |  |  |
| `aiChat.inputEntity` | 使える | 使える |  |  |  |
| `component.bracket` | 使える | 使える |  |  |  |
| `progressBar.settingTarget` | 使える | 使える |  |  |  |
| `photoField.illustrate` | 使える | 使える |  |  | 送らない |
| `component.callButton` | 使える | 使える |  |  | 送らない |
| `field.encrypted` | 使える | 使える |  |  | 送らない |
| `openUrlButton.app` | 使える | 使える |  |  |  |
| `fieldHealth` | 使える | 使えない(`disabledFeatures`) |  |  | 外す |
| `cameraCapture` | 使える | 使える |  |  | 外す |
| `sharedData` | 使える | 使える |  |  | 外す |
<!-- platform-table:end -->
