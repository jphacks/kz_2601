# DSL v1(統一版・正典)

> **正典はこのフォルダ**: `dsl.schema.json`(JSON Schema draft 2020-12)+ `examples/{initial,kakeibo,training,walk}.json`(デモ3本+初期状態)+ `examples/gallery-*.json`(部品ギャラリー)。
> Go 型は `backend/pkg/dsl`(`schema.json` を埋め込み・`make schema-sync` で同期・CI で byte 一致を検証)、Swift 型は `ios/Packages/MetamonDSL/Sources/MetamonDSL/Model`(examples をそのままバンドルしデコードテスト)。
> **計算式(Expr・computed フィールド・stat.value・ref 型・settlement)は [`expressions.md`](expressions.md)**(#100)。実例 `examples/warikan.json` / `examples/bmi.json`。
> backend 案 `docs/backend/dsl-proposal.md` と iOS 案 `docs/ios/dsl-proposal.md` は**すり合わせ前の記録**として残す。矛盾したら本書が勝つ。

## 1. 統一の原則(確定・議論不要)

| 原則 | 内容 |
|---|---|
| 構造は backend 案 | Claude 構造化出力互換: **非再帰 2 段(Screen > Container > Leaf)**・`type` で閉じた enum・`additionalProperties:false`・数値/文字列制約はスキーマに書かず Go の `Validate()` で |
| 命名・エルゴノミクスは iOS 案 | camelCase・props はフラット(`props:{}` で包まない)・1 ノード 1 `type` |
| iOS は Patch を見ない | 編集ターンは ai→appconfig が RFC 6902 を適用。iOS は常に**フル DSL 封筒**を受け取る。`version` は単調増加 int(封筒側) |
| iOS は寛容・backend は厳格 | 未知 type はプレースホルダ、未知キー/enum 値は無視・fallback。backend は schema+意味検証で弾く |
| データは DSL 外 | DSL は定義のみ。レコードは appconfig の `records` API(Firestore)。集計は v1 では **iOS クライアント集計** |

## 2. 決定表(論点ごと)

| # | 論点 | backend 案 | iOS 案 | **決定** | 理由(1行) |
|---|---|---|---|---|---|
| 1 | ツリー構造 | 非再帰 2 段 | 再帰(`children` 無制限・`card`/`zstack`) | **非再帰 2 段**(container=`vstack/hstack/section`、Leaf は children 不可) | Claude 構造化出力が再帰スキーマ非対応。iOS レンダラーも有界で済む |
| 2 | `schemaVersion` | `"0.1"` 文字列 | `1` 整数 | **整数 `1`**(`const`) | 比較・インクリメントが楽。JSON Schema の `const` で閉じられる |
| 3 | `appId` / `version` の置き場 | DSL 外(API 封筒 `DSLEnvelope{appId,version,dsl}`) | DSL ルート内 | **封筒側**。DSL 本文には持たない | Claude に出力させない値を本文に混ぜない。OpenAPI の `DSLEnvelope` が既にそう定義済み |
| 4 | `screens` | 配列 `[{id,...}]` | object `{id: Screen}` | **配列** | 動的キー object は `additionalProperties:false` 必須の構造化出力と両立しない。Swift 側は `first{ $0.id == }` 1 行で済む |
| 5 | props の包み方 | フラット | フラット推奨 | **フラット** | 両案一致。短い・Claude が書きやすい |
| 6 | ナビゲーション | `initialScreen` + `navigate{screen,presentation:push/sheet/fullScreen}` | `navigation{type:stack/tab, tabs[]}` + `navigate/present/dismiss` | **アクションは backend 案 + タブは iOS 案を `app.tabs[]: [{screen,title,icon?}]` として採用**(ユーザー決定・2〜5 個・各タブが独立スタック)。`back` が pop と dismiss を兼ねる | 実機で好感触だった「タブで切り替わるアプリらしさ」を残す。閉じた配列なので構造化出力でも問題なし |
| 7 | データ束縛 | `screen.entity` のドラフト + 入力 Leaf の `field` | `state` + `{{式}}` + `bind` + `setState` | **backend 案**。`{{...}}` 式・`state`・`sequence` は v1 に無し | 式言語は AI の出力が揺れる最大要因。entity/field の id 参照なら Go が整合性検証できる |
| 8 | 保存/削除 | `saveRecord{entity,screen?}` / `deleteRecord{entity,confirmMessage?}` | `entity.create/update/delete` + `sequence` | **backend 案**。`list.onItemTap` の navigate で iOS が **recordId を暗黙に渡し**編集モード | 1 アクションで完結。フォーム→保存→戻るを AI が 1 行で書ける |
| 9 | 一覧 | `list{entity,item{title,subtitle,trailing,image},sort,filter,swipeToDelete}` | `list{source:式,item:テンプレ Node}` | **backend 案**(item は field id) | 行テンプレの再帰を避ける。見た目は iOS のデザインシステムで固定 |
| 10 | 集計 | `stat{aggregate,field,filter,format}` を iOS がクライアント集計 | backend が `summary` を返す | **iOS クライアント集計**(count/sum/avg/min/max・`thisMonth`/`today` は端末ローカル時刻) | 2 日間で集計 API を作らない。件数が増えたら `GET /records?aggregate=` に移す |
| 11 | `voiceInput` | Leaf | 画面種別 `aiVoice` | **Leaf コンポーネント**(初期状態 = 1 画面に `voiceInput` 1 つ) | 画面種別を増やさない。音声以外の部品(説明文など)と同居できる |
| 12 | 声への再入 | `app.voiceEntry: fullscreen/floating/none` + `openVoice` | ランタイムが FAB を常時表示 | **backend 案** | `fullscreen` = 初期状態の判定にも使える。AI が FAB を消す判断もできる |
| 13 | 日付入力 | `datePicker{field,includeTime}` あり | 無し(textField 代用) | **`datePicker` を採用** | 家計簿/トレーニングの P0 要件 |
| 14 | カメラ(P0) | `photoField{field,source}` → Firebase Storage URL | `camera{bind}` → base64 を state へ | **`photoField` + 値は `data:image/jpeg;base64,...`(iOS が ≤800px・q0.5 に縮小)または https URL** | Storage SDK/ルール設定を 2 日間のクリティカルパスから外す。縮小後 ≈100KB なら Firestore 1MB 制限内。URL 化は P1 の最適化 |
| 15 | ローカル通知(P0) | `app.notifications[]`(DSL ロード時に同期)+ `scheduleNotification`/`cancelNotification` | `native{feature:"notification.schedule", inSeconds}` | **backend 案**。識別子 `dsl.<id>` で置換・取消。`once` は time が過ぎていれば翌日 | 宣言的なので「毎朝 8 時」を AI が 1 要素で書ける。iOS 側の同期セマンティクスは iOS の裁量 |
| 16 | 位置情報 / 地図 | Field `location`("lat,lng")のみ・`map` 無し | `map{lat,lng,pins}` | **P1**。Field `location` はスキーマに残す。`map` Leaf は P1 で追加 | 00 決定事項どおり P1。追加は enum に 1 要素足すだけで互換 |
| 17 | 汎用ネイティブ | 無し | `native{feature,args,into}` | **採用しない** | 自由文字列の `feature` は AI が発明する。必要な機能は専用アクション(通知)/Leaf(カメラ)で閉じる |
| 18 | テーマ | `theme{primaryColor,mode}` | `theme{accent,colorScheme}` | **`primaryColor` / `mode`** | iOS 案が「柔軟」としていた項目。AI に意味が伝わる名前を優先 |
| 19 | 未知 type / キー | スキーマで拒否 | 無視・プレースホルダ | **両方**(backend 拒否・iOS は落ちない) | 二重防御。Day2 に部品を足しても古いビルドが死なない |
| 20 | 「元に戻して」 | `versions/{n}/restore` | `ai.prompt` で再生成 | **versions restore**(appconfig)。undo はスキーマに無し | 既存 API で済む。DSL に履歴概念を持ち込まない |
| 21 | ID 形式 | `^[a-z][a-z0-9_-]{0,39}$`(Go で検証) | 任意 | **backend 案** | Firestore パス安全性。iOS は気にしない |
| 23 | **画面レイアウト** | 上から積むスタック | 上から積むスタック(+`padding`) | **グリッド座標配置**(ユーザー決定)。Screen 直下の各コンポーネントに省略可能な `area:"A1:C3"`。詳細は §3.4 | 「全部品を長方形とみなし、A1〜C3 の正方形に入力欄を置く」ように AI/ユーザーが位置を調整できる DSL が理想 |
| 24 | Container 内のグリッド | — | — | **card / section / vstack の子にも `area` を許す**(#81・§3.4)。Container の矩形を 6 列のローカルグリッドとみなす。入れ子は 1 段まで | 「カードの中で 2 列に並べたい」に応える。`area` の意味を広げるだけなので既存 DSL と互換(v1 は Screen 直下のみだった) |
| 25 | **計算ロジック**(#100) | — | — | **小さな式ツリー `Expr`**(四則・ルート・比較・if・日付・agg)を `Field.computed` と `stat.value` に置ける。他 entity 参照は `Field.type:"ref"`+`refEntity`、精算は専用 Leaf `settlement`。変数・ループ等は持たない(割り切りは `expressions.md` §8) | 割り勘の「割る」が書けなかった。汎用言語にせず、よく使う演算だけ閉じた一覧で持つ |
| 22 | 編集ターンの形 | `$defs/PatchDocument{patch,summary}`(構造化出力) | — | **backend 案**。`$defs/PatchValue` で value を閉じる | iOS には無関係(常にフル DSL) |

## 3. 構造

```
AppDSL { schemaVersion:1, app:AppMeta, initialScreen, entities[], screens[] }
AppMeta { name, description?, icon?, theme?{primaryColor,mode}, voiceEntry: fullscreen|floating|none, notifications[]?, tabs[]?: {screen,title,icon?} }
Entity  { id, name, titleField?, scope?: private|shared|public, edit?: author|members, fields[]: Field{ id, name, type, required?, options?, defaultValue?, unit?, refEntity?, computed?: Expr, health?: "weight" } }
  予約値(field id の代わり): $author(共有 entity の投稿者)・$createdAt(作成日時)— §3.5
  Entity.marks?: [{ id, icon, label }](共有 entity の 1 人 1 回の印・いいね / 保存)・ListItem.mark? / 予約値 $marks.<id>(件数・自分が付けたか)— §3.5
Screen  { id, title, entity?, toolbar[]?: {label, icon?, action}, components[]: Container|Leaf }
Container { type: vstack|hstack|section, id?, area?, title?, spacing?, children[]: Leaf }
Leaf (44 種・全て id? と area? を持つ) / Action (9 種) / Notification { id, title, body, repeat: once|daily|weekly, time?, weekday? }
API 封筒(appconfig): DSLEnvelope { appId, version, dsl: AppDSL, source?, createdAt? }
```

### 3.1 コンポーネント(51 種 = Container 7 + Leaf 44)

全種に共通の任意キー: `id`(画面内一意)・`area`(§3.4 のグリッド矩形。Screen 直下と、card / section / vstack の子で有効)。用途・レイアウトの定石は `dsl.schema.json` 各定義の `description` / `x-usage` が正典(AI へのカタログはそこから生成。§4)。

| type | 必須 | 任意 | 要 `screen.entity` | P |
|---|---|---|---|---|
| `vstack` / `hstack` | `children` | `spacing` | | P0 |
| `section` | `children` | `title`, `spacing` | | P0 |
| `text` | `text` | `style`(title/headline/body/caption), `align` | | P0 |
| `button` | `label`, `action` | `icon`, `style`(primary/secondary/destructive/plain) | | P0 |
| `textField` | `field` | `label`, `placeholder`, `multiline` | ○ | P0 |
| `numberField` | `field` | `label`, `placeholder`, `unit` | ○ | P0 |
| `datePicker` | `field` | `label`, `includeTime` | ○ | P0 |
| `picker` | `field`(enum または ref) | `label`, `style`(menu/segmented/wheel) | ○ | P0 |
| `toggle` | `field`, `label` | | ○ | P0 |
| `photoField` | `field`(photo) | `label`, `source`(camera/library/both) | ○ | P0 |
| `image` | `field`(photo) | `height`(small/medium/large) | ○ | P0 |
| `videoField` | `field`(video) | `label`, `source`(camera/library/both), `maxDuration`(秒・既定 60) | ○ | #206 |
| `videoPlayer` | `field`(video) | `height`(small/medium/large) | ○ | #206 |
| `audioRecorder` | `field`(audio) | `label`, `maxDuration`(秒・既定 600・最大 7200) | ○ | #420 |
| `list` | `entity`, `item{title}` | `item.subtitle/trailing/icon/image`, `sort`, `filter`, `emptyText`, `onItemTap`, `swipeToDelete` | | P0 |
| `stat` | `label` + (`entity`+`aggregate` **または** `value`(Expr)) | `field`(count 以外必須), `by`(latest/earliest/distance/duration), `filter`, `format`(plain/currency/percent) | | P0 |
| `chart` | `entity`, `kind`(bar/line/pie), `groupBy`, `aggregate` | `title`, `field`, `bucket`(day/week/month), `filter` | | P1 |
| `voiceInput` | | `prompt`, `size`(compact/large) | | P0 |
| `spacer` | | `size`(small/medium/large/flexible) | | P0 |
| `divider` | | | | P0 |
| `card`(Container) | `children` | `title`, `spacing` | | P1 |
| `disclosure`(Container) | `children` | `title`, `expanded`, `spacing` | | P1 |
| `carousel`(Container) | `children`(1 つ以上) | `itemWidth`(small/medium/large), `spacing` | | P1 |
| `grid`(Container) | `children`(1 つ以上) | `columnCount`(2/3/4・`columns` は table の列定義なので別名), `spacing` | | P1 |
| `searchBar` | `field`(画面ローカルの検索キー) | `placeholder` | | P1 |
| `calendar` | `entity`, `dateField`(date/datetime) | `onDateTap`, `title` | | P1 |
| `gridTable` | `entity`, `columnField`(enum/string), `rowField`(integer/enum/string), `titleField` | `subtitleField`, `columnValues`, `rowValues`, `rowRange`{from,to}(integer・既定 1〜6・最大 24 行), `onItemTap`, `title` | string の軸は `columnValues`/`rowValues` 必須 | P1 |
| `shareButton` | `text`(`{{fieldId}}` 可) | `field`, `label`, `icon`, `shareTo`(any/line/slack/discord・既定 any) | `field`/`{{}}` 使用時のみ | P1 |
| `openUrlButton` | `label`, `url`(https か `line` `slack` `discord` `tel` `mailto` `maps` `comgooglemaps` スキーム) | `icon`, `style`(primary/secondary/plain) | | P1 |
| `mapView` | `entity`, `locationField`(location) | `titleField`, `zoom`(near/city/wide), `onItemTap`, `kind`(pins/route・#168), `dateField`(route のみ) | | P1 |
| `locationButton` | `field`(location) | `label` | ○ | P1 |
| `locationPicker` | `field`(location) | `label`, `nameField`(string・場所の名前) | ○ | P1 |
| `qrScanner` | `field`(string/text) | `label` | ○ | P1 |
| `timerDisplay` | `mode`(stopwatch/countdown) | `seconds`(countdown 必須 >0), `field`(integer/number), `label` | `field` 使用時のみ | P1 |
| `webView` | `url`(https のみ) | `title` | | P1 |

`list` は任意キー `searchField` を持ち、同じ画面の `searchBar.field` を指すと title/subtitle の部分一致で絞り込む(backend は対応する `searchBar` の存在を検証)。
| `slider` | `field`(integer/number/currency), `min`, `max` | `label`, `step`(省略で連続・integer は 1) | ○ | P1 |
| `stepper` | `field`(integer/number/currency) | `label`, `min`, `max`, `step`(既定 1) | ○ | P1 |
| `textEditor` | `field`(string/text) | `placeholder`, `minLines`(既定 4) | ○ | P1 |
| `secureField` | `field`(string/text) | `label`, `placeholder` | ○ | P1 |
| `timePicker` | `field`(string/text → `"HH:mm"` / datetime → RFC3339) | `label` | ○ | P1 |
| `colorPicker` | `field`(string/text → `"#RRGGBB"`) | `label` | ○ | P1 |
| `rating` | `field`(integer/number・1〜max) | `label`, `max`(既定 5・1〜10) | ○ | P1 |

入力系バッチの補足:
- **`segmented` は独立 type にしない**。`picker{style:"segmented"}` がセグメントコントロール(enum field・4 択以下なら既定で segmented)。同じ enum 束縛を 2 type に分けると AI の選択が揺れ、レジストリ/検証も二重になるため。
- **`timePicker` の保存形式**: `Field.type` に `time` は無い。string/text field なら `"HH:mm"`(`Notification.time` と同じ書式・日付を持たない)、datetime field なら今日の日付付き RFC3339。「毎朝の起床時刻」のように日付が要らないなら string を使う。
- 数値系(`slider`/`stepper`/`rating`)は integer/currency field では丸めて保存。`min >= max`・`step <= 0`・`rating.max` が 1〜10 の整数でない場合は backend `Validate()` がエラー。
- 実例: `examples/gallery-input.json`(entity 1 つに上記全部品を area 配置)。
| `label` | `text`, `icon`(SF Symbols) | `style`(title/headline/body/caption) | | P1 |
| `icon` | `name`(SF Symbols) | `size`(small/medium/large), `color`(primary/secondary/accent/red/green/orange/blue) | | P1 |
| `progressBar` | `value`(0〜1) **または** `entity`+`target`(>0) | `label`, `aggregate`(count/sum・既定 sum), `field`(count 以外必須), `filter` | | P1 |
| `gauge` | `value` **または** `entity`(+`aggregate`/`field`) | `label`, `min`(既定 0), `max`(既定 1・min より大), `filter` | | P1 |
| `badge` | `text` | `color`(primary/secondary/accent/red/green/orange/blue) | | P1 |
| `link` | `text`, `url`(http(s)) | | | P1 |
| `markdown` | `text`(インライン markdown) | | | P1 |
| `avatar` | | `image`(https/data URL), `field`(photo・要 screen.entity), `name`(イニシャル), `size` | △(`field` 時) | P1 |
| `table` | `entity`, `columns[{field,title?,aggregate?}]` | `sort`, `filter`, `limit`, `groupBy`(#261) | | P1 |
| `timeline` | `entity`, `dateField`(date/datetime), `titleField` | `subtitleField`, `filter`, `limit` | | P1 |
| `emptyState` | `title` | `icon`, `message` | | P1 |
| `settlement` | `entity`, `nameField`, `balanceField`(数値・通常は computed) | | | P1 |
| `randomPicker` | `entity`, `titleField` | `subtitleField`, `filter`, `label`(既定「ランダムに決める」), `title`, `onItemTap` | | #263 |
| `aiButton` | `task`(summarize\|classify\|rewrite\|extract\|suggest), `output`(string/text・classify は enum も可) | `label`, `input`(field)または `entity`+`limit`, `prompt`(`{{field}}` `{{input}}` 置換), `options`(classify 候補) | ✓ | P1 |
| `nearbyShare` | `label`, `entity` | | | P2 |
| `locationTracker` | `entity`, `locationField`(location) | `dateField`(datetime), `label`, `seconds`(自動停止・既定 3600) | | P3 |
| `healthStat` | `metric`(steps\|weight), `label` | `period`(today\|week・既定 today), `target`(steps の 1 日の目標) | | P2 |
| `chatThread` | `entity`, `textField`(string/text) | `photoField`(photo), `placeholder`, `emptyText` | | #260 |

`color` は `primary|secondary|accent|red|green|orange|blue` の閉じた enum(hex 文字列は不可・Claude 構造化出力互換。iOS は `Color.semantic(_:)` でシステム色に写像)。表示系の実例は `examples/gallery-display.json`(entity は家計簿と同じ `expense` なので iOS モックの seed がそのまま入る)。

### 3.2 アクション(9 種)

| type | フィールド | iOS の挙動 |
|---|---|---|
| `navigate` | `screen`, `presentation?`(push/sheet/fullScreen), `prefillRef?` | 遷移。`list.onItemTap` からは **recordId を暗黙に渡し**遷移先フォームが編集モードになる(だから遷移先は同じ entity のフォーム)。`prefillRef` 付きは**親から子を作る**: 今のレコードを子フォームの ref に入れて新規で開く(下の「親から子を作る」) |
| `back` | | スタックがあれば pop、モーダル最下層なら dismiss |
| `saveRecord` | `entity`, `screen?` | ドラフトを POST(新規)/PATCH(編集)→ レコード再取得 → `screen` へ、無ければ `back` |
| `deleteRecord` | `entity`, `confirmMessage?` | 現在の recordId を DELETE → back。list の長押し削除も同じ |
| `openVoice` | | 音声シートを開く |
| `openUrl` | `url` | Safari で開く |
| `resetForm` | | ドラフトを既定値に戻す |
| `scheduleNotification` | `notification` | ローカル通知を予約(`dsl.<id>`) |
| `cancelNotification` | `notificationId` | 予約を取消 |

`confirmMessage` はどのアクションにも付けられ、先に確認ダイアログを出す。

構造・ナビ・ネイティブ系の設計メモ(新規アクションは追加しない):

- `shareButton` / `openUrlButton` / `locationButton` / `qrScanner` / `timerDisplay` は **Leaf 内で完結**し(ShareLink / `openURL` / CoreLocation / AVFoundation / Timer)、新しい `Action` 種別(`share` / `requestLocation` 等)は足さない。`ActionType` に case を足すとホスト(`AppRuntime.run` の網羅 switch)の改修が必須になるため。URL は既存の `openUrl` アクションで足りる。
- **他アプリ連携(#37 最小版)**: `shareButton.shareTo` で送り先を選ぶ。`line` → `https://line.me/R/share?text=<urlencoded>` を `openURL`(LINE 未インストールなら Web 版に遷移・写真は送れないので文面のみ)。`slack` / `discord` → 公式に汎用の共有 URL スキームが無いので **OS 共有シート(ShareLink)を開き、文面をチャンネル投稿向けに整形**(`**` `#` 等の Markdown 記号を落としたプレーンテキスト + 先頭に 💬)。`any`(既定)は従来どおり。キー名が `target` でないのは `progressBar.target`(number)と JSON キーが衝突するため(`columnCount` と同じ理由)。**Slack/Discord への Incoming Webhook 直接投稿は後段(#37 中版: appconfig 側にユーザー設定として保存し `postWebhook` アクションから投稿)**で、この版には含めない。
- **`locationTracker`(#168)**: 「記録を開始 / 記録を停止」と記録中の状態(経過時間・距離・点の数)。**開始を押した間だけ**現在地を `entity` に 1 点 = 1 レコード(`locationField` に "lat,lng"、`dateField` に時刻)で足す。アプリを閉じても停止まで続き、`seconds`(既定 1 時間・300〜14400 に丸める)で自動停止。権限は When In Use だけ。点の entity はフォーム不要(lint の「add form が無い」から除外)で、他の required field には defaultValue が要る(validate)。見せ方は既存部品の拡張だけ: `mapView.kind: "route"` で点を `dateField` の時刻順に線で結び、`stat.aggregate: "distance"`(m / km)/ `"duration"`(m:ss / h:mm:ss)を `field` = location field、`by` = 日時 field で集計する。**10 分以上空いた点は別の記録**として線を切り、距離・時間にも足さない(朝と夕方の散歩を 1 本にしない)。Expr の `agg` には足していない(必要になったら)。例: `examples/walk_route.json`。
- **`nearbyShare`(#36)**: 近くの Tailor It 端末へ MultipeerConnectivity(Wi-Fi / Bluetooth 自動・インターネット不要)でレコードを送る。**送る範囲**: `screen.entity == entity` のフォーム画面なら入力中の 1 件(未保存でも今の値)、それ以外の画面ではその entity の新しい順 最大 50 件。**受け取り**: 受信側は前面にいて今のアプリが nearbyShare を使っている間だけ待ち受け(ローカルネットワーク許可ダイアログを無関係なアプリで出さないため)、招待を受けると「◯◯さんから「出席」3 件」の確認を出し、承認した時だけ接続する(暗号化必須)。取り込みは **同じ appId**、または **同じ entity id で保存フィールドの id+型が一致**する時だけ。未知のフィールドは捨てる。設計・実機手順は `docs/ios/architecture.md` §近距離共有。Leaf 内で完結し新しい Action は足さない(上と同じ理由)。
- `openUrlButton.url` は https のほか allowlist のカスタムスキーム(`line` `slack` `discord` `tel` `mailto` `maps` `comgooglemaps`)を許可する(backend `dsl.OpenURLSchemes` と iOS `ExternalURLPolicy` が同じ表・`line`/`slack`/`discord` は Info.plist `LSApplicationQueriesSchemes` にも登録)。`http://` や未知スキームは backend がエラー・iOS はプレースホルダ。
- `webView` は `WKWebView` の `UIViewRepresentable`(戻る/進む/再読込の小ツールバー + 読み込みバー)。https 以外は不可。WKWebView 自身がスクロールするので `GridScreenLayout.scrollsInside` には**入れない**(セル内で二重スクロールしない)。ページ内リンクが allowlist のスキーム(tel: 等)なら OS に渡し、それ以外の非 http(s) は無視。実例: `examples/gallery-web.json`(webView + LINE/Slack/Discord/共有シートの 4 ボタン + maps/tel/mailto/line リンク)。
- `calendar.onDateTap` は既存の `navigate` を使う。行タップは recordId を渡して編集、「追加」はタップした日を `dateField` の初期値として Renderer 内の `FormPrefill` に積み、次に開いた新規フォームが 1 回だけ消費する(`Action` に params を増やさない)。
- **親から子を作る(#256)**: コメント・回答・出欠・いいねのような「親のレコードに付く子のレコード」は、子の entity に親を指す `ref` field(例 `post_comments.post` → `posts`)を置き、`navigate` に `prefillRef: "<その ref field id>"` を付けて子のフォームを開く。**今のレコード** = `list` / `mapView` の `onItemTap` ならタップした行、`button` / `toolbar` ならその画面(`screen.entity`)のフォームのレコード。iOS は今のレコードの id を `FormPrefill` に積み、子のフォームを**新規**で開いて ref に 1 回だけ入れる(recordId は渡さない)。親が未保存(新規フォームの button / toolbar)の時は子を開かず「先に「<親の entity 名>」を保存してください」を出す(`AppRuntime.perform(.navigate)`)。実例: `examples/post_comments.json`(投稿のフォームの「コメントを書く」)。
  - 検証(`backend/pkg/dsl/prefill_ref.go`・エラー): `list` / `mapView` / `gridTable` の `onItemTap` と `calendar.onDateTap` が**別 entity のフォーム**へ `prefillRef` 無しで行く(タップした行の id で別 entity のフォームが編集モードで開き、該当レコードが無い。旧 d_quiz_game の問題一覧 → 回答フォーム)。メッセージに「同じ entity のフォームへ向ける / `prefillRef` に使える ref field」を入れ、AI の再生成にそのまま渡す。
  - `prefillRef` の条件: 遷移先は `screen.entity` のあるフォーム・field はその entity の保存される `ref`(`refs` / computed 不可)・`refEntity` が今のレコードの entity・遷移先フォームにその field の `picker` を残す(親が見えて選び直せる。prefillRef を知らない古い iOS でも保存できる)・`screen.entity` の無い画面の button / toolbar と、`calendar` / `gridTable` のタップ(自前で日付・マスを `FormPrefill` に積む)では使えない。
- **`audioRecorder` / Field `audio`(#420)**: マイクで録音して audio field に保存する入力(ボイスメモ・会議の録音)。録音 → 停止 → 再生・録り直し・削除。録音中は経過時間 / 上限と音量メーター。値は動画(#206)と同じ考え方で **端末内のファイル + レコードには `"audio:<id>|<秒>"` だけ**(AAC m4a・モノラル 64kbps を `Application Support/metamon-audio/<id>.m4a`・iCloud バックアップ対象外。Firebase Storage へのアップロードは動画と同じく未実装なので、他の端末では長さだけ出て再生できない)。レコード削除・録り直し・アプリ削除・アカウント削除でファイルも消す。`videoField` と同じ「field の型 + 入力部品」の形にしたのは、録音はレコードの 1 項目(メモと同じ 1 件に入る)で、一覧・ニアバイ共有・AI ボタンが型で扱いを決められるため(再生は録音部品と一覧の行が持つので `audioPlayer` は作らない)。一覧で再生するには `list.item.trailing` にその audio field(行の右端に再生ボタン + 長さ。同時に鳴るのは 1 本)。上限は `maxDuration`(秒・既定 600・1〜7200。`record(forDuration:)` が止める)。**イヤホンのボタン**: 部品が画面にある間 `MPRemoteCommandCenter` の 再生 / 一時停止 / 切り替え を受け、録音中なら停止・それ以外は開始(録音済みなら録り直し・再生中は再生を止めるだけ)。画面を離れたら外す。iOS がボタンを届けるのは最後に音を扱ったアプリだけなので、他のアプリで音楽を流している間は届かない(実機確認)。**音声入力を壊さない**: 録音・再生の間だけ AVAudioSession を借り(`AudioSessionLease`)、終わったら借りる前の category / mode / options に戻して非アクティブにする(途中で音声入力が設定を変えたら触らない)。音声入力側(`WhisperKitRecognizer`)は開始のたびに `.playAndRecord` を自分で設定する。審査用ビルド(#245)では部品ごと外す(`docs/ios/review-build.md`)。古い iOS では audio field は文字列扱い・部品はプレースホルダ(落ちない)。
- `gridTable`(#234)は時間割のマス目。列 = columnField の値(enum の options か columnValues)、行 = rowField の値(integer は rowRange、enum は options か rowValues)。マスのタップは `onItemTap`: 記録 1 件 = recordId 付きで編集、空き = 列・行の値を `FormPrefill` に積んで新規、複数 = メニュー(各記録の編集 + 追加)。1 マスに複数ある時は先頭 + 「+n」。列見出しが曜日なら今日の列を強調。列が画面幅に収まらなければ横スクロール、縦は `GridCell` の内側スクロール。配置計算は iOS `GridTableLayout`(MetamonDSL)。実例: `examples/timetable.json`。
- **`randomPicker`(#263)**: ボタンを押すたびに entity のレコード(`filter` で絞れる)から 1 件をランダムに選び、`titleField`(+ `subtitleField`)を大きく出す(約 0.7 秒のシャッフル演出・「視差効果を減らす」では演出なし)。候補が 2 件以上なら直前と同じ 1 件は続けて出さない。**乱数は Expr に入れない**: 計算式は描画のたびに評価されるので、乱数を入れると再描画・レコード再取得のたびに値が変わる。選んだ 1 件は部品の表示状態(SwiftUI `@State`)だけに持ち、保存しない(画面を離れると消える・再描画では変わらない)。選んだ 1 件のタップは `onItemTap` を recordId 付きで実行(list の行タップと同じ)。選び方は iOS `RandomPick`(MetamonDSL・乱数源を差し替えてテスト)。古い iOS では未知 type としてプレースホルダになる(落ちない)。実例: `examples/dinner.json`。
- `searchBar.field` は entity の field ではなく**画面ローカルのキー**(`ScreenLocals`)。`list.searchField` で参照し、レコードには保存しない。
- `mapView` は `latField`/`lngField` の 2 列ではなく既存の **`location` 型 field 1 つ**(`"lat,lng"`)を使う(`locationButton` の保存先と同じ表現・entity の field 数を増やさない)。既存 `map` Leaf は無かったので新設。
- **`locationPicker`(#212)**: 現在地以外の場所を選ぶ入力欄。シートで店名・地名・住所を検索(MapKit `MKLocalSearch`・API キー不要)/ 地図を動かして中央のピン / 現在地 から選ぶ。保存は `locationButton` と同じ `"lat,lng"`(値の形を増やさない=mapView・appconfig の検証はそのまま)。場所の名前は任意の `nameField`(string field)に入れる(ピン・現在地で名前が無い時は打った名前を消さない)。「今いる場所」だけなら `locationButton`。モック(`METAMON_MOCK=1`)の検索は固定 3 件。
- **`healthStat` / `Field.health`(#167)**: 端末のヘルスデータ。`healthStat` は **entity を持たない表示カード**で、歩数は iPhone の歩数計(CoreMotion・直近 7 日・capability 不要)、歩数計の無い端末(シミュレータ)はヘルスケアの歩数、体重はヘルスケアの最新値を読む。today は数値(steps は `target` で達成率バー)、week は 7 日のグラフ(steps は棒 + 目標線・weight は折れ線)。許可が無い・使えない端末は「ヘルスケアへのアクセスが許可されていません」等を表示。書き込みは部品ではなく **`Field.health: "weight"`**(number・unit kg): フォームで**新規**保存した時にヘルスケアの体重にも 1 件書く(編集保存では書かない・失敗してもレコードは保存済みでトーストだけ)。入力部品や `saveRecord` を増やさず「どの値をヘルスケアに出すか」をデータ側に 1 キーで持つため。実例: `examples/health.json`。設計は `docs/ios/architecture.md` §15.1。
- `aiButton`(#78)は **オンデバイス完結の AI 部品**(Apple Intelligence / Foundation Models・iOS 26+)。データは端末外に出ず、クラウド API も課金も無い。結果は output field のドラフトに入るだけで保存はユーザー。非対応端末・Apple Intelligence OFF・モデル準備中は「この端末では AI を使えません」と理由付きで無効化。`Action` の `runAI` 案ではなく Leaf にしたのは、(1) 生成 AI が 1 部品で書き切れる(button + action の 2 段より短く、入力・出力 field を同じ場所で検証できる)、(2) 実行中スピナー・不可時の無効表示が部品の見た目に属する、(3) `ActionType` を増やすとホストの網羅 switch 改修が要る、の 3 点。実例: `examples/diary_ai.json`(本文 → 要約・気分の分類)。
- **`table.groupBy`(#261)**: 同じ値のレコードを 1 行にまとめる集計表(「店ごとの平均点・件数でランキング」)。列は groupBy の field の列(グループ名・aggregate なし)+ `aggregate`(count / sum / avg / min / max)付きの列だけ。groupBy にできるのは string / enum / ref / boolean / integer / date。`sort.field` は集計列の field(同じ field の集計列が複数なら先頭)、集計列に無ければ groupBy の field = グループ名順。sort 省略 = 最初の集計列の降順、`limit` は上位 N グループ。値が空のレコードはどの行にも入れない。avg / min / max は数値が無ければ「—」、avg は小数 1 桁。iOS は `TableGrouping`(MetamonDSL)でクライアント集計(chart.groupBy と同じ)。新しい部品・Expr ではなく table の拡張にしたのは、(1) 表の見出し・行の描画をそのまま使える、(2) chart / stat と同じ aggregate 語彙で AI が書ける、(3) 古い iOS は `groupBy` と列の `aggregate` を無視して従来のレコード表として描く(落ちない・未知 type のプレースホルダにもならない)ため。マスタ entity の computed `agg` + table(人ごとの合計)も引き続き使える。実例: `examples/ranking.json`。
- **`chatThread`(#260)**: LINE のようなチャット。`entity` のレコードを 1 通 = 1 件の吹き出しで**古い順に上から下へ**積み(最新が一番下・開いた時も新着が来た時も一番下へスクロール)、**自分の分は右**(ブランド色)・**相手の分は左**(上に送信者名 = `$author`。同じ人が続けて送った 2 通目以降は省く)、下に送った時刻(`$createdAt`)。自分の判定は `Record.author` とサインイン中の uid の比較(#249 / #251 と同じ。author の無い未送信の自分の分と private の entity は全部自分)。**入力欄は部品の下端に固定**(複数行は 4 行まで伸びる・`photoField` があれば写真ボタン)で、送信 = `entity` に 1 件保存(ローカル DB に即反映して同期キューが送る・フォーム画面は作らない)。共有アプリへの初めての送信はフォームの保存と同じく表示名を聞く(#250)。他の人のメッセージは既存の records の購読(SSE `GET /events` → 再取得)で数秒以内に届く。器の中で自前でスクロールするので `GridScreenLayout.scrollsInside` には入れず、マイク FAB が入力欄の左に重なる時だけその分を空ける(`ChatInputInset`)。backend: `textField` は保存する string/text、`photoField` は photo、他の required field は defaultValue 必須(`Validate`)/ フォームが無くても「add form が無い」と言わない・`list` 扱い(`Lint`)/ private の entity は「誰にも見えない」警告。`list` の表示形式(例 `list.style:"chat"`)にしなかったのは、(1) 入力欄を持つ = 一覧 + フォームの 2 役で list の意味(表示だけ・追加はフォーム)を変えてしまう、(2) 下から積む・左右に振る・送信者名をまとめるは list の行テンプレ(title/subtitle/trailing)と共有できる部分が無い、(3) 部品が 1 つなら AI が「チャット = chatThread」と 1 語で選べ、iOS も 1 部品 = 1 ファイルで閉じる、の 3 点。iOS が知らない古いビルドでは未知の部品としてプレースホルダ(RELEASE は何も描かない)になり落ちない。実例: `examples/chat.json`。

### 3.3 フィールド型と値(appconfig `RecordData` と一致)

| Field.type | 値 |
|---|---|
| `string` / `text` | string |
| `integer` / `currency` | 整数 |
| `number` | 数値 |
| `boolean` | bool |
| `date` | `"YYYY-MM-DD"` |
| `datetime` | RFC3339 |
| `enum` | `options` の 1 つ |
| `photo` | `"data:image/jpeg;base64,..."`(v1)または https URL |
| `location` | `"lat,lng"`(P1) |
| `ref` | 参照先(`refEntity`)のレコード id 文字列。入力は picker |
| (computed) | 保存しない。`Field.computed` の Expr を iOS が評価(`expressions.md`) |

`defaultValue` は date/datetime で `"now"` を許可。


### 3.4 画面レイアウト = グリッド座標配置

| 項目 | 仕様 |
|---|---|
| グリッド | 各 Screen は暗黙のグリッドを持つ。**横 = A〜F の 6 列**(可視幅 ÷ 6)、**縦 = 1〜N 行**(1 行 = 可視高さ ÷ 12)。13 行目以降を使うと画面全体が縦スクロール。上限 48 行 |
| `area` | Screen 直下の Container/Leaf に省略可能。スプレッドシート記法 `"左上セル:右下セル"`(例 `A1:C3` = 3 列 × 3 行の正方形、`A1:F12` = 全画面)。スキーマ pattern `^[A-F][1-9][0-9]*:[A-F][1-9][0-9]*$` |
| 重なり禁止 | `area` を持つ兄弟同士でセルを共有したら **backend `Validate()` がエラー**。文言例: `screen "home" components[2] (id "recent") area A3:F10 overlaps screen "home" components[0] (id "summary") area A1:F4; areas of sibling components must not share cells`。範囲外(G 列・0 行・右下が左上より前・48 行超)も同様にエラーで列挙 → Claude 再生成に渡す |
| セル内スクロール | セル = 長方形の器。可変長(`list`・`section`・`vstack`/`hstack`・長い `text`)は**器の内側でスクロール**(`scrollBounceBehavior(.basedOnSize)` なので短ければ動かない)。その他(入力・ボタン・stat・chart・image)は器に収めて `clipped` |
| フォールバック(`area` 無し) | 従来どおり**上から積む**。同一画面で混在した場合は「`area` で使われた**最終行の次の行**から、全幅で上から順に積む」(決定的・LLM が配置を出し損ねても崩れない)。`area` が 1 つも無ければ 1 行目から積む |
| Container 内(#81) | 子は既定で積む。**card / section / vstack だけ**は子にも `area` を付けられ、Container の中身の矩形を**ローカルグリッド**(A1 = Container の左上・横 6 列 = 中身の幅 ÷ 6・行は Container 自身の `area` の行数まで)として配置する。ルール: 子の**全部に付けるか全部付けないか**(一部だけはエラー)/ 子同士の重なり禁止 / 行は Container の行数以内(Container が `area` 無しで積まれている時は 48 行まで)/ **入れ子は 1 段まで**(hstack / grid / carousel / disclosure の子と孫には付けない)。用途は「カードの中で 2 列に並べる」(stat 2 枚・ボタン 2 つ)だけ。iOS: 通常画面では中身の高さ ÷ 子が使う最終行 を 1 行とし `GridCell` を再利用して絶対配置。フォーム画面(`FormFlowLayout`)と `area` 無しで積まれた Container の中では横位置と幅だけ効かせ、高さは中身なり(#139 の方針) |
| タブとの関係 | グリッドは**タブ内の各 Screen の中身**の配置。直交する概念で、タブの有無に関係なく同じ |
| iOS 実装 | `GridScreenLayout`: `GeometryReader` で `cellW = (幅-24)/6`、`cellH = (高さ-24)/12` を算出し、`area` 子を `.frame(width: colSpan*cellW-8, height: rowSpan*cellH-8).offset(...)` で絶対配置。`voiceInput` 画面(初期状態)は非スクロール |

例(家計簿 home): 合計カード `A1:F2`・最近の支出(list・セル内スクロール) `A3:F10`・追加ボタン `D11:F12`。散歩メモ home: 今日 `A1:C2` / ぜんぶ `D1:F2` の 2 枚並び + 一覧 `A3:F12`。

### 3.5 entity.scope = レコードを誰と共有するか(#47)

**共有の依頼の扱い(#259 #215 #254)**: 語の表は `backend/internal/ai/share_intent.go` の 1 か所で、生成(作成・編集)のプロンプト、チャットのプロンプト、有料機能の判定(`paid.go`)、サーバーの点検が同じ表を読む。

- 「友達のスマホからも」「みんなで」「〜と共有」= 他の人と同じ記録を使う → **作成でも編集でも** その entity を `public`。編集は `{"op":"add","path":"/entities/<i>/scope","value":"public"}`(既にあれば replace)。編集が「できない」と返して全 entity が private のままなら、サーバーがレシピを添えて 1 回だけ作り直させる。
- 「家族だけ」「家族で共有」「家族みんなで」「サークルで」「チームで」「招待した人だけ」= 決まった人だけの共有 → **作成でも編集でも** その entity を `shared`(#388 で作成者とシェアリンクで参加した人だけになった・D-06)。public にはしない: 作成で shared が 1 つも無い / 編集で shared にならなかった時は、サーバーが `"value":"shared"` のレシピを添えて 1 回だけ作り直させる。新しく shared になった entity があれば summary に「○○は、作った人とリンクで招待した人だけが見られます(共有シートからリンクを送って招待)。」が入る(下の「誰でも見られます」は public になった entity だけ)。private のままなら「共有の設定は変えていないため、記録は自分だけのままです。」が入る。チャットは「できる」と答え、公開の確認はしない。
- 新しく public / shared になった entity があれば、サーバーが summary に「○○は、このアプリを開いた人なら誰でも見られます(みんなに公開)。」を足す(D-05 の文言・すでに言っていれば足さない)。
- **個人の情報(#253)**: public / shared の entity に名前・電話番号・住所・メールアドレス・誕生日・居場所らしい field(`backend/internal/ai/public_pii.go` が field の id・name・型で判定。ペットの名前・商品名は対象外)が新しく入ったら、サーバーが上の一文の後に「「名前」「電話番号」のような個人の情報も公開されます。ほかの人に見られてよいか確かめてください。」を足し、応答の `meta.publicPersonalData` に field ごとの警告を入れる(iOS が確認ダイアログを出せる)。チャットは依頼文と brief からこれを読み取り、ready の前に 1 回だけ question で確かめる(`confirmPublic`)。
- 「自分だけ」「非公開」「共有をやめて」は scope を remove。「LINE で送る」「共有ボタン」は scope ではなく shareButton。

| scope | 意味 | レコードの置き場(appconfig) | 書き込み権限 |
|---|---|---|---|
| `private`(省略時) | 本人だけ。家計簿・日記・体重など | `users/{uid}/apps/{appId}/entities/{e}/records` | 本人のみ(従来どおり) |
| `public` | 誰でも読める・ログイン済みなら誰でも投稿できる。掲示板・みんなのランキング・共有リスト | `apps_public/{appId}/entities/{e}/records`(**appId だけで一意**。同じ appId を開いた全員が同じデータを見る) | create は全員、update は投稿者本人のみ(`edit:"members"` なら参加者全員)、delete は投稿者本人のみ(他人のは 403) |
| `shared` | 同じアプリを開いた人と共有 | **v1 では `public` と同じ扱い**(参加者限定=招待制は後段) | public と同じ |

- public/shared のレコードには backend が **`author`(uid)と `authorName`(iOS が POST 時に任意送信・無ければ "匿名")を自動付与**し、`GET /records` のレコード最上位(`data` の外)で返す。ゆえに public/shared entity の field id に `author` / `authorName` は使えない(`Validate()` がエラー)。
- **`shared` の参加者(#254)**: 上の表の「v1 では public と同じ扱い」は #254 で変わった。`shared` の records は**作成者とシェアリンク / QR で参加した人だけ**が読み書きでき、それ以外は 403(`details.reason: not_member`)。置き場は public と同じ `apps_public/{appId}`。参加は `POST /shares/{token}/import`、退出・一覧は `/apps/{appId}/members`。家族・サークル・友達・チームは `shared`、不特定多数(掲示板・ランキング)は `public`。詳細と移行は [architecture.md §3.4](../backend/architecture.md)。
- public entity を 1 つでも含むアプリは、作成者の `PUT/PATCH /dsl`(と `POST /apps`・restore)で **`apps_public/{appId}` に DSL が複製**される。他ユーザーが同じ appId で `GET /v1/apps/{appId}/dsl` を開くと、自分の `users/{uid}/apps/{appId}` が無ければその DSL が**インストール**(自分の下に複製・version 1)される。以後の編集は各自のコピーに対して行われ、公開 DSL を上書きできるのは最初に公開したユーザーだけ。
- **`entity.edit`(#252)**: public/shared のレコードを誰が更新できるか。`author`(省略時)は投稿者本人だけ、`members` は同じ appId を開いた全員が PATCH できる。削除はどちらでも投稿者本人だけ。private では意味を持たない(書いても無視)。家事分担の完了・共有の買い物リスト・出欠・落とし物の解決・シフトの確定のように「みんなで 1 つのレコードを更新する」用途で使う。参加者を招待制で絞るのは D-06 で別途。
- iOS(#249): `Record.author` と自分の uid を比べ、変更できないレコードでは一覧のチェックを薄く表示して押すと理由を出し、長押し削除を出さず、フォームの保存ボタンを「このデータは作った人だけが変更できます」に置き換える。サーバーが 403 を返した時も同じ文言を出し、送信待ちに残さずサーバーの内容に戻す。
- 作り方: AI が「みんなで」「共有」「投稿」「ランキング」「掲示板」を聞いたら `scope:"public"`、個人の記録なら省略。iOS のデコーダは未知キーを無視するので、未対応ビルドでも落ちない。

#### レコードの予約値 `$author` / `$createdAt`(#250 #251)

field id の代わりに書ける、どのレコードにもある値。`$` 始まりなので field id(`^[a-z]…`)とぶつからない。正典は `dsl.schema.json` の `$defs/Entity` `x-recordValues`(AI カタログはそこから生成・Go の `dsl.RecordValues` とテストで突き合わせる)。実例: `examples/board.json`(落とし物掲示板)。

| 値 | 中身 | 書ける場所 | 使える entity |
|---|---|---|---|
| `$author` | 投稿者の表示名(`authorName`)。絞り込みでは投稿者の uid(`author`)と比べる | `list.item` の title / subtitle / trailing・`table.columns[].field`・`timeline` の titleField / subtitleField・部品の `filter.field` | public / shared だけ(private は全部自分の記録なので `Validate()` がエラー) |
| `$createdAt` | レコードを作った日時(datetime) | 上の表示箇所 + `timeline.dateField`・`list` / `table` の `sort.field` | 全部 |

- **自分の投稿だけ**: 部品の filter に `{"field":"$author","op":"eq","value":{"me":true}}`(他の人の分は `op:"neq"`)。`{"me":true}` は `$author` 専用で、`$author` の filter はこの形だけ。list / table / timeline / stat(件数)/ chart など部品の filter で使え、agg 式の中では使えない(iOS の評価器に uid を渡さないため。自分の件数は stat `aggregate:"count"` + 同じ filter)。iOS はサインイン中の uid に置き換えて `Record.author` と比べる。appconfig の `filterField=author`(eq / neq・値 = uid)と同じ意味。
- **投稿者名に何を出すか**: iOS の設定「投稿に表示する名前」に本人が入れた名前だけ。共有 entity に初めて投稿する時に聞き(「匿名のまま」も選べる)、未設定・空なら送らず backend が「匿名」を付ける。Apple の氏名・メールアドレス・端末名は送らない(本人が公開すると決めていない個人情報を他人に見せない)。名前を後で変えても、過去の投稿の表示名は投稿時のまま(backend は create 時だけ受け取る)。
- iOS は共有 entity のレコードを作る時、ローカル行にも自分の uid と表示名を入れる(未送信のうちから「自分の投稿」に出る・作者判定も同じ)。
- AI: 共有 entity に名前の手入力欄(member_name / person / staff_name 等)を作らず `$author` を表示する。ai-eval の `authorShown` / `nameInput` / `meFilter`(`m0disc_cases.json`)。

#### 自分だけの印 `marks`(#255)

いいね・保存・お気に入り・既読のように、**1 人 1 回のトグル**で共有レコードに付ける印。印はレコードの `data` ではなく appconfig が「誰が付けたか」を記録するので、押すたびに件数が増えたり、他人の印を付けたりできない。public / shared の entity だけ(private は自分の記録なので boolean field + `list.item.toggle` を使う)。実例: `examples/recipes.json`(みんなのレシピ)。

| 書く場所 | 形 | 意味 |
|---|---|---|
| `entity.marks` | `[{ "id": "like", "icon": "heart", "label": "いいね" }]`(3 個まで。icon は heart / star / bookmark / thumbsup / check / eye / flag) | その entity に付けられる印 |
| `list.item.mark` | `"like"` | 行の右端にアイコンと件数のトグル。他人の投稿にも付けられる(楽観的に反映・失敗したら戻す) |
| `$marks.<id>` | `list.item` の title / subtitle / trailing・`table.columns[].field`・`list` / `table` の `sort.field` | 件数(integer)。`sort` を desc で「いいねの多い順」 |
| 部品の `filter` | `{"field":"$marks.save","op":"eq","value":{"me":true}}` | 自分が付けたものだけ(`op:"neq"` = まだ付けていない = 未読)。stat の件数にも使える |

- **いいね数の集計(agg)との関係**: 印はレコードではないので、computed の `agg` からは数えられない(`agg` の filter に `$marks.<id>` を書くと `Validate()` がエラー)。件数は `$marks.<id>` で出す。いいね用の子 entity + ref + `agg count` は作らない(1 人で何回でも押せる・別の画面で親を選び直すフォームになる)。コメント・出欠のように中身のある記録は今まで通り子 entity。
- appconfig: `PUT` / `DELETE /v1/apps/{appId}/entities/{entityId}/records/{recordId}/marks/{markId}` で自分の印を付ける / 外す(どちらも冪等・`{count, mine}` を返す)。`GET /records` と `GET /records/{id}` はレコード最上位に `marks: {"<markId>": {"count": n, "mine": bool}}` を付ける。Firestore は `…/records/{id}/marks/{markId}`(`count`)と `…/marks/{markId}/by/{uid}`。レコードを消すと印も消える。
- iOS: 一覧の行のトグルは押した瞬間に件数と塗りを変え、サーバーの `{count, mine}` で確定する(失敗したら戻して理由を出す)。オフラインの間は付けられない(送信待ちの列には積まない)。自分の未送信の投稿にもまだ付けられない。
- AI: 「いいね」「お気に入り」「保存」「既読」は marks を使い、子 entity を作らない。ai-eval の `mark` / `markedByMe` / `markEntity`(`m0disc_cases.json`)。

### 3.6 ナビゲーション = 画面グラフの規則(#112 #114 #115)

画面を頂点、`navigate`(button / toolbar / `list.onItemTap` / `calendar.onDateTap` / `mapView.onItemTap`)と `saveRecord.screen` を辺とする有向グラフで考える。根はタブがあれば `app.tabs[].screen`、無ければ `initialScreen`(タブがある時に `initialScreen` がタブの根でなければ iOS は先頭タブを開く)。実装は `backend/pkg/dsl/nav.go`(`NavEdges` / `Reachable` / `Unreachable` / `HasNav`)。

| 規則 | 扱い | 理由 |
|---|---|---|
| `navigate.screen` / `saveRecord.screen` / `app.tabs[].screen` / `initialScreen` が存在する画面 id | **エラー**(`Validate`。メッセージに使える画面 id の一覧と直し方を入れ、AI の再生成プロンプトにそのまま渡す) | 押すと何も起きない・描画できない |
| `app.tabs` は 0 個(省略)か 2〜5 個 | **エラー** | タブ 1 個は TabView として意味が無い・6 個以上はタブバーに入らない |
| どの根からも辿れない画面 | 警告(`Lint`)。フォームは追加ボタンや `list.onItemTap` から辿れれば可(entity のフォームが 1 つも辿れない時は従来の「form screen is not reachable」1 件だけ) | 表示はできるので保存済み DSL を壊さない。声で直せばよい |
| タブがあるのに `initialScreen` がタブの根でない | 警告 | 起動画面が意図と違う |
| `fullScreen` で開く画面に戻る手段(`back` か、遷移先指定の無い `saveRecord`)が無い | 警告 | iOS は fullScreenCover に閉じるボタンを自動で付けないので閉じ込められる。push は OS の戻る、sheet は下スワイプで戻れるので対象外 |

- 警告は `Meta.lint` に載るだけで再生成は起こさない(`AI_LINT_RETRY=1` の時だけ 1 回再生成)。
- AI 側の基準(`internal/ai/prompt.go`): 役割の違う一覧・集計・設定はタブ(2〜4 個)、入力はフォーム画面を sheet、詳細は `list.onItemTap` で push、1 画面の直下は 8 部品まで、孤立画面を作らない。navigate はどこからどこへ飛んでもよい前提(タブの根へ飛んだらタブ切替は iOS 側 #113)。
- 遷移を変える編集は JSON Patch のレシピで出す: 「B から C にも」= B の components に navigate C の button を add、「タブを足す」= `/screens/-` + `/app/tabs/-`(タブが無ければ `/app/tabs` を 2 個で add)、「タブを消す」= `/app/tabs/i` を remove(残り 1 個なら `/app/tabs` ごと remove し、辿れなくなる画面も remove)、「順番」= `/app/tabs` の move。eval は `cases.json` の `group: nav`(`expect` の `patch` / `unreachable` / `nav` / `tabs` / `tabOrder` / `tabHas`)。

## 4. コンポーネント追加手順(スキーマが唯一の正典・3 ステップ)

AI に渡す部品カタログは手書きではなく **`dsl.schema.json` から自動生成**される(`backend/internal/ai/catalog.go` の `BuildCatalog`)。検証(Go)・描画(iOS)・AI(プロンプト)の 3 者はすべてスキーマから導出され、抜けはテストで赤になる。

| # | やること | 揃うもの |
|---|---|---|
| 1 | `docs/dsl/dsl.schema.json` に定義を足す: `$defs` に object(`type` は `const`)、`Leaf.anyOf` と `PatchValue` に `$ref` 追加。**`description`(何に使うか・いつ選ぶか 1〜2 文)と `x-usage`(レイアウトの定石: area の例・組み合わせ)を必ず書く**。Container の種別は `properties.type.x-enumDescriptions`、特定 type だけのキーは `x-for: {"section":"optional"}`。Action を足す時は `Action.properties.type.enum` + `x-enumDescriptions`、専用キーに `x-for: {"navigate":"required"}` | AI: `make schema-sync` 後にカタログへ自動掲載。`backend/pkg/dsl`: `dsl.go` の `ComponentType` 定数と `validate.go` の per-type 検証を追加、`examples/*.json` に使用例 |
| 2 | iOS: `MetamonDSL` の `ComponentType` に case 1 つ + `MetamonRenderer` に Leaf の View ファイル 1 つ + `ComponentRegistry.standard` に 1 行 | 描画 |
| 3 | `cd backend && make test`(= schema-check + `go test ./...`)と `cd ios && xcodebuild test` が**両方緑** | 検証・AI・描画の一致 |

ステップ 3 で赤になる条件(= 追記漏れの検出):

- backend `internal/ai/catalog_test.go`: スキーマの全 Container/Leaf type と全 Action type がカタログ文に出現しない / `description`・`x-usage`・`x-enumDescriptions` が空 / Go の `ComponentType` 定数とスキーマの type 集合が不一致。
- backend `pkg/dsl`: 埋め込み `schema.json` が docs と byte 不一致(`make schema-sync` 忘れ)/ `examples` が round-trip しない。
- iOS `ComponentRegistryTests`: スキーマの type が `ComponentRegistry.standard` / `ComponentType` に無い(またはその逆)。

`description` / `x-*` キーは JSON Schema の検証では無視され、iOS のデコーダ(`MetamonDSL` は examples JSON をデコードするだけでスキーマ本文は読まない)にも影響しない。生成用スキーマ(`ai.GenerationSchema()`)では `description` と `x-*` を落としてトークンを節約し、意味はカタログ側だけに載せる。

要求された部品がカタログに無い時(カレンダー・地図など)は、プロンプトの規則で**最も近い既存部品で代替し、存在しない type は出力しない**。代替したことは編集ターンなら `summary`、新規作成なら `app.description` 末尾の「○○は△△で代替しました。」として残り、backend の `summarize()` が読み上げ文に含める。
