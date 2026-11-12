# DSL 適合テスト(#352)

同じ DSL を iOS と Android(#357)で描いた時に「何が・どこに・どの値で・押すと何が起きるか」が同じかを、**画面を描かずに**比べる試験。
設計は #351 の決定文書「適合テストの骨組み」に従う。ピクセル・フォント・余白・色・アイコンの絵は比べない。

## 目次

1. [置き場](#置き場)
2. [case.json](#casejson)
3. [描画の木(expected.json)の形](#描画の木expectedjsonの形)
4. [期待値の更新(iOS だけ)](#期待値の更新ios-だけ)
5. [iOS の実装](#ios-の実装)
6. [網羅](#網羅)
7. [Android で合わせる時の注意](#android-で合わせる時の注意)

## 置き場

```
docs/dsl/conformance/
  cases/<名前>/case.json      DSL + レコード + 端末の条件 + 操作
  cases/<名前>/expected.json  期待する描画の木(全 OS 共通・iOS が書く)
  coverage.json               まだ case に出てこない部品の type
```

case の名前: `example-*`(`docs/dsl/examples` をそのまま描く)・`flow-*`(操作の流れ)・`edge-*`(境界値)・`layout-*`・`form-*`・`search-*`。

## case.json

```json
{
  "dsl": "examples/kakeibo.json",
  "records": { "expense": [ { "id": "e1", "data": { "amount": 1200, "date": "2026-11-10" }, "author": "me" } ] },
  "device": {
    "now": "2026-11-12T12:00:00+09:00",
    "timeZone": "Asia/Tokyo",
    "locale": "ja_JP",
    "userId": "me",
    "capabilities": { "components": ["text", "list"], "missingComponents": ["mapView"] }
  },
  "operations": [ { "tap": "add" }, { "input": { "field": "amount", "value": 980 } }, { "tapToolbar": "保存" } ]
}
```

| キー | 意味 |
|---|---|
| `dsl` | DSL の中身、または `docs/dsl` からの相対パスの文字列 |
| `records` | entity id → レコード(`id` / `data` / `author` など、appconfig の応答と同じ形) |
| `device.now` | 「今」(期間の filter・日付の既定値・今年なら年を省く書式)。省略時 `2026-11-12T12:00:00+09:00` |
| `device.timeZone` / `locale` | 省略時 `Asia/Tokyo` / `ja_JP`(`en*` なら英語 UI) |
| `device.userId` | サインイン中の uid(共有レコードの作者判定・hidden の項目) |
| `device.capabilities.components` | 描ける部品の type(省略 = 全部)。`missingComponents` はそこから引く略記 |
| `operations` | 上から順に実行する操作(下表) |

| 操作 | 意味 |
|---|---|
| `{"tap": "<部品 id>"}` | 今の画面の部品の `action` を押す。見えていない部品(`state` が `rendered` 以外)は押せない |
| `{"tapToolbar": "<label>"}` | 画面の toolbar の項目(保存バーを含む)を押す |
| `{"tapRow": {"component": "<list id>", "index": 0}}` | 一覧の行を押す(`onItemTap` に行の recordId を渡す)。`component` 省略時は最初の `onItemTap` を持つ部品 |
| `{"input": {"field": "<field id>", "value": <JSON>}}` | 今のフォームの値を変える |
| `{"search": {"field": "<searchBar.field>", "text": "…"}}` | 検索欄に入力する |
| `{"importCsv": {"component": "<fileImport id>", "text": "…"}}` | `fileImport` に CSV の中身を渡す(ファイルを選ぶ操作は含めない)。`component` 省略時は画面の最初の `fileImport`。`text` は 1 つの文字列、または 1 行 1 要素の配列(読みやすさのため) |
| `{"back": true}` | 戻る(スタックを 1 つ戻す・最下層のモーダルは閉じる) |

確認ダイアログ(`confirmMessage`)は「はい」で進める。

## 描画の木(expected.json)の形

比べ方は **JSON として同じか**(キーの順・空白・数値の `1200` と `1200.0` の違いは見ない)。

```json
{
  "screens": [ <画面>, ... ],
  "steps": [ { "op": <操作>, "top": <route>, "effects": [ ... ], "screen": <画面>, "error": "no visible target" } ]
}
```

- `screens`: DSL の `screens` の順に全部。タブの根はタブのスタック(`tab:<id>`)、他は `main` に**新規で**開いた状態(フォームは新規のドラフト)。
- `steps`: 操作ごとに、操作の後で一番上に見えている画面(`top`)とその木、起きたこと(`effects`)。対象が無い・見えていない時だけ `error`。

### 画面

| キー | 中身 |
|---|---|
| `id` / `title` | 画面の id と見出し |
| `route` | `{screen, stack, recordId}`。stack は `main` / `tab:<id>` / `sheet` / `full` |
| `layout` | `grid`(6 列 × 12 行に絶対配置)/ `form`(フォーム画面: area は並び順と幅だけ、段 = `band` で上から積む)/ `voice` |
| `nodes` | 部品(下記)。area のある部品は (行, 列, DSL の順) の順、その後に area の無い部品を DSL の順 |
| `toolbar` | ナビバーに出す項目 `[{label, action}]`(同じフォームを開く button と重なる項目・保存バーに移した項目・見えないボタンは除く) |
| `saveBar` | フォーム画面の下の保存バー `{label, action}`。他人の共有レコードは `"readOnly"`(「作った人だけが変更できます」) |
| `form` | フォーム画面だけ `{entity, recordId, readOnly, values}`。`values` は保存する形に正規化した束縛中の値 |

### 部品(node)

| キー | 中身 |
|---|---|
| `id` | DSL の id(無ければ `null`) |
| `type` | DSL に書いてある type の文字列そのまま(知らない type も) |
| `cell` | `{row, col, rowSpan, colSpan}`。**行・列とも 0 始まり**(`A1:C2` → row 0, col 0, rowSpan 2, colSpan 3)。Container 内グリッドの子はローカル座標 |
| `stacked` | area が無く、グリッドの下に全幅で積んだ部品は `true` |
| `band` | フォーム画面の段の番号(行が重なる部品は同じ段) |
| `state` | `rendered` / `hidden`(作者以外に隠す項目・新規フォームの削除ボタン・読み取りだけの保存ボタン)/ `needsForm`(入力がフォームの外)/ `unavailable`(この端末で描けない既知の type)/ `unknown`(知らない type) |
| `props` | `rendered` の時だけ。DSL に書いた意味のある値 + 解決した値(下表) |
| `actions` | `[{on: tap / rowTap / dateTap / threshold, action: {type, screen, presentation, prefillRef, entity, url, confirmMessage, format, notificationId}}]` |
| `localGrid` / `children` | Container の子。子が全員 area を持つ card / section / vstack は `localGrid: true` で子に `cell` |

`props` の解決した値(View と同じ関数で出す):

| type | 解決した値 |
|---|---|
| 入力(textField など 20 種) | `value`(ドラフトの値・JSON のまま)・`label`(toggle / datePicker / colorPicker / stepper / timePicker は field の名前で補う)・picker は `options` と `style` |
| computed / ref / refs の field を指す入力 | `input`(`computed` / `ref` / `refs`)・computed は書式後の `value`・ref は `options` |
| `stat` | `value`(書式後の文字: `¥1,520`・`2 件`・`—`)・`unavailable` |
| `list` | `rows: [{recordId, title, subtitle?, trailing?, checked?}]`(filter → sort → 検索の後)・空なら `emptyText` |
| `table` | `columns`(見出し)・`rows`(セルの文字) |
| `timeline` | `rows: [{recordId, date, title, subtitle?}]` |
| `chart` | `buckets: [{label, value}]` |
| `progressBar` | `ratio`(1 を超えてもよい)・`percent` |
| `gauge` | `value`・`unit` |
| `settlement` | `title`・`transfers: [{from, to, amount}]` |
| `bracket` | `rounds: [{title, matches: [{a, b, winner, bye?}]}]`(a / b は `{recordId, title}` か `null`・winner は `"a"` / `"b"` / `null`・1 回戦の不戦勝は `bye: true`)・`champion`(`{recordId, title}` か `null`)。組み合わせと勝ち上がりの規則は `../README.md` の bracket の補足 |
| `flashcards` | `cards: [{recordId, front, back, learned?}]`(開いた直後の山札: filter → 登録順。toggle を比べる filter は値の無いカードを「まだ」として扱う)・`progress`(`1 / 3`。空なら `0 / 0`) |
| `link` | `text`・`opens`(URL として開けるか) |
| `disclosure` | `title`・`expanded` |
| `grid` | `columns`(2〜4 に丸めた列数) |
| `searchBar` | `query` |
| `image` | `hasImage` |

DSL に書いた値で `props` に入れるもの: `title text label message placeholder url field entity aggregate format groupBy bucket kind mode metric period sensor task output address unit min max step target seconds limit expanded show`。`style`・`icon`・`color`・`size`・`height` などの見た目のキーは入れない。

### effects

| 形 | 意味 |
|---|---|
| `{"confirm": "<文言>"}` | 確認ダイアログを出した(「はい」で進めた) |
| `{"saved": {entity, recordId, data}}` | 保存した。新規の recordId は `new-1`, `new-2`, … |
| `{"rejected": {"missing": ["金額"]}}` | 必須が空で保存しなかった(項目の名前) |
| `{"rejected": "noForm" / "unsavedParent"}` | フォームが無い / 親が未保存で子を作れない |
| `{"deleted": {entity, recordId}}` | 削除した |
| `{"imported": {entity, matched, unused, failures, overLimit}}` | CSV を読んで列を当てた(`importCsv` の後)。`matched` は `[{column, field}]`(当てた列 → field の**名前**・列の順)、`unused` はどの field にも当たらない列の見出し、`failures` は読めなかった行 `[{line, field, kind, cell}]`(`line` は見出しを 1 行目とする行番号・`kind` は `invalid` / `required` / `tooLong`)、`overLimit` は上限(1 回 500 件・entity の合計 2000 件)で入れない行の数。確認ダイアログの文言は比べない(件数はここで比べる)。続けて足した記録が `saved` で 1 件ずつ出る |
| `{"request": <action>}` | OS の機能(URL・通知・書き出し・ライブアクティビティ・音声入力)を頼んだ。結果は比べない |

## 期待値の更新(iOS だけ)

- **expected.json を書き直すのは iOS だけ**。Android は書き換えずに合わせる。
- 更新: `CONFORMANCE_UPDATE=1` を付けて `ConformanceTests` を回す。`xcodebuild` では `TEST_RUNNER_` を前に付ける。

  ```
  cd ios && xcodegen generate
  TEST_RUNNER_CONFORMANCE_UPDATE=1 xcodebuild test -project Metamon.xcodeproj -scheme Metamon \
    -destination 'platform=iOS Simulator,name=iPhone 17' -only-testing:MetamonRendererTests/ConformanceTests
  ```

- 書き直した差分は PR で人がレビューする(描画の意味が変わった = 全 OS の受け入れ条件が変わった)。
- 新しい case を足す時: `cases/<名前>/case.json` だけ作って上の更新を回すと expected.json ができる。
- 失敗した時は `$.screens[0].nodes[2].props.value: expected "¥1,200" but got "¥1,300"` の形で、場所と値の差が出る。
- CI: `ios-ci` の `MetamonRendererTests` に入っている。`docs/dsl/conformance/**` と `docs/dsl/examples/**` の変更でも回る(macOS のジョブは増やしていない)。

## iOS の実装

- 木を作る: [`RenderTree.swift`](../../../ios/Packages/MetamonRenderer/Sources/MetamonRenderer/Conformance/RenderTree.swift)(`MetamonRenderer`)。SwiftUI の View は作らない。
  - 配置: `Screen.gridPlacement`・`Component.localGridPlacement`(`MetamonDSL`)・`FormFlowLayout.bands`(`GridScreenLayout` と同じ)。
  - 見える / 見えない: `FormDraft.hides`・`ButtonLeaf.isVisible`・`FormFieldOverride.applies`・`ComponentRegistry` の登録(`DSLView` と同じ分岐)。
  - 値: 各 Leaf の `static` な関数(`StatLeaf.resolve`・`ListLeaf.rows`・`ListRow.texts`・`TableLeaf.cellRows`・`TimelineLeaf.rows`・`ChartLeaf.buckets`・`ProgressBarLeaf.ratio`・`GaugeLeaf.value`・`SettlementLeaf.transfers`・`PickerLeaf.options`・`ComputedFieldRow.resolve`)。View の body もこれを呼ぶ。
  - フォーム: `ScreenView.makeDraft`。ナビバー: `DuplicateAddControl.toolbar`・`FormSaveBar.pinned`。
- テスト: [`ConformanceTests.swift`](../../../ios/Packages/MetamonRenderer/Tests/MetamonRendererTests/ConformanceTests.swift)。操作は `ConformanceRuntime` が実行する。遷移は `NavigationResolver`(アプリと同じ)、保存・削除はアプリ本体の `AppRuntime.perform` の規則を写したもの(`AppRuntime` はアプリのターゲットにあってパッケージのテストから呼べないため)。
- 時刻: `DSLClock.fixed` に `device.now` を入れる。日付の既定値(`"now"`)・期間の filter・日付の書式はこれを使う。

### iOS で木に出せていないもの(既知の隙間)

- 入力のラベルの補い方(`c.label` だけか、field の名前で補うか)は Leaf ごとの書き方を `RenderTree` の表に写している(共通の関数が無い)。
- OS の機能に頼る部品(mapView・healthStat・sensorValue・routeGuide・musicPlayer・dashcam・nearbyShare・locationTracker・chatThread・webView・calendar・gridTable・randomPicker・aiButton・timerDisplay・qrCode など)は、`state` と DSL に書いた値だけで、中で解決する値(地図のピン・歩数・センサーの値・タイマーの残り・カレンダーのマス)はまだ木に入れていない。
- 書き出し・通知・ライブアクティビティ・URL を開くは `request` として「頼んだこと」だけ。結果(ファイルの中身・通知の時刻)は比べない。
- 一覧の印(いいね #255)・保存待ちのチェックの表示・新しい行のハイライト・読み込み中(`loading`)の表示は入れていない(端末の一時状態)。
- 仲間探し(#507)の `matchList` の行と `list.item.interest` の「つながりたい」ボタンは、appconfig の `/interests`・`/matches`(誰が誰に送ったか)で決まり、レコードには無い。木には `matchList` の `state` と DSL の値(`entity`・`show`・`title`)だけを入れ、行・ボタンは入れない(`edge-matching`。`item.interest` の付いた list は普通の list と同じ行を出す)。
- appconfig が取得時に行う部品の置き換え(`substituted:<type>`)はサーバーの仕事なので、ここでは `unavailable` だけ。

## 網羅

`ConformanceTests.testCoverageListIsUpToDate` が「全 case に出てくる部品の type + `coverage.json` の `notYetCovered` = スキーマの全 type」を確かめる(足りない・余計なら赤)。

今: **74 / 77 の type が case に出てくる**(`notYetCovered` は `placeSearch`・`aiChat`・`callButton`。placeSearch の検索は `PlaceSearchTests`、aiChat は審査用の版から外す部品で、会話の並び・送る会話は `AIChatTests` で確かめる)。そのうち値まで解決して比べるもの:

| 区分 | type |
|---|---|
| Container(7) | vstack hstack section card disclosure carousel grid(子の順・ローカルグリッドの位置・見出し・列数・開閉) |
| 値まで解決する Leaf | text label badge markdown link emptyState button stat list table timeline chart progressBar gauge settlement flashcards searchBar image、入力 20 種(textField numberField picker toggle datePicker slider stepper textEditor secureField timePicker rating colorPicker photoField drawingField videoField audioRecorder transcribeField locationPicker locationButton qrScanner) |
| state と DSL の値だけの Leaf | 上の「既知の隙間」に挙げた OS の機能に頼る部品・matchList(仲間探しのつながりの状態に頼る)・icon avatar spacer divider voiceInput shareButton openUrlButton videoPlayer speakButton・feed(行は appconfig の `/v1/feeds` から取るので木に入れない・`edge-feed`)・fileImport(木には `state` と DSL の値だけ。CSV の列の当て方と足す記録は `importCsv` の操作と `imported` / `saved` の effects で比べる・`flow-file-import`) |

アクションは navigate(push / sheet / prefillRef なし)・back・saveRecord(必須の検証・新規・遷移)・deleteRecord(確認・閉じる)を操作で試している。CSV の取り込み(`fileImport`)は `importCsv` で試している。startActivity・endActivity・scheduleNotification は examples の木に `actions` として出る(実行は `request`)。

## Android で合わせる時の注意

- 同じ case.json を読み、DSL → 描画の木を**画面を描かずに**出す関数を作って、expected.json と JSON として比べる。expected.json は書き換えない。
- 違う時は、まず Android の解決の規則(書式・並べ替え・丸め)を iOS に合わせる。iOS が間違っていると思ったら Issue にして、iOS を直してから期待値を更新する。
- 書式は `ja_JP` が前提: 金額 `¥1,200`(負は `-¥500`)・件数 `2 件`・日付 `11/10(火)`(今年なら年を省く)・値が無い `—`。
- 時刻は `device.now` と `timeZone` で固定する。期間の filter(`thisWeek` は月曜始まり)も同じ時刻で評価する。
- 一覧の行の並びは filter → sort → 検索(title / subtitle の部分一致・大小無視)の順。
- `unknown` / `unavailable` の部品も node として残す(穴として位置を持つ)。
