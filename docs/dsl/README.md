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
AppDSL { schemaVersion:1, app:AppMeta, initialScreen, entities[], screens[], serverRules[]? }(serverRules は §3.12)
AppMeta { name, description?, icon?, theme?{primaryColor,mode}, voiceEntry: fullscreen|floating|none, notifications[]?, tabs[]?: {screen,title,icon?} }
Entity  { id, name, titleField?, scope?: private|shared|public, edit?: author|members, fields[]: Field{ id, name, type, required?, options?, defaultValue?, unit?, refEntity?, computed?: Expr, health?: "weight" } }
  予約値(field id の代わり): $author(共有 entity の投稿者)・$createdAt(作成日時)— §3.5
  Entity.marks?: [{ id, icon, label }](共有 entity の 1 人 1 回の印・いいね / 保存)・ListItem.mark? / 予約値 $marks.<id>(件数・自分が付けたか)— §3.5
  Entity.retentionDays?: 1〜3650(記録の保存期間。期限を過ぎた記録を端末が自動で消す)— §3.7
  Entity.single?: true(設定値の置き場。レコードは常に 1 件・式 {"setting": "<fieldId>"} で読む)— §3.11
  Entity.unique?: [fieldId|$author](1〜3・一意)/ deleteBy?: author|admins|authorOrAdmins・Field.editableBy?: "admins"・Field.type "member"・button.visibleTo?: "admins" — §3.13
  Entity.addressedTo?: fieldId(宛先の member / members field。作った人と宛先の人だけが読む)・Field.type "members" — §3.14
Screen  { id, title, entity?, toolbar[]?: {label, icon?, action}, components[]: Container|Leaf }
Container { type: vstack|hstack|section|card|disclosure|carousel|grid, id?, area?, title?, spacing?, children[]: Leaf }
Leaf (68 種・全て id? と area? を持つ) / Action (16 種) / Notification { id, title, body, repeat: once|daily|weekly|monthly|yearly, time?, weekday?, entity?, field?, offsetMinutes?, titleField?, enabledField?, when?, alarm?, near?{entity, locationField, radiusM?}, jitterMinutes? }(monthly / yearly はレコードの日付 field から・expressions.md §7 の 9)
API 封筒(appconfig): DSLEnvelope { appId, version, dsl: AppDSL, source?, createdAt? }
```

### 3.1 コンポーネント(75 種 = Container 7 + Leaf 68)

全種に共通の任意キー: `id`(画面内一意)・`area`(§3.4 のグリッド矩形。Screen 直下と、card / section / vstack の子で有効)。用途・レイアウトの定石は `dsl.schema.json` 各定義の `description` / `x-usage` が正典(AI へのカタログはそこから生成。§4)。

| type | 必須 | 任意 | 要 `screen.entity` | P |
|---|---|---|---|---|
| `vstack` / `hstack` | `children` | `spacing` | | P0 |
| `section` | `children` | `title`, `spacing` | | P0 |
| `text` | `text`(screen.entity の画面では `{{fieldId}}` 可) | `style`(title/headline/body/caption), `align`, `highlight`({words} か {field}・`style` marker/bold) | | P0 / 下の「文中の語に印」 |
| `button` | `label`, `action` | `icon`, `style`(primary/secondary/destructive/plain) | | P0 |
| `textField` | `field` | `label`, `placeholder`, `multiline` | ○ | P0 |
| `numberField` | `field` | `label`, `placeholder`, `unit` | ○ | P0 |
| `datePicker` | `field` | `label`, `includeTime` | ○ | P0 |
| `picker` | `field`(enum または ref) | `label`, `style`(menu/segmented/wheel) | ○ | P0 |
| `toggle` | `field`, `label` | | ○ | P0 |
| `photoField` | `field`(photo) | `label`, `source`(camera/library/both), `analyze`([{kind, into, options?, joint?}]・#418) | ○ | P0 |
| `image` | `field`(photo) | `height`(small/medium/large) | ○ | P0 |
| `videoField` | `field`(video) | `label`, `source`(camera/library/both), `maxDuration`(秒・既定 60) | ○ | #206 |
| `videoPlayer` | `field`(video) | `height`(small/medium/large) | ○ | #206 |
| `audioRecorder` | `field`(audio) | `label`, `maxDuration`(秒・既定 600・最大 7200) | ○ | #420 |
| `transcribeField` | `field`(string / text) | `label`, `placeholder` | ○ | 文字起こし |
| `speakButton` | `text` / `field` / `value` のどれか 1 つ | `label` | `field`・`{{fieldId}}` の時 ○ | 読み上げ |
| `drawingField` | `field`(photo) | `label` | ○ | 手書き |
| `list` | `entity`, `item{title}` | `item.subtitle/trailing/icon/image`, `item.interest`(match "profile" の一覧・#507), `sort`, `filter`, `emptyText`, `onItemTap`, `swipeToDelete` | | P0 |
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
| `locationButton` | `field`(location) | `label`, `addressField`(string / text・住所) | ○ | P1 |
| `locationPicker` | `field`(location) | `label`, `nameField`(string・場所の名前) | ○ | P1 |
| `routeGuide` | 目的地: `locationField`(location)か `address` のどちらか | `entity`(候補から選ぶ)・`titleField`・`mode`(automobile\|walking)・`title`・`label` | | #411 |
| `placeSearch` | `nameField` / `locationField` / `addressField` の 1 つ以上 | `label`・`query`・`category`(cafe 等 18 種)・`near`(current)・`pins` | ○ | |
| `qrScanner` | `field`(string/text) | `label` | ○ | P1 |
| `qrCode` | `text` / `field` / `value`(Expr) のどれか 1 つ | `label`(QR の下の説明), `title` | | #422 |
| `timerDisplay` | `mode`(stopwatch/countdown) | `seconds`(countdown 必須 >0), `field`(integer/number), `label`, `liveActivity`(bool・ロック画面 / Dynamic Island にも出す #423) | `field` 使用時のみ | P1 |
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
| `fileImport` | `entity`, `accept`(csv / text / pdf / audio の配列) | `label`(既定「ファイルから取り込む」), `field`(text / pdf の本文・audio の文字起こしの先・その時は必須), `map`([{column, field}]・csv の列の当て方) | | 下の「ファイルの取り込み」 |
| `flashcards` | `entity`, `front`(field), `back`(field) | `filter`(list と同じ形), `toggle`(boolean field・覚えた / まだ) | | #264 |
| `feed` | `provider`(weather\|holidays\|rss)。weather は `region`(府県予報区のコード)、rss は `feedId`(許可したフィードの id)が必須 | `title`, `limit`(1〜20・既定 5), `emptyText` | | #348 |
| `aiButton` | `task`(summarize\|classify\|rewrite\|extract\|suggest\|generate), `output`(string/text・classify は enum も可。generate だけ任意) | `label`, `input`(field)または `entity`+`limit`, `prompt`(`{{field}}` `{{input}}` 置換。generate は必須・500 字まで), `options`(classify 候補), `shape`(line\|list\|fields・generate)+ `count`(list 1〜10)/ `items`(fields の名前 1〜10), `length`(short\|long。long は長い文・オンライン AI だけ・classify 以外・generate は line だけ), `inputPhoto`(同じ entity の photo field。写真も AI に見せる・オンライン AI だけ・screen.entity 必須), `saveTo`({entity, field}・generate の list の 1 項目を 1 件の記録に), `inputFeeds`(同じ画面の feed の id 1〜3 個・generate の材料に feed の行を足す), `outputs`(fields の項目名 → field id・generate の fields を項目ごとにフォームへ) | ✓ | P1 / #417 |
| `nearbyShare` | `label`, `entity`(passby は `fields` も) | `mode`(send\|passby・既定 send)、passby のみ `fields` `limit`(1〜20・既定 1) `distance` | | P2 / #425 |
| `locationTracker` | `entity`, `locationField`(location) | `dateField`(datetime), `label`, `seconds`(自動停止・既定 3600) | | P3 |
| `dashcam` | | `title`, `keepMinutes`(既定 10), `maxStorageMB`(既定 2000), `impactG`(既定 2.5), `autoStart`(landscape\|still\|charging の配列), `audio` | | #410 |
| `healthStat` | `metric`(steps\|weight), `label` | `period`(today\|week・既定 today), `target`(steps の 1 日の目標) | | P2 |
| `chatThread` | `entity`, `textField`(string/text) | `photoField`(photo), `placeholder`, `emptyText` | | #260 |
| `aiChat` | `entity`, `textField`(string/text), `roleField`(string か options に user / assistant を持つ enum), `persona`(500 字まで) | `length`(short\|long), `placeholder`, `emptyText`, `speak`(bool) | | |
| `sensorValue` | `sensor`(tilt\|pitch\|roll\|acceleration\|rotation\|pressure\|altitude), `label` | `style`(number\|gauge\|level), `min`/`max`(gauge), `entity`+`field`(number)+`dateField`(datetime), `seconds`(一定間隔 1〜3600), `limit`(既定 60・〜1000), `threshold`{`above`\|`below`, `message`, `save`, `action`} | | #419 |
| `musicPlayer` | | `title`, `allowRequests`(既定 false), `autoAccept`(`allowRequests` 時のみ・既定 false) | | #412 |
| `matchList` | `entity`(match "profile" の entity), `show`(incoming\|outgoing\|matches) | `title`, `emptyText`, `item`(title / subtitle / trailing / icon), `onItemTap` | | #507 |

`color` は `primary|secondary|accent|red|green|orange|blue` の閉じた enum(hex 文字列は不可・Claude 構造化出力互換。iOS は `Color.semantic(_:)` でシステム色に写像)。表示系の実例は `examples/gallery-display.json`(entity は家計簿と同じ `expense` なので iOS モックの seed がそのまま入る)。

### 3.2 アクション(16 種)

| type | フィールド | iOS の挙動 |
|---|---|---|
| `navigate` | `screen`, `presentation?`(push/sheet/fullScreen), `prefillRef?` | 遷移。`list.onItemTap` からは **recordId を暗黙に渡し**遷移先フォームが編集モードになる(だから遷移先は同じ entity のフォーム)。`prefillRef` 付きは**親から子を作る**: 今のレコードを子フォームの ref に入れて新規で開く(下の「親から子を作る」) |
| `back` | | スタックがあれば pop、モーダル最下層なら dismiss |
| `saveRecord` | `entity`, `screen?` | ドラフトを POST(新規)/PATCH(編集)→ レコード再取得 → `screen` へ、無ければ `back` |
| `deleteRecord` | `entity`, `confirmMessage?` | 現在の recordId を DELETE → back。list の長押し削除も同じ |
| `follow` / `unfollow` | `entity`(public / shared) | 今開いている投稿を書いた人をフォローする / 外す(#270・下の「フォロー」)。今の状態に合わない方と、自分の投稿では隠れる |
| `openVoice` | | 音声シートを開く |
| `openUrl` | `url` | Safari で開く |
| `resetForm` | | ドラフトを既定値に戻す |
| `scheduleNotification` | `notification` | ローカル通知を予約(`dsl.<id>`) |
| `cancelNotification` | `notificationId` | 予約を取消 |
| `export` | `entity`, `format`(csv/pdf), `filter?`, `fields?`, `title?` | 記録を CSV / PDF に書き出して共有シート(ファイル・メール・AirDrop)を開く(#421・下の「書き出し」) |
| `startActivity` | `activity`{`title`, `subtitle?`, `icon?`, `minutes?`(1〜480), `stat?`} | ライブアクティビティ(ロック画面 / Dynamic Island)を出す(#423・下の「ライブアクティビティ」) |
| `endActivity` | | 出しているライブアクティビティを消す(#423) |
| `addToCalendar` | `entity`, `calendarEvent`{`startField`, `endField?`, `durationMinutes?`(1〜10080・既定 60), `titleField?`, `locationField?`, `notesField?`, `alertMinutes?`(0〜10080)} | 今のレコードを端末のカレンダーに入れる。OS の「新規イベント」画面を埋めて開く(下の「カレンダーへの追加」) |
| `lookupBook` | `bookLookup`{`isbnField`, `titleField?`, `authorsField?`, `publisherField?`, `publishedDateField?`} | フォームの ISBN から書名・著者・出版社・出版日を引いて、入力中のフォームの field を上書きする(保存はしない・下の「本の情報」) |

`confirmMessage` はどのアクションにも付けられ、先に確認ダイアログを出す。

- **書き出し `export`(#421)**: 記録を CSV(UTF-8 BOM 付き・CRLF・1 行目は項目名)か A4 の PDF(見出し・書き出し日・絞り込み・件数・グラフ・表)にして `UIActivityViewController` で共有する。値は一覧・表と同じ書式(日付・¥・選択肢・参照先の名前・計算項目)で、写真・動画は入れない。`fields` 省略で全項目。`filter` は `list.filter` と同じ(単体 / 配列)。hidden(#262)は自分が書いた行だけ値を出し、他人の行しか無ければ列ごと出さない。`=` `+` `@` 等で始まるセルは `'` を前に付ける(CSV インジェクション対策)。PDF には押した画面にある同じ entity の `chart` を `ImageRenderer` で画像にして表の上に入れる。Leaf(`exportButton`)ではなく Action にしたのは、(1) toolbar からも同じ動作を置ける、(2) `confirmMessage` 等の既存の仕組みと button の見た目をそのまま使える、(3) 古い iOS は「未対応のアクション」の toast で済み画面が崩れない、の 3 点(上の「新規アクションは追加しない」の例外)。整形は `RecordExport` / `CSVWriter`(MetamonDSL・純関数)、PDF とファイルは `RecordExporter.swift`(Renderer)。審査用ビルドにも残す(ネイティブ権限が不要)。

- **カレンダーへの追加 `addToCalendar`**: 今のレコード(`screen.entity == entity` のフォーム画面の button / toolbar なら入力中の値・未保存でも、`list.onItemTap` ならタップした行)を EventKitUI の `EKEventEditViewController` に埋めて開き、ユーザーが「追加」を押すと端末のカレンダー(iCloud・Google など端末に設定したもの)に入る。iOS 17 以降この画面は別プロセスで動くので、**カレンダーの許可・`NSCalendars*UsageDescription`・entitlement は要らない**(アプリは端末の予定を読まない)。`startField` が `date` なら終日、`datetime` ならその時刻から。終わりは `endField` の値(空・始まりより前なら無視)→ `durationMinutes`(既定 60)。名前は `titleField` → `entity.titleField` → 最初の string → entity 名。`alertMinutes` はカレンダー側の通知(N 分前・0 は始まり)。始まりが空なら「「開始」を入力してください」。Go は field の存在と型(start / end = date・datetime、title / location = string・text・enum、notes = string・text)と範囲を検証(`backend/pkg/dsl/calendar_event.go`)。組み立ては `CalendarEventPlan`(MetamonDSL・純関数)、画面は `AppRuntime+Calendar.swift`。Action にしたのは `export` と同じ 3 点(toolbar にも置ける・button の見た目と `confirmMessage` を使える・古い iOS は「未対応のアクション」の toast で済む)。審査用ビルドにも残す(権限が要らない)。止めるまで鳴る目覚ましはカレンダーではなく通知の `alarm`(§3.15・AlarmKit)。スクリーンタイム(FamilyControls)は権限と entitlement が要るので入れていない。

- **本の情報 `lookupBook`**: `screen.entity` のフォーム画面の button / toolbar だけに置ける(タップには置けない)。押すと入力中の `isbnField` の ISBN(10 / 13 桁・ハイフン可。本のバーコードは `qrScanner` で読める)を端末とサーバーで検査数字まで確かめ、appconfig `GET /v1/lookup/book` が鍵の要らない決まった出どころ(openBD)だけから引く。端末は外部へ直接つながらず、DSL に URL・出どころは書けない。見つかった値を `titleField` / `authorsField` / `publisherField`(string / text)・`publishedDateField`(string / text なら「2014-04-08」の形、date なら足りない月日を 1 日に)に上書きし、待つ間はボタンにくるくる。見つからない・つながらない時は toast で手入力に戻る。Go は置き場所と field の存在・型・重複を検証(`backend/pkg/dsl/book_lookup.go`)。値の組み立ては `BookLookupPlan`(MetamonDSL・純関数)、実行は `AppRuntime+Books.swift`。審査用ビルドにも残す。詳細: [book-lookup.md](../backend/features/book-lookup.md)。

構造・ナビ・ネイティブ系の設計メモ(新規アクションは追加しない):

- `shareButton` / `openUrlButton` / `locationButton` / `qrScanner` / `timerDisplay` は **Leaf 内で完結**し(ShareLink / `openURL` / CoreLocation / AVFoundation / Timer)、新しい `Action` 種別(`share` / `requestLocation` 等)は足さない。`ActionType` に case を足すとホスト(`AppRuntime.run` の網羅 switch)の改修が必須になるため。URL は既存の `openUrl` アクションで足りる。
- **ライブアクティビティ(#423)** は例外的に Action を 2 つ足した: `startActivity{activity:{title, subtitle?, icon?, minutes?(1〜480), stat?}}` / `endActivity`。自分の見た目を持たず、どのボタン(toolbar・list.onItemTap も)からでも「◯◯中を出す / 消す」をしたいので Leaf にしない。タイマーは `timerDisplay.liveActivity: true` で Leaf 内に閉じる。`activity.stat` は同じアプリの `stat` / `progressBar` の id(Go が型まで検証)。更新は端末内だけ(push 無し)。iOS 側の仕組みは `docs/ios/architecture.md` §12.1、裏で続くもの・止まるものは §17.4。実例 `examples/ramen_timer.json`。
- **他アプリ連携(#37 最小版)**: `shareButton.shareTo` で送り先を選ぶ。`line` → `https://line.me/R/share?text=<urlencoded>` を `openURL`(LINE 未インストールなら Web 版に遷移・写真は送れないので文面のみ)。`slack` / `discord` → 公式に汎用の共有 URL スキームが無いので **OS 共有シート(ShareLink)を開き、文面をチャンネル投稿向けに整形**(`**` `#` 等の Markdown 記号を落としたプレーンテキスト + 先頭に 💬)。`any`(既定)は従来どおり。キー名が `target` でないのは `progressBar.target`(number)と JSON キーが衝突するため(`columnCount` と同じ理由)。**Slack/Discord への Incoming Webhook 直接投稿は後段(#37 中版: appconfig 側にユーザー設定として保存し `postWebhook` アクションから投稿)**で、この版には含めない。
- **`locationTracker`(#168)**: 「記録を開始 / 記録を停止」と記録中の状態(経過時間・距離・点の数)。**開始を押した間だけ**現在地を `entity` に 1 点 = 1 レコード(`locationField` に "lat,lng"、`dateField` に時刻)で足す。アプリを閉じても停止まで続き、`seconds`(既定 1 時間・300〜14400 に丸める)で自動停止。権限は When In Use だけ。点の entity はフォーム不要(lint の「add form が無い」から除外)で、他の required field には defaultValue が要る(validate)。見せ方は既存部品の拡張だけ: `mapView.kind: "route"` で点を `dateField` の時刻順に線で結び、`stat.aggregate: "distance"`(m / km)/ `"duration"`(m:ss / h:mm:ss)を `field` = location field、`by` = 日時 field で集計する。**10 分以上空いた点は別の記録**として線を切り、距離・時間にも足さない(朝と夕方の散歩を 1 本にしない)。Expr の `agg` には足していない(必要になったら)。例: `examples/walk_route.json`。
- **`dashcam`(#410)**: ドラレコ。背面カメラのプレビューを出しながら 1 分ごとの区間(mp4)で録画し、直近 `keepMinutes` 分だけ端末(Application Support/metamon-dashcam/)に残す(ループ録画)。`maxStorageMB` を超えたら古いループ区間から消し、保存済みだけで上限に達したら録画を止める。重力を除いた加速度が `impactG` を超えたら、直前 1 分〜直後 30 秒にかかる区間を「保存済み」に移して消さない。`autoStart` の条件(横向き = 重力が端末の横方向・ほぼ静止・充電中)が全部 5 秒続いたら自動で開始(手で止めた後は、条件が一度外れるまで再開しない)。保存済みは部品の「保存した映像」から再生・写真に保存・共有・削除。録画中は画面のスリープを止める。**iOS は裏に回るとカメラを止める**ので、画面を出している間だけ録画する(部品に常に表示・裏に回ったら区間を閉じて停止)。映像はレコードに入れない(entity 不要・同期しない)。1 アプリに 1 つ(validate)。残す区間・衝撃・自動開始の判定は iOS `DashcamLoop` / `DashcamImpact` / `DashcamAutoStart`(MetamonDSL・時刻とセンサー値を差し替えてテスト)。審査用ビルド(#245)では外す。シミュレータはカメラが無いので「実機のみ」の表示。例: `examples/dashcam.json`。
- **`nearbyShare`(#36)**: 近くの Tailor It 端末へ MultipeerConnectivity(Wi-Fi / Bluetooth 自動・インターネット不要)でレコードを送る。**送る範囲**: `screen.entity == entity` のフォーム画面なら入力中の 1 件(未保存でも今の値)、それ以外の画面ではその entity の新しい順 最大 50 件。**受け取り**: 受信側は前面にいて今のアプリが nearbyShare を使っている間だけ待ち受け(ローカルネットワーク許可ダイアログを無関係なアプリで出さないため)、招待を受けると「◯◯さんから「出席」3 件」の確認を出し、承認した時だけ接続する(暗号化必須)。取り込みは **同じ appId**、または **同じ entity id で保存フィールドの id+型が一致**する時だけ。未知のフィールドは捨てる。設計・実機手順は `docs/ios/architecture.md` §近距離共有。Leaf 内で完結し新しい Action は足さない(上と同じ理由)。
- **`nearbyShare` mode:`passby`(すれ違い通信・#425)**: 新しい部品ではなく nearbyShare のモード(同じ MultipeerConnectivity・同じ審査用ビルドでの除外・Registry/型の追加なし)。この画面を開いて部品のスイッチをオンにしている間だけ、近くで**同じアプリ(appId)**を開いている人と自動でつながり、`entity` の自分の記録(受け取った記録・他人の記録は除く)の新しい順 `limit` 件(既定 1・最大 20 件・16 KB)を交換する。**送るのは `fields` に書いた保存 field だけ**(hidden #262・photo・video・ref・refs・computed は指定不可=backend 検証でエラー)。受け取った記録は同じ entity に追加(`entity` は private 必須。共有 entity だと他人のプロフィールを全員に公開してしまうため)、同じ相手・同じ記録は 2 度追加せず内容が変わったら更新する。部品内に会った人と日時の一覧を出す。初回オンの前に何を誰に送るかの同意を出し、オンの間は「探しています」の表示、画面を閉じる/アプリを裏に回すと止まる(iOS の制約)ことを常に表示する。`distance:true` は Nearby Interaction(UWB)でつながった相手との距離と方向を出す(iPhone 11 以降・SE を除く。非対応は一言だけ出して隠す)。
- `openUrlButton.url` は https のほか allowlist のカスタムスキーム(`line` `slack` `discord` `tel` `mailto` `maps` `comgooglemaps`)を許可する(backend `dsl.OpenURLSchemes` と iOS `ExternalURLPolicy` が同じ表・`line`/`slack`/`discord` は Info.plist `LSApplicationQueriesSchemes` にも登録)。`http://` や未知スキームは backend がエラー・iOS はプレースホルダ。
- `webView` は `WKWebView` の `UIViewRepresentable`(戻る/進む/再読込の小ツールバー + 読み込みバー)。https 以外は不可。WKWebView 自身がスクロールするので `GridScreenLayout.scrollsInside` には**入れない**(セル内で二重スクロールしない)。ページ内リンクが allowlist のスキーム(tel: 等)なら OS に渡し、それ以外の非 http(s) は無視。実例: `examples/gallery-web.json`(webView + LINE/Slack/Discord/共有シートの 4 ボタン + maps/tel/mailto/line リンク)。
- `calendar.onDateTap` は既存の `navigate` を使う。行タップは recordId を渡して編集、「追加」はタップした日を `dateField` の初期値として Renderer 内の `FormPrefill` に積み、次に開いた新規フォームが 1 回だけ消費する(`Action` に params を増やさない)。
- **親から子を作る(#256)**: コメント・回答・出欠・いいねのような「親のレコードに付く子のレコード」は、子の entity に親を指す `ref` field(例 `post_comments.post` → `posts`)を置き、`navigate` に `prefillRef: "<その ref field id>"` を付けて子のフォームを開く。**今のレコード** = `list` / `mapView` の `onItemTap` ならタップした行、`button` / `toolbar` ならその画面(`screen.entity`)のフォームのレコード。iOS は今のレコードの id を `FormPrefill` に積み、子のフォームを**新規**で開いて ref に 1 回だけ入れる(recordId は渡さない)。親が未保存(新規フォームの button / toolbar)の時は子を開かず「先に「<親の entity 名>」を保存してください」を出す(`AppRuntime.perform(.navigate)`)。実例: `examples/post_comments.json`(投稿のフォームの「コメントを書く」)。
  - 検証(`backend/pkg/dsl/prefill_ref.go`・エラー): `list` / `mapView` / `gridTable` の `onItemTap` と `calendar.onDateTap` が**別 entity のフォーム**へ `prefillRef` 無しで行く(タップした行の id で別 entity のフォームが編集モードで開き、該当レコードが無い。旧 d_quiz_game の問題一覧 → 回答フォーム)。メッセージに「同じ entity のフォームへ向ける / `prefillRef` に使える ref field」を入れ、AI の再生成にそのまま渡す。
  - `prefillRef` の条件: 遷移先は `screen.entity` のあるフォーム・field はその entity の保存される `ref`(`refs` / computed 不可)・`refEntity` が今のレコードの entity・遷移先フォームにその field の `picker` を残す(親が見えて選び直せる。prefillRef を知らない古い iOS でも保存できる)・`screen.entity` の無い画面の button / toolbar と、`calendar` / `gridTable` のタップ(自前で日付・マスを `FormPrefill` に積む)では使えない。
- **`audioRecorder` / Field `audio`(#420)**: マイクで録音して audio field に保存する入力(ボイスメモ・会議の録音)。録音 → 停止 → 再生・録り直し・削除。録音中は経過時間 / 上限と音量メーター。値は動画(#206)と同じ考え方で **端末内のファイル + レコードには `"audio:<id>|<秒>"` だけ**(AAC m4a・モノラル 64kbps を `Application Support/metamon-audio/<id>.m4a`・iCloud バックアップ対象外。Firebase Storage へのアップロードは動画と同じく未実装なので、他の端末では長さだけ出て再生できない)。レコード削除・録り直し・アプリ削除・アカウント削除でファイルも消す。`videoField` と同じ「field の型 + 入力部品」の形にしたのは、録音はレコードの 1 項目(メモと同じ 1 件に入る)で、一覧・ニアバイ共有・AI ボタンが型で扱いを決められるため(再生は録音部品と一覧の行が持つので `audioPlayer` は作らない)。一覧で再生するには `list.item.trailing` にその audio field(行の右端に再生ボタン + 長さ。同時に鳴るのは 1 本)。上限は `maxDuration`(秒・既定 600・1〜7200。`record(forDuration:)` が止める)。**イヤホンのボタン**: 部品が画面にある間 `MPRemoteCommandCenter` の 再生 / 一時停止 / 切り替え を受け、録音中なら停止・それ以外は開始(録音済みなら録り直し・再生中は再生を止めるだけ)。画面を離れたら外す。iOS がボタンを届けるのは最後に音を扱ったアプリだけなので、他のアプリで音楽を流している間は届かない(実機確認)。**音声入力を壊さない**: 録音・再生の間だけ AVAudioSession を借り(`AudioSessionLease`)、終わったら借りる前の category / mode / options に戻して非アクティブにする(途中で音声入力が設定を変えたら触らない)。音声入力側(`WhisperKitRecognizer`)は開始のたびに `.playAndRecord` を自分で設定する。審査用ビルド(#245)では部品ごと外す(`docs/ios/review-build.md`)。古い iOS では audio field は文字列扱い・部品はプレースホルダ(落ちない)。
- **`drawingField`(手書き・お絵かき)**: 指・Apple Pencil で描いて **photo field** に入れる入力(PencilKit。サイン・スケッチ・図・手書きのメモ)。フォームには描いた絵(無ければ「タップして描く」の枠)を出し、タップで全画面のキャンバスを開く(フォームのスクロールと描く指がぶつからないため)。道具はペン(黒)・消しゴム・元に戻す・やり直す・全部消す(確認あり)、上に キャンセル(描いた線があれば確認)/ 完了。完了で白地に描いた絵を**写真と同じ形**(`"data:image/jpeg;base64,..."`・800px 以下・線が残るよう q0.7。iOS `DrawingCodec`)にして field に入れ、何も無ければ空にする。インクはダークモードでも黒・地は白で保存する。保存済みの絵を開くとそれを下絵にして描き足せる(線は 1 枚の画像になるので、後から 1 本ずつは直せない)。**新しい field の型を足さず photo に入れた**のは、(1) 一覧(`item.image`)・`image`・共有ページ・同期・ニアバイの除外など写真の扱いがそのまま効く、(2) サーバーに保存される物の種類が増えず、プライバシーの表示(「写真またはビデオ」)と審査の説明が写真と同じで済む(`docs/store/privacy-label.md`)、の 2 点。写真も撮るなら photo field を分ける(photoField と drawingField を同じ field に付けない)。手書きの文字認識はしない。VoiceOver: フォームの枠は「<label>(描いた絵あり)」+ ヒント、キャンバスは「手書きのキャンバス」(直接操作)と線の本数、全部消すはヒントと読み上げ。大きい文字では「描き足す / 削除」を縦に並べる。審査用の版にも入れる(権限を使わない)。古い iOS では未知の部品としてプレースホルダ(落ちない・backend は v1.0 に出さない)。検証 `backend/pkg/dsl/drawing_field.go`(screen.entity 必須・保存される photo field)。
- `gridTable`(#234)は時間割のマス目。列 = columnField の値(enum の options か columnValues)、行 = rowField の値(integer は rowRange、enum は options か rowValues)。マスのタップは `onItemTap`: 記録 1 件 = recordId 付きで編集、空き = 列・行の値を `FormPrefill` に積んで新規、複数 = メニュー(各記録の編集 + 追加)。1 マスに複数ある時は先頭 + 「+n」。列見出しが曜日なら今日の列を強調。列が画面幅に収まらなければ横スクロール、縦は `GridCell` の内側スクロール。配置計算は iOS `GridTableLayout`(MetamonDSL)。実例: `examples/timetable.json`。
- **`randomPicker`(#263)**: ボタンを押すたびに entity のレコード(`filter` で絞れる)から 1 件をランダムに選び、`titleField`(+ `subtitleField`)を大きく出す(約 0.7 秒のシャッフル演出・「視差効果を減らす」では演出なし)。候補が 2 件以上なら直前と同じ 1 件は続けて出さない。**乱数は Expr に入れない**: 計算式は描画のたびに評価されるので、乱数を入れると再描画・レコード再取得のたびに値が変わる。選んだ 1 件は部品の表示状態(SwiftUI `@State`)だけに持ち、保存しない(画面を離れると消える・再描画では変わらない)。選んだ 1 件のタップは `onItemTap` を recordId 付きで実行(list の行タップと同じ)。選び方は iOS `RandomPick`(MetamonDSL・乱数源を差し替えてテスト)。古い iOS では未知 type としてプレースホルダになる(落ちない)。実例: `examples/dinner.json`。
- **ファイルの取り込み `fileImport`**: ボタンで iOS のファイル選択(`.fileImporter`・別プロセスで権限の確認なし)を開き、選んだファイルから entity に記録を足す。**処理はすべて端末内**で、ファイルそのものは送らない・コピーしない・残さない(足した記録だけが `createRecord` で今まで通り保存・同期される)。足す前に件数と読めない行を出して確かめる。
  - **csv**: 1 行 = 1 件。1 行目は項目名で、`map` に書いた列が先に、残りは項目名が field の `name` か `id` と同じ列に当たる(前後の空白・大文字小文字は無視・1 つの field に 1 列)。計算項目・写真・動画・録音・参加者(member / members)の列は使わない。`export` の CSV(BOM・CRLF・`¥1,200`・はい / いいえ・`10/24(金)`(年なしは今年)・参照先の名前・先頭の `'`)はそのまま読み戻せる。文字コードは UTF-8(BOM 有無)→ UTF-16(BOM)→ Shift_JIS(日本語の Excel)。引用符・`""`・セル内の改行・CRLF / LF / CR に対応。型に合わないセル(数にできない・選択肢に無い・参照先が無い・長すぎる)や空の必須項目がある行は入れずに数え、最初の 3 行を「n 行目: 項目「値」を読めません」で見せる。空のセルはフォームの新規と同じ既定値(boolean false・enum の最初・日付は今日・`defaultValue`)。1 回 500 件・entity の合計 2000 件(`store.MaxRecordsPerEntity`)まで。
  - **text / pdf**: 本文を取り出して `field`(text は 20000 字・string は 4000 字まで。超えたら切って伝える)に入れた記録を 1 件足し、entity の `titleField`(string / text)にはファイル名(拡張子なし)を入れる。PDF は PDFKit の文字の層を読み、文字の無いページ(スキャン)は `photoField.analyze` と同じ Vision の文字認識で読む(最大 10 ページ)。パスワード付きの PDF は読めない。
  - **audio(録音ファイルの文字起こし)**: ファイル選択で録音(`UTType.audio` = m4a・mp3・wav・aiff など。ボイスメモの書き出し・授業 / 会議の録音)を選ぶと、音声入力・`transcribeField` と同じ端末内の WhisperKit で文字にして、text / pdf と同じく `field` に入れた記録を 1 件足す(題名はファイル名)。1 ファイル **30 分・200MB まで**(`dsl.MaxImportAudioMinutes` / `MaxImportAudioMB`・iOS `AudioFileImport`)。2 分ごとに区切って文字にし(無音の区切りは飛ばす)、進み具合(%)と「キャンセル」を出す。**前面にある間だけ**: 文字にしている間は画面を消さず、背面に回る・画面を離れると止めて「もう一度選んでください」と出す(背面で続ける仕組みは持たない)。**音声も文字もサーバー・AI に送らない**(足した記録だけがいつも通り同期)、音声ファイルはコピーも保存もしない(読むのは選んだファイルを開いている間だけ)。WhisperKit のモデルが未用意なら部品の下に「録音ファイル用の音声モデルを用意する」(音声入力と同じダウンロードの確認)。音声入力の仮の聞き取り(Apple の端末内の認識)は短い発話向けなので録音ファイルには使わない。録音中・音声入力のシート中は使えない。話者の区別・時刻付きの字幕は無い。
  - 検証(`backend/pkg/dsl/file_import.go`・`file_import_audio.go`): entity は private / shared だけ(public・single・match は不可)。`accept` は csv / text / pdf / audio の重複なし。text / pdf / audio があれば `field` は保存する text / string の field(audio は text だけ: 30 分の文字起こしは string の 4000 字に収まらない)、無ければ `field` を書かない。`map` は csv の時だけで、列・field の重複なし、計算項目・写真・動画・録音・参加者(member / members)は不可(最大 50)。text / pdf / audio の entity は、本文・題名・既定値で埋まらない required の field を持てない。
  - 読めないもの: 動画・画像・Word・Excel(.xlsx)のファイル(Excel は CSV で保存してもらう)。審査用ビルドにも残す(権限が不要で端末内の処理だけ・`docs/ios/review-build.md`)。プライバシーの申告は変わらない(`docs/store/privacy-label.md`)。
- **`flashcards`(#264)**: 単語カード・暗記カード。entity のレコード(`filter` で絞れる)を 1 枚ずつ出し、タップで `front` と `back` をめくる(「視差効果を減らす」では回転せずに切り替える)。右スワイプ = 「覚えた」・左 = 「まだ」で `toggle`(boolean・computed 不可)に true / false を保存し次のカードへ(保存は `list.item.toggle` と同じ `updateRecord` の経路・楽観的 UI)。スワイプの代わりに「まだ」「覚えた」ボタンも出す(VoiceOver はカードの「めくる」と 2 つのボタン)。`filter: {field: toggle, op: eq, value: false}` で覚えたカードを出題から外す(保存値の無いカードは「まだ」として扱う)。上に「n / 全体」の進み具合、全部終えたら完了表示と「もう一度」。どこまで進んだかは部品の表示状態だけに持ち、保存しない(画面を離れると最初から)。`toggle` を省くと記録せずにめくるだけ。並べ方は登録順。状態の計算は iOS `FlashcardDeck`(MetamonDSL)。古い iOS では未知 type としてプレースホルダになる。実例: `examples/vocab.json`。
- **`feed`(#348)**: 外部の公開データの読み取り専用の一覧。appconfig(`GET /v1/feeds/{provider}`)が**運営の許可リストの出どころだけ**から取る(気象庁の天気予報・内閣府の祝日・許可した公的機関の RSS)。DSL に URL は書けない(provider は閉じた enum・`region` / `feedId` は許可リストと照合)。行は日付・見出し・(天気)アイコン・最高 / 最低・降水確率、(RSS)タップで記事を Safari で開く(https だけ)。下に出典を必ず出す。読むだけで、記録に保存しない・計算やグラフに使えない。審査用の版では部品ごと外す。古い iOS では未知 type としてプレースホルダになる。詳細: [external-feeds.md](../backend/features/external-feeds.md)。
- `searchBar.field` は entity の field ではなく**画面ローカルのキー**(`ScreenLocals`)。`list.searchField` で参照し、レコードには保存しない。
- `mapView` は `latField`/`lngField` の 2 列ではなく既存の **`location` 型 field 1 つ**(`"lat,lng"`)を使う(`locationButton` の保存先と同じ表現・entity の field 数を増やさない)。既存 `map` Leaf は無かったので新設。
- **`routeGuide`(#411)**: 目的地までの道案内。MapKit `MKDirections` の経路を地図に線で出し、次に曲がる所(矢印・距離・案内文)・残りの距離・あと何分・到着の予想時刻を現在地(`CLLocationUpdate.liveUpdates`)に合わせて進める。音声案内(`AVSpeechSynthesizer`・700 / 300 / 80 m 手前、徒歩は 100 / 25 m 手前・到着)はスピーカーのボタンでオン・オフ(端末に保存)。車 / 徒歩は画面で切り替えられ(`mode` は最初の値)、案内中に切り替えると今の位置から引き直す。道を外れた位置(車 50 m・徒歩 30 m + 位置の精度)が 3 回続いたら引き直す(最短 15 秒おき)。目的地は 1 つだけ: `address`(住所・施設名を `MKLocalSearch` で検索した 1 件目)/ `entity` + `locationField`(画面の entity と別 = ホームに置き、場所を持つレコードから部品の中で選ぶ)/ `locationField` だけ(`screen.entity` の今のレコード・同じフォームの `locationPicker` で選んだ値もそのまま使う)。**使用中の位置だけ**(背面で案内を続けない・`CLBackgroundActivitySession` を使わない)ので審査用ビルドにも残す。案内中は画面を消さない(`isIdleTimerDisabled`)。画面を離れると止まる。車線・渋滞の回避・経由地・自転車 / 電車は無い(MapKit の範囲)。進み方・読み上げる時は iOS `RouteGuidance`(MetamonDSL・位置を渡してテスト)。モック(`METAMON_MOCK=1`)は権限・通信を使わず金沢駅からの L 字の経路を 0.5 秒ごとに進む。実例: `examples/drive.json`。
- **`qrCode`(#422)**: 文字・URL・レコードの値から QR コードを作って表示する(読み取りの `qrScanner` と対)。値は次のどれか 1 つ(検証 `backend/pkg/dsl/qrcode.go`): `text` = 固定の文字(最大 1000 文字かつ 2331 バイト = バージョン 40・誤り訂正 M の容量。screen.entity の画面では `{{fieldId}}` をそのレコードの値で置換)/ `field` = screen.entity の field(表示中のフォーム・詳細のレコード。photo / video / refs と hidden は不可)/ `value` = Expr(screen.entity の画面なら `ref` でそのレコード)。iOS は CoreImage `CIQRCodeGenerator`(誤り訂正 M)で作り、1 モジュール = 整数ピクセルに拡大して補間なしで描く(ぼけない)。周りに 4 モジュールの白い余白を付け、ダークモードでも白地に黒。長押しのメニューとボタンで写真に保存(`NSPhotoLibraryAddUsageDescription`・追加のみ)と共有シート。値が空・長すぎる時は案内を出す(落ちない)。審査用の版にも残す。
- **`transcribeField` / `speakButton`(文字起こし・読み上げ)**: どちらも端末の中だけで動き、声をサーバーに送らない。審査用の版にも残す(検証 `backend/pkg/dsl/speech.go`)。`transcribeField` は押して話すと、アプリの音声入力(マイク FAB)と同じ端末内の音声モデル(WhisperKit)で文字にし、screen.entity の string / text の field の後ろに足す(空なら置き換え・それ以外は改行で区切る)。欄は複数行でそのまま手で直せるので、同じ field に textEditor を重ねない。録音は残さない(音声も残すなら別の audio field に `audioRecorder`)。マイク FAB の音声入力が始まった・画面を離れた時は録音を捨てる。`speakButton` は `qrCode` と同じ 3 つの出どころ(`text` 最大 2000 文字・`{{fieldId}}` 可 / `field`(photo / video / audio / refs 不可)/ `value`)の文を `AVSpeechSynthesizer` で読み上げ、もう一度押すと止まる。声は UI の言語(ja-JP / en-US)。`text` の `{{fieldId}}` と `value` に hidden の field は使えない。読み上げは録音の間は止まり、アプリの共有の音声セッションに触らない(読み上げ #43 と同じ別セッション)。
- **`placeSearch`(近くの場所を探す)**: 現在地の近くのお店・施設を Apple のマップで探す一覧(フォーム画面のみ)。`category` があれば `MKLocalPointsOfInterestRequest`(`query` も付いていれば `MKLocalSearch` に `pointOfInterestFilter`)、それ以外は `MKLocalSearch`(`query` か利用者が打った語)を現在地の周り約 3 km で引き、近い順に名前・距離・住所を最大 15 件出す。`pins: true` で同じ結果を小さな地図のピンでも出す。「記録に追加」は入力中のフォームの `nameField`(string / text)・`locationField`(location・`"lat,lng"`)・`addressField`(string / text)を上書きするだけで、保存は `saveRecord`(lookupBook と同じ)。行った所はホームの `mapView` に出る。`category` は閉じた 18 種(iOS 17 の `MKPointOfInterestCategory` に写像・`dsl.PlaceCategories`)、`near` は今は `current` だけ(使用中の位置・権限はその場で聞き、断られたら「設定で位置情報を許可すると近くを探せます」と出して検索欄だけで探せる)。**検索語と現在地のおおよその範囲は端末から Apple に送られ、本アプリのサーバーには送らない**(API キー無し)。営業時間・空席・値段・口コミ・上映時間は出ない。検証 `backend/pkg/dsl/place_search.go`(screen.entity 必須・field の存在・型・重複・category / near / query の長さ)。iOS は `PlaceSearchLeaf` と `PlacePicking`(locationPicker と共用)。モック(`METAMON_MOCK=1`)は権限・通信を使わず固定 3 件。審査用ビルドにも残す(使用中の位置だけ)。実例 `examples/nearby_spots.json`。
- **`locationButton.addressField`**: 取った現在地を `CLGeocoder` で住所(例: 石川県金沢市広坂1丁目)にして string / text field に入れる(逆ジオコーディング・座標は端末から Apple に送られる)。住所が取れない時は座標だけ入れて住所の欄は変えない。field と同じ field は不可(`place_search.go`)。
- **`locationPicker`(#212)**: 現在地以外の場所を選ぶ入力欄。シートで店名・地名・住所を検索(MapKit `MKLocalSearch`・API キー不要)/ 地図を動かして中央のピン / 現在地 から選ぶ。保存は `locationButton` と同じ `"lat,lng"`(値の形を増やさない=mapView・appconfig の検証はそのまま)。場所の名前は任意の `nameField`(string field)に入れる(ピン・現在地で名前が無い時は打った名前を消さない)。「今いる場所」だけなら `locationButton`。モック(`METAMON_MOCK=1`)の検索は固定 3 件。
- **`healthStat` / `Field.health`(#167)**: 端末のヘルスデータ。`healthStat` は **entity を持たない表示カード**で、歩数は iPhone の歩数計(CoreMotion・直近 7 日・capability 不要)、歩数計の無い端末(シミュレータ)はヘルスケアの歩数、体重はヘルスケアの最新値を読む。today は数値(steps は `target` で達成率バー)、week は 7 日のグラフ(steps は棒 + 目標線・weight は折れ線)。許可が無い・使えない端末は「ヘルスケアへのアクセスが許可されていません」等を表示。書き込みは部品ではなく **`Field.health: "weight"`**(number・unit kg): フォームで**新規**保存した時にヘルスケアの体重にも 1 件書く(編集保存では書かない・失敗してもレコードは保存済みでトーストだけ)。入力部品や `saveRecord` を増やさず「どの値をヘルスケアに出すか」をデータ側に 1 キーで持つため。実例: `examples/health.json`。設計は `docs/ios/architecture.md` §15.1。
- **`sensorValue`(#419)**: iPhone のセンサーの今の値。tilt / pitch / roll(°)・acceleration(G・重力を除いた揺れ)・rotation(°/s)は CoreMotion の device motion、pressure(hPa)・altitude(m・**画面を開いた時を 0 とする相対値**)は CMAltimeter。**画面に出ている間だけ**測り(`.task` の開始・取り消し)、表示の更新は最大 10 回/秒。`entity`+`field` で「今の値を記録」(1 回 1 レコード・`dateField` に時刻・他の field は `defaultValue`)、`seconds` で「記録を開始/停止」(画面を開いている間だけ・`limit` 件で自動停止)、`threshold` は条件に入った瞬間に 1 回だけ発火(警告表示 + 振動・`save` で 1 件保存・`action` は navigate / scheduleNotification / cancelNotification のみ)、条件から出るまで再発火せず最短 3 秒おき。form の無い entity でも lint は通る(locationTracker と同じ)。**温度・湿度は iPhone にセンサーが無いので無い**(AI は「測れません」と答え、手入力の記録を代わりに出す・外付けセンサーは別 Issue)。センサーの無い端末(シミュレータ)は「このセンサーはこの端末では使えません」、`METAMON_MOCK=1` は決まった波形。計算は iOS `SensorReading`(MetamonDSL)。審査用ビルドでは外す(docs/ios/review-build.md)。実例: `examples/sensors.json`。
- **`text.highlight`(文中の語に印)**: text の中の語に印をつける(表示だけで文は変えない・VoiceOver はふつうの文として読む)。語は次のどちらか 1 つ: `words` = 決まった語(1〜20 個・1 語 50 文字まで)/ `field` = screen.entity の string / text の field(値を「,」「、」か改行で区切った語。iOS は空・重複を捨てて先頭 20 個)。`style` は `marker`(既定: 蛍光ペンの背景 + 太字)か `bold`(太字だけ)。照合は NFKC + 小文字で大文字小文字・全角半角を区別せず、語の区切りを見ない部分一致(日本語は語の一部でも合う)・重なりはまとめる。合わせて screen.entity の画面(フォーム・詳細)では text の `{{fieldId}}` が開いているレコードの値になる。置き換えるのは、見えている(hidden でない)ふつうの値の field(string / text / integer / number / boolean / date / datetime / enum / currency)だけで、知らない名前・hidden・写真や参照などの field と screen.entity の無い画面の `{{x}}` は前と同じく書いたままの文字で出す(どれも検証エラーにしない・保存済みのアプリを壊さない・hidden の値は出さない。Go `dsl.TextSubstitutes`・iOS `TextHighlight.substitutes` が同じ決まり)。共有ページ(Web)はレコードの値を出さないので、置き換える `{{fieldId}}` を「—」にする。検証 `backend/pkg/dsl/text_highlight.go`、iOS は `TextHighlight`(MetamonDSL・照合は純関数)と `TextLeaf`(AttributedString)。印の語を AI に選ばせるなら同じフォームに `aiButton`(task `extract`・output = その field)を置く(jphacks/E_2202 のエラー文の重要箇所)。新しい部品にしなかったのは、(1) 文の表示は text の役目で、印は見た目の付け足し(古い iOS は `highlight` を無視して文だけ出す)、(2) 閉じた 3 キーで AI が迷わない、の 2 点。
- `aiButton`(#78)は **端末内の AI を優先する AI 部品**(Apple Intelligence / Foundation Models・iOS 26+)。端末内で動く時はデータは端末外に出ず、課金も無い。結果は output field のドラフトに入るだけで保存はユーザー。非対応端末・Apple Intelligence OFF・モデル準備中は、generate と同じく ai サービスの `POST /v1/text` に `task`(summarize など)を付けて回す(**Tailor It Pro の機能**・AI 送信の同意が要る・材料は文字だけ 4 KB まで・写真は送らない・[text.md](../backend/features/text.md))。オンライン版を持たないホスト(審査用ビルド・プレビュー)では「この端末では AI を使えません」と理由付きで無効化。DSL の宣言は変わらない(どこで動くかはホストが決める)。`Action` の `runAI` 案ではなく Leaf にしたのは、(1) 生成 AI が 1 部品で書き切れる(button + action の 2 段より短く、入力・出力 field を同じ場所で検証できる)、(2) 実行中スピナー・不可時の無効表示が部品の見た目に属する、(3) `ActionType` を増やすとホストの網羅 switch 改修が要る、の 3 点。実例: `examples/diary_ai.json`(本文 → 要約・気分の分類)。
- **`photoField.analyze`(#418・写真を AI で読み取る / 骨格)**: 写真を撮った・選んだ直後に**端末内で**読み取り、同じレコードの `into` field のドラフトに入れる(保存はユーザー・読み取れなかった項目は今の値のまま・プレビューの下の「もう一度読み取る」で読み直す)。`kind` は閉じた 11 種: `text`(Vision の文字認識 ja+en → string/text)・`amount`(OCR の「合計」の行、無ければ ¥/円 付きの最大額 → currency/integer/number)・`date`(OCR の `2026/10/28`・`2026年10月28日` など → date/datetime)・`classify`(Vision の画像分類のラベル + OCR を Foundation Models で候補 1 つに寄せる。Apple Intelligence が無い端末は候補名とラベル・文字の一致だけ → enum か options 付き string/text)・`describe`(ラベル + OCR から Foundation Models で 1 文。無い端末はラベルの列挙)・`jointAngle`(Vision の `VNDetectHumanBodyPoseRequest` で `joint` の角度 0〜180 度 → number/integer。プレビューに関節の点と線を重ねる。`knee` など左右無しは信頼度の高い側)・`label`(`VNClassifyImageRequest` のラベルのうち、`food` `plant` などの大まかな語を除いたいちばん確かなもの 1 つ。主なラベルは日本語に直し、知らないラベルは英語のまま → string/text)・`labels`(同じラベルを最大 5 つ「、」区切り → string/text)・`barcode`(`VNDetectBarcodesRequest` で写真の中の最初のバーコード・QR の中身 → string/text。カメラで連続して読む `qrScanner` とは別で、撮った・選んだ写真から読む)・`peopleCount`(`VNDetectFaceRectanglesRequest` の顔の数と `VNDetectHumanRectanglesRequest` の人の数の多い方 → integer/number。0 人も値として入れる。顔の特徴は取らず誰かは見分けない)・`color`(写真の中央 60% を 32×32 に縮めた画素を 12 色の名前に振り分け、いちばん多い色 → string/text)。検証(`backend/pkg/dsl/photo_analyze.go`): into は screen.entity の保存される field で photo 自身・computed 不可、kind ごとの型、into の重複、jointAngle の joint 必須、classify の候補(2 つ以上・enum なら options の部分集合)。**新しい部品ではなく photoField の拡張**にしたのは、(1) 撮る操作と読み取りが 1 部品で書け AI が迷わない、(2) 古い iOS は `analyze` を無視して従来の写真入力として動く(落ちない)、(3) 写真ライブラリから選ぶだけでも動くので審査用の版(#245)でも外さなくてよい、の 3 点。backend(Claude の画像入力)へのフォールバックと、カメラ映像でのリアルタイムの回数カウントは後続(課金・認証・レート制限の重み付けが要るため)。実例: `examples/receipt_ai.json`(レシート → 金額・日付・分類・文字)、`examples/form_check.json`(スクワット → 膝・腰の角度)。
  - **拡張の kind(label / labels / barcode / peopleCount / color)の範囲**: 過去の JPHACKS の作品で「写真の理解」が要る 18 作品(`docs/research/jphacks-coverage.md` の 6 番)のうち、端末内の Vision の標準機能だけでできるものに絞った。どれも写真は端末の外に出さず(クラウドの AI に送らない)、新しい権限も使わないので、**プライバシーの申告(App Privacy・プライバシーポリシー・同意の画面)は変わらない**。審査用の版でも同じように残す(選んだ写真を端末内で読むだけ)。作らなかった物: 食べ物のカロリー・栄養の推定と植物・名所の細かい種類(端末内に使えるモデルが無い。独自モデルの同梱は 2.5.2 とアプリの大きさの面で別の判断)、顔で人を見分ける(個人の特定になる)、2 枚の写真の見比べ(`VNGenerateImageFeaturePrintRequest` で距離は出せるが、お手本の写真が別のレコードにあることが多く、点数の目安も決めにくいので後続)、書類の検出・切り抜き(値ではなく写真そのものを書き換えるため `into` の形に合わない。切り抜くなら撮影の画面の側の作業)、明るさ(要る作品が無い)。
- **`aiButton` の `task: "generate"`(#417)**: 決まった指示(`prompt`)で AI に新しく書かせるボタン(罰ゲームの案・今日の献立・励ましの一言)。結果は `shape` の形でボタンの下に出す: `line`(1 行・既定)/ `list`(箇条書き `count` 個・既定 3)/ `fields`(`items` の名前ごとに 1 つ)。材料は任意で `entity`(新しい順 `limit` ≤ 20 件)か form の `input` / `{{field}}`、iOS が 4 KB に切って渡す。`output` を付けるとフォームのドラフトにも入る(保存は利用者)。付けなければ表示だけで、どの画面にも置ける(screen.entity 不要)。実行は **端末内 AI(Foundation Models)を優先**し、使えない端末だけ ai サービスの `POST /v1/text`(Claude・出力 400 トークン・利用上限の重さ 1)に回す。オンライン版は **Tailor It Pro の機能**(Pro でなければ課金画面・AI 送信の同意が無ければ同意画面)。モック(`METAMON_MOCK=1`)は決まった文を返す。新しい部品 `aiText` にせず aiButton の task にしたのは、(1) 指示テンプレ・`entity`+`limit` の材料・`output` への書き込み・端末内 AI の実行と可用性表示が aiButton と同じで、別部品だと全部が二重になる、(2) AI が「AI で何かするボタン = aiButton」と 1 語で選べる、(3) 部品の enum・レンダラ登録など他の部品と取り合う共有行を増やさない、の 3 点。違いは「入力を変換する(要約など・端末内だけ・無料)」か「指示で新しく書く(generate・オンラインの代わりあり)」で、task で分かれる。
- **`aiButton.length`**: `short`(既定・今までどおり)/ `long`(長い文: 長い文の要約・手紙・計画・台本)。long は端末内 AI を使わず、**いつも `POST /v1/text` のオンライン AI**(Tailor It Pro・AI 送信の同意が要る)で、材料 16 KB まで・出力 1500 トークン(答え 3000 字)まで・利用上限の重さ 3([rate-limit.md](../backend/rate-limit.md))。classify には付けない、generate は `shape: line` だけ、`output` は text field(string は 1 行のため)。iOS は結果をボタンの下でスクロール・選択できる形で出す。端末内 AI は短い答えに合わせた大きさで、長い文は途中で切れる・形が崩れるため分けた。古い iOS は `length` を無視して短い答えのまま動く(落ちない)。
- **`aiButton.inputPhoto`**: フォーム(screen.entity)の photo field の id。押すとその写真 1 枚も材料として AI に見せる(処方箋・薬の袋・ラベル・ホワイトボード・手書きのメモを読む)。端末内 AI は写真を読めないので、**いつも `POST /v1/text` のオンライン AI**(Tailor It Pro・AI 送信の同意が要る)。iOS が JPEG・長い辺 1024px 以下・base64 400 KB 以下に縮めて送る。task は何でもよく、写真だけを材料にしてもよい(`input` は任意)。利用上限の重さ 3(task / length の重さと大きい方)。サーバーは `AI_TEXT_IMAGE=1` の時だけ受け付ける(プライバシーポリシーが写真の送信を書くまで既定 off・[features/text.md](../backend/features/text.md))。古い iOS は `inputPhoto` を無視し、写真なしで動く。
- **`aiButton.saveTo`({`entity`, `field`})**: generate の `shape: list` の結果を、**1 項目 = 1 件の記録**として entity に足せるようにする(問題・単語カード・手順・アイデアを一覧に貯める)。iOS は結果の各行に「保存」、下に「全部保存」を出し、利用者が押した時だけ足す(押すまで何も保存しない・保存した行は「保存済み」になり二重に足さない・もう一度生成すると新しい結果になる)。記録はフォームの保存と同じ経路(`createRecord` → 端末の DB → 同期の列)で書くので、共有データの決まり・プランの上限・表示できない言葉の検査はいつもどおり効く。field は entity の string / text(上限を超える文字は切る)、ほかの field は defaultValue か空(boolean・enum・date は新しいフォームと同じく自動で入る)。1 回で足すのは結果の個数まで(`count` ≤ 10)。検証(`backend/pkg/dsl/ai_save_to.go`): task generate + `shape: list` だけ、`output` とは一緒に使わない、entity は存在して private / shared(public = 全員に AI の行を投稿してしまう・single・match・addressedTo は不可)、field は computed でも editableBy admins でもない string / text、それ以外の required field は defaultValue か自動で入る型であること。`/v1/text` は変えない(list の結果を iOS が記録にするだけ)。古い iOS は `saveTo` を無視して表示だけになる(落ちない)。
- **`aiButton.inputFeeds`**: generate の材料に、**同じ画面の `feed` が読み込んだ行**を足す(「今日の天気と気圧から体調のアドバイス」「天気に合う服装」「警報を見て今日の注意点」)。値は同じ画面(器の中も可)にある feed 部品の `id` の配列(1〜3 個・重複不可)。iOS は押した時に、その feed が画面に出している行(同じ問い合わせのキャッシュ = 表示と同じ行)を、日付・見出し・補足・値(最高 / 最低・降水確率・気圧の幅と急な低下)の 1 行ずつにして材料の末尾に「外部データ」の節として付ける。上限は**合わせて 20 行・2 KB**(材料全体は今までどおり 4 KB / long 16 KB で、はみ出す時は記録・フォーム側を先に削る)。まだ読み込んでいなければ少し(8 秒まで)待って取り、取れなければ「データを読み込んでから押してください」。端末内 AI でもオンライン AI(Tailor It Pro・作成者の Pro で判定)でも同じ材料で動く。feed の行は運営が許可した公開データで、他の利用者の記録ではない。`/v1/text` は変えない(材料の文字列が増えるだけ)。検証(`backend/pkg/dsl/ai_input_feed.go`): task generate だけ、各 id は同じ画面の feed。審査用ビルド(feed を外す)と古い iOS は `inputFeeds` を無視して feed の行なしで動く(落ちない)。
- **`aiButton.outputs`**: generate の `shape: fields` の結果を、**項目ごとにフォームの field へ入れる**(貼ったポスターの文から題名・日時・場所を抜き出して予定のフォームに入れ `addToCalendar` で予定にする / 答えを採点して点数を number field に入れる)。値は `items` の名前 → screen.entity の field id(1〜10 個)。iOS は結果を出したあと、各値を field の型に直してフォームのドラフトに入れる: string / text はそのまま(上限で切る)、integer / number は数(「¥1,200」「85 点」も読む・integer は丸める)、date / datetime は日付(「2026年11月20日 18:00」「11/20(金) 18:00」・RFC 3339 など。CSV の取り込みと同じ読み方)、enum は options のどれかと一致する時だけ。読めない値(「不明」「-」も)は**空のまま**にする(入力済みの値は消さない)。保存は利用者が押す(勝手に保存しない)。入れた field は VoiceOver で読み上げる。検証(`backend/pkg/dsl/ai_outputs.go`): task generate + `shape: fields` だけ、`output` とは一緒に使わない、screen.entity が要る、各キーは `items` の名前、各 field は存在して computed でない string / text / integer / number / date / datetime / enum、同じ field を 2 つの項目で使わない、`input` と同じ field は不可。`/v1/text` は変えない。端末の機能キーは `aiButton.outputs`(無い版には生成しない・古い iOS は `outputs` を無視して結果をボタンの下に出すだけ)。
- **`table.groupBy`(#261)**: 同じ値のレコードを 1 行にまとめる集計表(「店ごとの平均点・件数でランキング」)。列は groupBy の field の列(グループ名・aggregate なし)+ `aggregate`(count / sum / avg / min / max)付きの列だけ。groupBy にできるのは string / enum / ref / boolean / integer / date。`sort.field` は集計列の field(同じ field の集計列が複数なら先頭)、集計列に無ければ groupBy の field = グループ名順。sort 省略 = 最初の集計列の降順、`limit` は上位 N グループ。値が空のレコードはどの行にも入れない。avg / min / max は数値が無ければ「—」、avg は小数 1 桁。iOS は `TableGrouping`(MetamonDSL)でクライアント集計(chart.groupBy と同じ)。新しい部品・Expr ではなく table の拡張にしたのは、(1) 表の見出し・行の描画をそのまま使える、(2) chart / stat と同じ aggregate 語彙で AI が書ける、(3) 古い iOS は `groupBy` と列の `aggregate` を無視して従来のレコード表として描く(落ちない・未知 type のプレースホルダにもならない)ため。マスタ entity の computed `agg` + table(人ごとの合計)も引き続き使える。実例: `examples/ranking.json`。
- **`chatThread`(#260)**: LINE のようなチャット。`entity` のレコードを 1 通 = 1 件の吹き出しで**古い順に上から下へ**積み(最新が一番下・開いた時も新着が来た時も一番下へスクロール)、**自分の分は右**(ブランド色)・**相手の分は左**(上に送信者名 = `$author`。同じ人が続けて送った 2 通目以降は省く)、下に送った時刻(`$createdAt`)。自分の判定は `Record.author` とサインイン中の uid の比較(#249 / #251 と同じ。author の無い未送信の自分の分と private の entity は全部自分)。**入力欄は部品の下端に固定**(複数行は 4 行まで伸びる・`photoField` があれば写真ボタン)で、送信 = `entity` に 1 件保存(ローカル DB に即反映して同期キューが送る・フォーム画面は作らない)。共有アプリへの初めての送信はフォームの保存と同じく表示名を聞く(#250)。他の人のメッセージは既存の records の購読(SSE `GET /events` → 再取得)で数秒以内に届く。器の中で自前でスクロールするので `GridScreenLayout.scrollsInside` には入れず、マイク FAB が入力欄の左に重なる時だけその分を空ける(`ChatInputInset`)。backend: `textField` は保存する string/text、`photoField` は photo、他の required field は defaultValue 必須(`Validate`)/ フォームが無くても「add form が無い」と言わない・`list` 扱い(`Lint`)/ private の entity は「誰にも見えない」警告。`list` の表示形式(例 `list.style:"chat"`)にしなかったのは、(1) 入力欄を持つ = 一覧 + フォームの 2 役で list の意味(表示だけ・追加はフォーム)を変えてしまう、(2) 下から積む・左右に振る・送信者名をまとめるは list の行テンプレ(title/subtitle/trailing)と共有できる部分が無い、(3) 部品が 1 つなら AI が「チャット = chatThread」と 1 語で選べ、iOS も 1 部品 = 1 ファイルで閉じる、の 3 点。iOS が知らない古いビルドでは未知の部品としてプレースホルダ(RELEASE は何も描かない)になり落ちない。実例: `examples/chat.json`。
- **`aiChat`**: クラウドの AI と何往復も話す画面(相談相手・家庭教師・会話の練習相手)。見た目は `chatThread` と同じ吹き出し(自分は右・AI は左に「AI」)と下に固定の入力欄。送ると **`POST /v1/text` の `task: "chat"`**(直近 20 発言・本文 16 KB まで・`persona` は `instruction`)で返事をもらい、**自分の発言と AI の返事を 1 件ずつ `entity` に保存**する(フォームを開かない・ローカル DB に即反映して同期キューが送る)。各発言の `roleField` に `user` / `assistant` を書く(役を並び順から決めないので、保存に失敗しても以後の役がずれない)。自分の発言を先に保存するので、返事に失敗しても発言は残り、次の送信で続けて読まれる。`roleField` の無い古い DSL(stg で作られたもの)は iOS が交互の並びで読み、返事が来てから 2 件続けて保存し、返事の保存に失敗したら自分の発言も消して並びを崩さない。端末内 AI は使わず**いつもオンライン**(Tailor It Pro・共有のアプリは作成者の Pro・AI 送信の同意が要る・利用上限の重さは long と同じ 3)。`persona` はアプリの作者が書く役と話し方で、サーバーは利用者の発言と同じ会話の中に `<persona>` で入れ、固定の安全の決まりより下に置く。backend: `textField` は保存する string/text、`roleField` は必須で string か user / assistant を含む enum、`persona` 必須(500 字まで)、他の required field は defaultValue 必須、`entity` は private だけ(shared / public はエラー。会話は本人だけのもので、AI が読む履歴も本人の会話だけになる)(`Validate`)/ フォームが無くても「add form が無い」と言わない・`list` 扱い(`Lint`)。iOS も履歴を組む時は自分(または作者の無い未送信)の記録だけを使う。審査用の版(オンライン AI が無い)と古い版には出さない。AI の吹き出しにはどれも小さな読み上げボタン(`speakButton` と同じ端末の声・`LeafSpeech`・もう一度押すと止まる・サーバーに送らない)が付き、`speak: true` なら新しい返事が届くたびに自動で読み上げる(声で聞く相談・会話の練習。録音の間は読まない・画面を離れたら止める・消音スイッチの扱いも `speakButton` と同じ)。実例: `examples/ai_counselor.json`。
  - **話しかけて記録する(`saveTo: {entity, fields}`)**: 「今日カレー食べた」と話すと、AI が返事と一緒に記録の案を返し、iOS は AI の吹き出しの下に小さなカード「この内容で記録しますか?」(値の一覧 + **追加** / **直す**)を出す。**追加**を押した時だけ `entity` に 1 件足す(`createRecord` = フォームの保存と同じ経路なので、共有データの決まり・プランの上限・表示できない言葉はいつもどおり)。**直す**はカードを閉じて入力欄に戻り、会話で言い直す(AI が新しい案を出す)。勝手には保存しない・カードは最新の返事の分だけ(会話の記録には残さない)。`fields` は AI が埋める field id 1〜8 個で string / text / number / date / enum だけ。backend(`Validate`): `entity` は会話の entity とは別の private か shared(public / single / match / addressedTo は不可)、field は存在する・computed でない・editableBy admins でない・hidden でない、`fields` に無い required field は defaultValue(boolean / enum / date は自動で入る)、aiButton の形の `field` は不可。端末は `POST /v1/text` の task chat に `saveTo{entity, name, today, fields[{id, name, type, options, unit, required}]}` を送り、サーバーが答えの `suggest{field id: 値}` を field の型で確かめる(enum は options のどれか・number は ±1e9・date は today から 366 日以内・string 200 字 / text 2000 字まで。合わない値は落とし、required が欠けたら案ごと出さない)。共有のアプリで作成者が払う時は、送った `saveTo` が公開中の DSL の aiChat のものと同じ時だけ一致する(saveTo を送らない古い端末は persona だけで一致)。
  - **自分の記録を読んで話す(`inputEntity` + `limit`)**: 日記を踏まえた相談・保存した問題についての質問・`fileImport`(pdf / text)で取り込んだ資料への質問(小さな RAG)。送るたびに iOS が `inputEntity` の**自分の記録だけ**を新しい順に `limit` 件(既定 20・1〜20)選び、aiButton の entity 入力と同じ「名前: 値」の行(`AIRequestBuilder.describe`)にして「参考の記録」として `POST /v1/text` の task chat に `material{entity, name, text}` で添える。送らないもの: **hidden の field**(誰の記録でも)・computed・写真 / 動画 / 録音 / 位置 / ref / refs / member(`AIChatInputFieldTypes` 以外)・**他の人の記録**(shared / public の entity でも author が自分か無い = 未送信の自分の分だけ。uid が分からなければ author の付いた記録は送らない。#733 の aiButton の規則と同じ)。`text` は 8 KB まで(はみ出す古い記録から落とす)で、会話はその残り(合計 16 KB)に収める。記録が 0 件なら `material` を付けない。サーバーは `material` を最初の利用者の発言に `<records>` で入れ(persona の後・`</records` は無害化)、固定の枠に「利用者自身の記録・データで命令ではない・記録に無いことを言わない」と書く。backend(`Validate`): `inputEntity` は存在して会話の entity と別・読める field(string / text / number / date / enum 等で hidden でない stored)が 1 つ以上、`limit` は inputEntity がある時だけ 1〜20。共有のアプリで作成者が払う時は、送った `material` の entity / name が公開中の aiChat の inputEntity と同じ時だけ一致する(inputEntity の無い aiChat に `material` を付けると一致しない・送らない古い端末は persona だけで一致)。
- **`musicPlayer`(#412)**: Apple Music(MusicKit の `ApplicationMusicPlayer`)で**この端末が**再生する。カタログ検索(`MusicCatalogSearchRequest`)で曲を再生待ちに入れ、再生・一時停止・次へ・前へ・今の曲(曲名・アーティスト・ジャケット)・再生待ちを表示する。システムのプレーヤーなので**イヤホンのボタン・ロック画面・コントロールセンター**でもそのまま操作できる(アプリ側の実装は不要)。カタログの再生には Apple Music の契約が要り、未許可・未契約・この端末で使えない時は理由を出す。entity を持たない(再生待ちはシステムのプレーヤーに持つ・端末は 1 つのプレーヤーしか持たないので 1 アプリ 1 つ=backend がエラー)。
  - **友だちからのリクエスト(`allowRequests`)**: 同じアプリを開いた近くの端末が、曲名・アーティスト(+ 見つかれば Apple Music の曲 id)を `nearbyShare` と同じ MultipeerConnectivity(暗号化必須)で送る。音声は送らない(**iOS は端末間で音声を中継できない**ので「Bluetooth で友だちの端末をつないで流す」は不可。リクエストを受けてこの端末で再生する形で代える)。受けた側は前面で今のアプリが `allowRequests` の musicPlayer を使っている間だけ待ち受け、届いたリクエストを一覧に出して「追加 / 見送る」を選ぶ(`autoAccept` なら確認せず再生待ちへ・運転中向け)。リクエストは 1 件ずつ・曲名 / アーティストは各 100 文字・id は 40 文字までに切り、未承認は 20 件まで(超えたら古い順に捨てる)。審査用ビルド(`APP_STORE_REVIEW`)では `nearbyShare` と同じくリクエスト部分をバイナリから外し、再生だけ残す(`docs/ios/review-build.md`)。
  - **Spotify は開くだけ**: Spotify の iOS SDK(App Remote)は使わない(Spotify の開発者登録・クライアント ID・Premium・利用者数の制限と審査が要る)。リクエストの行に「Spotify で探す」(`https://open.spotify.com/search/<曲名 アーティスト>`・Spotify アプリがあればアプリで開く)を出すだけで、再生・操作はできない。AI にもそう答えさせる(`backend/internal/ai/music_rules.go`)。実例: `examples/drive_music.json`。

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
| `video` | `"video:<id>\|data:image/jpeg;base64,<先頭フレーム>"`(本体は録画した端末の中) |
| `audio` | `"audio:<id>\|<秒>"`(本体は録音した端末の中) |
| `location` | `"lat,lng"`(P1) |
| `ref` | 参照先(`refEntity`)のレコード id 文字列。入力は picker |
| `refs` | 参照先(`refEntity`)のレコード id の配列。入力は複数選択の picker |
| `member` | 参加者の uid 文字列。scope `shared` だけ(§3.13) |
| `members` | 参加者の uid の配列。scope `shared` だけ(§3.14) |
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
| 上限(極端な値) | `area` の行は 48 まで・入れ子は 2 段(Screen > Container > Leaf)まで。どちらも backend が断る。iOS も二重に守り、48 行を超える行は 48 に丸め、8 段より深い `children` は読まず描かない(空の枠) |
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
- **`shared` の参加者(#254)**: 上の表の「v1 では public と同じ扱い」は #254 で変わった。`shared` の records は**作成者とシェアリンク / QR で参加した人だけ**が読み書きでき、それ以外は 403(`details.reason: not_member`)。置き場は public と同じ `apps_public/{appId}`。参加は `POST /shares/{token}/import`、退出・一覧は `/apps/{appId}/members`。家族・サークル・友達・チームは `shared`、不特定多数(掲示板・ランキング)は `public`。詳細と移行は [features/members.md](../backend/features/members.md)。
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

#### フォロー `myFollows`(#270)

「フォローした人の投稿だけが流れるタイムライン」。フォローは**アプリごとの人と人の関係**で、appconfig が持つ(DSL の entity ではない)。DSL は読むだけ: 一覧の絞り込みと、投稿の画面のボタン。public / shared の entity だけ。実例: `examples/timeline.json`(ひとことタイムライン)。

| 書く場所 | 形 | 意味 |
|---|---|---|
| 部品の `filter` | `{"field":"$author","op":"eq","value":{"myFollows":true}}` | 自分がフォローしている人の投稿だけ(`op:"neq"` = フォローしていない人の投稿。自分の投稿はどちらでも「フォローしていない人」側)。list / table / timeline / stat(件数)/ chart で使え、agg の中では使えない |
| Action `follow` / `unfollow` | `{"type":"follow","entity":"posts"}` | 今開いている投稿(`list.onItemTap` で開いたその entity の画面)を書いた人をフォローする / 外す。button はフォロー済みなら `follow`、まだなら `unfollow` を隠し、自分の投稿では両方隠す |

- follow / unfollow を Leaf ではなく Action にしたのは `export` と同じ理由(button の見た目と `confirmMessage` を使える・toolbar にも置ける・古い iOS は「未対応のアクション」の toast で済む)。
- **プロフィール**は投稿者の表示名(`$author`・各自がアプリの設定で決めた名前)。アイコン・自己紹介・人ごとのプロフィール画面は今は無い(参加者の情報に写真が無いため)。
- **見える範囲**: フォローは本人だけが見られる。フォローされた人には知らせず、フォロワーの一覧・人数も出さない(つきまといに使われにくくする)。scope:shared だけのアプリでは参加者だけがフォローでき、フォローできる相手も参加者だけ。
- appconfig: `GET /v1/apps/{appId}/follows`(`{uids}`)・`PUT` / `DELETE /v1/apps/{appId}/follows/{uid}`(冪等・`{following, count}`)。一覧の API は `filterField=author&filterOp=in&filterValue=myFollows`(`nin` も)。Firestore は `apps_public/{appId}/follows/{uid}`(`{uids: [...]}`・1 人 1 文書)。1 人が 1 アプリでフォローできるのは 500 人まで。
- **Firestore の `in`(30 件まで)は使わない**: records は今も 1 entity 2000 件までを読んでメモリで絞っている(§3)。フォローの一覧は 1 文書なので 1 回の読み取りで、何人フォローしても同じ。件数が増えて records をクエリで絞るようになったら、30 人ずつの分割か、投稿時に配る(自動の動き・#341)を選び直す。
- iOS: 起動時にこのアプリのフォロー一覧を読み、`{"myFollows":true}` を uid の集合に置き換えて端末で絞る(`$author` の `{"me":true}` と同じ所)。ボタンは押した瞬間に一覧へ反映し、失敗したら戻して理由を出す。読み込む前・オフラインで一度も読めていない時は、フォロー中の一覧は空。
- AI: 「フォロー」「フォロー中」「好きな人の投稿だけ」はこの filter とボタン。フォロー用の entity・相手を選ぶ ref は作らない。ai-eval の `followFilter` / `followButton` / `followEntity`(`follow_cases.json`・`m0disc_cases.json` d_sns_follow)。
- 古い iOS(この機能の前の版)は `{"myFollows":true}` を読めずに filter ごと捨てるので、その一覧は全員の投稿になり、ボタンは押すと「未対応のアクション」になる(落ちない)。

### 3.6 ナビゲーション = 画面グラフの規則(#112 #114 #115)

画面を頂点、`navigate`(button / toolbar / `list.onItemTap` / `calendar.onDateTap` / `mapView.onItemTap`)と `saveRecord.screen` を辺とする有向グラフで考える。根はタブがあれば `app.tabs[].screen`、無ければ `initialScreen`(タブがある時に `initialScreen` がタブの根でなければ iOS は先頭タブを開く)。実装は `backend/pkg/dsl/nav.go`(`NavEdges` / `Reachable` / `Unreachable` / `HasNav`)。

| 規則 | 扱い | 理由 |
|---|---|---|
| `navigate.screen` / `saveRecord.screen` / `app.tabs[].screen` / `initialScreen` が存在する画面 id | **エラー**(`Validate`。メッセージに使える画面 id の一覧と直し方を入れ、AI の再生成プロンプトにそのまま渡す) | 押すと何も起きない・描画できない |
| `app.tabs` は 0 個(省略)か 2〜5 個 | **エラー** | タブ 1 個は TabView として意味が無い・6 個以上はタブバーに入らない |
| どの根からも辿れない画面 | 警告(`Lint`)。フォームは追加ボタンや `list.onItemTap` から辿れれば可(entity のフォームが 1 つも辿れない時は従来の「form screen is not reachable」1 件だけ) | 表示はできるので保存済み DSL を壊さない。声で直せばよい |
| タブがあるのに `initialScreen` がタブの根でない | 警告 | 起動画面が意図と違う |
| `fullScreen` で開く画面に戻る手段(`back` か、遷移先指定の無い `saveRecord`)が無い | 警告 | iOS は fullScreenCover に閉じるボタンを自動で付けないので閉じ込められる。push は OS の戻る、sheet は下スワイプで戻れるので対象外 |
| 同じ画面の toolbar(右上の＋)と `button` が、同じフォーム画面(`screen.entity`)を同じ `prefillRef` で開く(#416) | 警告(`Lint`)。生成時は toolbar 側を消す(`dropDuplicateAddToolbar`)。iOS は保存済み DSL でも toolbar 側を出さない(`DuplicateAddControl`) | 同じ「追加」が 2 つ並ぶ。押しやすい右下の button を残し、button が無い画面だけ toolbar の＋にする |

- 警告は `Meta.lint` に載るだけで再生成は起こさない(`AI_LINT_RETRY=1` の時だけ 1 回再生成)。
- AI 側の基準(`internal/ai/prompt.go`): 役割の違う一覧・集計・設定はタブ(2〜4 個)、入力はフォーム画面を sheet、詳細は `list.onItemTap` で push、1 画面の直下は 8 部品まで、孤立画面を作らない。navigate はどこからどこへ飛んでもよい前提(タブの根へ飛んだらタブ切替は iOS 側 #113)。
- 遷移を変える編集は JSON Patch のレシピで出す: 「B から C にも」= B の components に navigate C の button を add、「タブを足す」= `/screens/-` + `/app/tabs/-`(タブが無ければ `/app/tabs` を 2 個で add)、「タブを消す」= `/app/tabs/i` を remove(残り 1 個なら `/app/tabs` ごと remove し、辿れなくなる画面も remove)、「順番」= `/app/tabs` の move。eval は `cases.json` の `group: nav`(`expect` の `patch` / `unreachable` / `nav` / `tabs` / `tabOrder` / `tabHas`)。

### 3.7 entity.retentionDays = 記録の保存期間(#482)

「1週間たった記録は消して」「30日で消えるようにして」で、その entity に `"retentionDays": 7` のように日数を付ける。省略 = 消さない。

| 項目 | 決めたこと |
|---|---|
| 数え方 | レコードの `createdAt`(作った日時)から。日付の項目の値ではない。`createdAt` が無い・読めないレコードは消さない |
| 消す時機(iOS) | DSL を当てた時(起動・切替・生成・ホットリロード)・前面に戻った時・サーバーから読み直した後・保存の後。サーバー側の定期削除は無い(誰も開かなければ残る) |
| 消し方 | 手で消すのと同じ削除(ローカル DB から即消し、同期キューが appconfig へ `DELETE`)。消えた記録は戻せない |
| 確認 | 期間が付いた・短くなった時に端末が確認ダイアログ(「自動で消す」「自動で消すのをやめる」「あとで」)を出し、**確認するまで消さない**。確認した期間は端末・appId・entity ごとに保存。延ばしただけなら確認し直さない |
| 共有(shared / public) | **各自が自分で作った記録だけ**を自分の端末で消す(`author` が自分の uid と一致するもの。author の無い旧データ・uid 不明は消さない)。他の人の記録は、その人が開いた時にその人の端末で消える。他人の記録を消す権限は誰にも足さない |
| 表示・解除 | 設定シートの「記録の自動削除」に entity ごとの期間。「自動で消すのをやめる」は声の「自動で消すのをやめて」と同じ修正(remove `/entities/<i>/retentionDays`)を送る |
| 付ける entity | 日々たまる記録だけ。ref で選ばれる一覧(カテゴリ・メンバー)と設定には付けない(AI のルール) |

- AI: `backend/internal/ai/retention_rules.go`。一括削除の制限(chatLimits)と混同して断らない。ai-eval の `retention`(日数・0 = 無し)/ `retentionOnRef`(`retention_cases.json` / `retention_chat_cases.json`)。
- 古い iOS(#302): v1.0 は `retentionDays` を知らない(無視して消さない)。iOS は generate / chat の body `supportedFeatures: ["recordRetention"]` で実装済みを伝え、`allowedComponents` を送るのに `recordRetention` を含めない版には AI が使わない(§5.2・§5.3)。審査用の版も記録の自動削除を持つ(端末内の処理だけで、共有もネイティブ API も使わない)。

### 3.8 Web の共有ページで描ける部品(#362・#351 の Web 列)

`GET /s/{token}`(appconfig・認証不要)は、シェアのスナップショット DSL を**読むだけ**で描く(Go の `html/template`・JavaScript なし。実装 `backend/internal/appconfig/shareview.go`)。**レコードは出さない**(#253): データを使う部品は空の状態(「—」・`emptyText`・「記録はアプリで見られます」)だけ。グリッド(§3.4 の `area`、card / section / vstack のローカルグリッド)はそのまま 6 列の CSS grid に置き、`area` 無しは下に積む。

| 区分 | 部品 | Web での見え方 |
|---|---|---|
| 描く | `vstack` `hstack` `section` `card` `disclosure` `carousel` `grid` | 並び・見出し・枠(disclosure は `<details>`、carousel は横スクロール) |
| 描く | `text` `label` `markdown` | 文字(`style` title / headline は見出し。markdown は記法を解釈せず文字のまま) |
| 描く | `badge` `divider` `spacer` `emptyState` `icon` | 文字・線・余白(icon は丸い印だけ。SF Symbols は出さない) |
| 描く(空) | `stat` `healthStat` `progressBar` `gauge` | ラベル + 「—」/ 空のバー |
| 描く(構造だけ) | `list` `timeline` `table` | 行に出る項目名(list の title / subtitle / trailing、table の列名)+ 空の表示 |
| 代わりの表示 | `flashcards` | 「アプリで見られます」の枠に「カード(表: 項目名 / 裏: 項目名)」を添える(カードの中身は出さない・#264) |
| 描く(見本) | `chart` | 固定の見本の棒 / 円(SVG・データではない)+「グラフはアプリで見られます」 |
| 描く(押せない) | `textField` `numberField` `datePicker` `timePicker` `picker` `toggle` `slider` `stepper` `textEditor` `secureField` `colorPicker` `rating` `searchBar` | `disabled` の入力欄(picker は enum の選択肢、ラベルは無ければ項目名) |
| 描く(押せない) | `button` `shareButton` `openUrlButton` `photoField` `videoField` `drawingField` | `disabled` のボタン。URL は出さない |
| 描く(リンク) | `link` | `https://` の時だけリンク(`rel="nofollow noopener noreferrer ugc"`)、それ以外は文字だけ |
| 代わりの表示 | `image` `avatar` `videoPlayer` | 「写真」の枠 |
| 代わりの表示 | 上以外すべて(`voiceInput` `calendar` `gridTable` `mapView` `qrScanner` `qrCode` `transcribeField` `speakButton` `timerDisplay` `webView` `settlement` `randomPicker` `fileImport` `aiButton` `chatThread` `dashcam` `sensorValue` `musicPlayer` など)と未知の type | 「アプリで見られます」の枠(`title` があれば添える) |

- 画面の順: タブの順 → `initialScreen` → 残り。上限 12 画面・400 部品・200ms・出力 512KB(超えたら「続きはアプリで見られます」/ 名前と導線だけに落とす)。
- 公開レコードを見せる口: `RecordPeek`(`shareview.go`)。今は常に nil。実装しても渡るのは `scope:"public"` の entity だけ(private / shared は渡さない)。見せるかどうかは #253 の判断待ち。
- 部品を足した時: この表に行を足す(何もしなければ「アプリで見られます」になるので壊れない)。表と実装の一致は `TestShareViewMatchesDocTable` がこの表を読んで確かめる。
- 決まった文言(「共有」・「カード(表: … / 裏: …)」・空の表示など)は閲覧者の言語(日本語 / 英語・#278)で出す。DSL に書いた文字はそのまま。
- 見られない時: 無効・取り消し済み・運営が非公開にしたアプリ(#321)・`/s/` の下の別の形のパスは、どれも同じ HTML の「リンクが無効です」(404・`Cache-Control: no-store`)。非公開かどうかを確かめられない時も出さない。API(`/v1/shares/{token}`)は今までどおり JSON のエラー。
- 適合テスト(#352)のうち、この表の「描く」部品だけの case は `TestShareViewConformanceCases` が Web でも描き、iOS の expected.json の画面の見出し・部品の見出し・項目名・列名がページに出ることを確かめる(値は比べない・記録は出さないため)。

### 3.9 entity.match = 仲間探しのアプリ(#507)

趣味・勉強・イベントの仲間をプロフィールから探し、**お互いに承認した人とだけ**つながるアプリ。**恋愛・出会い目的は対象外**(AI が断る)。設計・API・安全の規則は [`docs/backend/features/matching.md`](../backend/features/matching.md)。

| 値 | 意味 | 制約(`Validate()`) |
|---|---|---|
| `profile` | みんなが見て「つながりたい」を送る 1 人 1 件のプロフィール。お互いにブロックしている人(どちら向きでも)の分は返らない | scope public / shared・アプリに 1 つまで・location / photo / video の field は不可・edit は書かない |
| `matched` | 書いた本人と、つながった相手だけが読める(連絡先・次の相談) | scope public / shared・profile の entity が要る・location の field は不可・edit は書かない |

- 誰が誰に送ったか・つながったかはレコードではなく appconfig の `/apps/{appId}/interests`・`/matches` が持つ。見える範囲は**公開 DSL(`apps_public/{appId}`)の match** で決める(各自のコピーではない)。
- 部品(phase 2):
  - **`list.item.interest: true`**: match "profile" の一覧の行に「つながりたい」ボタン。自分の行には出さない。送ると「送りました」、お互いに送ると「つながりました」。初めて送る時に「18 歳以上です」の確認を出し(端末に覚える・年齢は保存しない)、`ageConfirmed: true` を送る。オフラインでは送れない。
  - **`matchList`**{`entity`(プロフィールの entity), `show`}: `incoming` = 届いた人(「承認」「見送る」)/ `outgoing` = 送った人(「取り下げる」)/ `matches` = つながった人(長押しで「つながりを解除」)。どれも長押しで通報・ブロック(#321 の `ModerationHost` をプロフィールのレコードで使う)。行はプロフィールのレコード(`profileRecordId`)で描き、見つからない時は「プロフィールが見つかりません」。
  - どちらも楽観的に反映し、失敗したら戻して理由を出す。承認・解除の後は match "matched" の entity(連絡先)を読み直す。
  - Action は足していない(送る・承認は部品の中で完結。`shareButton` と同じ)。`Validate()`: `interest` は match "profile" の list だけ・`matchList.entity` は match "profile"・`show` は 3 値・`item` は title / subtitle / trailing / icon だけ。
- 版: iOS は `supportedFeatures` に `matching` を送る(審査用の版は送らず、`matchList` も `allowedComponents` から外す)。`matching` を送らない版・共有のデータを外した版には AI が match・`item.interest`・`matchList` を使わない(§5.3 と同じ仕組み・`restrict.go`)。古い iOS は未知のキー(`match`・`item.interest`)を無視し、未知の type(`matchList`)はプレースホルダで描く。

### 3.10 通知の when = 条件で知らせる(#265)

「在庫が少なくなったら教えて」「予算を超えたら通知」「目標に届いたら教えて」は、`app.notifications[]` に `when`(真偽の Expr)を書く。

```json
{"id":"low","title":"在庫が少なくなりました","body":"5 個を下回った商品があります","repeat":"once",
 "when":{"op":"gt","args":[{"agg":"count","entity":"items","filter":{"field":"stock","op":"lt","value":5}},0]}}
```

| 項目 | 決めたこと |
|---|---|
| 引き金は 1 つ | `when` があればそれだけが引き金。**時刻の通知(`time` / `weekday`・`repeat` daily / weekly)とレコードの日付の通知(`entity` / `field` / `offsetMinutes` / `titleField` / `enabledField`)とは組み合わせない**。`repeat` は `once` だけ(backend が拒否)。`scheduleNotification` には書けない(`app.notifications` だけ) |
| 式 | `stat.value` と同じ画面の式(行が無い: `ref` 不可・記録は `agg`)。結果は真偽(`lt` / `gt` / `gte` / `eq` / `and` …)。評価できない(nil)は偽 |
| 鳴る時 | 前回の評価が**偽 → 今回が真**の時に 1 回だけ、すぐ(約 1 秒後)にローカル通知。真のままの保存では鳴らない。偽に戻ってからまた真になれば鳴る。通知を足した直後の最初の評価は状態を覚えるだけ(足した時点で満たしていても鳴らさない) |
| 評価する時機(iOS) | **この端末で**レコードを保存・削除した直後と、同期(サーバーから読み直し・リアルタイムの変更)の後。DSL を当てた時にも評価して状態を覚える。読み込んでいない entity は端末内 DB の値で評価する |
| 状態 | 前回の真偽を端末の UserDefaults に appId × 通知 id ごとに保存(アプリを閉じても 1 回きりを守る)。DSL から消えた通知・アプリの削除で消える |
| 許可 | 通知が許可されていなければ出さない(状態は更新する。後で許可しても過去の分はまとめて出さない) |
| **他の人の操作(M4)** | 共有(shared / public)の entity で、**他の人の記録だけで条件を満たした時にすぐ届けることはできない**。この端末がアプリを開いて同期した時に評価されて届く。端末を開いていない人にも届けるのはサーバー側の評価(M4)でやる |

- AI: `backend/internal/ai/notify_when_rules.go`(在庫の閾値・予算オーバー・目標達成のレシピ)。ai-eval の `notifyWhen` / `notifyWhenOp`(`notify_when_cases.json` / `notify_when_chat_cases.json`、`m0disc_cases.json` の `d_inventory`)。
- 古い iOS(#302): `when` を知らない版は `when` を無視し、残りの `{repeat: once}` を**次の 9:00 の 1 回きりの通知として予約してしまう**(開くたびに予約し直すので、ほぼ毎日鳴る誤った通知)。そこで iOS は `supportedFeatures` に `notification.when` を入れ、`allowedComponents` を送るのにこれを含めない版には AI が `when` を使わない(生成されたら通知ごと取り除く・§5.3)。保存済みの DSL を古い版で開いた時の誤った通知は防げないので、`MIN_IOS_BUILD` を上げるまでの既知の制限。
- 新しい版で `when` が読めない(未知の式の形)時は、その通知ごと落とす(`Lossy`)。時刻の通知として鳴らさない。

### 3.11 entity.single = 設定値の置き場(#267)

周期の日数・目標・月の予算のように**アプリ全体で 1 つだけの値**は、`"single": true` の entity(例 `settings`)に置く。「設定を追加」でレコードを増やし最新の 1 件を `latest` で読む形はやめる。

```json
{"id":"settings","name":"設定","single":true,"fields":[{"id":"cycle_days","name":"周期","type":"integer","defaultValue":28,"unit":"日"}]}
```
次の予定日 = `{"op":"addDays","args":[{"agg":"latest","entity":"periods","field":"start_on","by":"start_on"},{"setting":"cycle_days"}]}`。

| 項目 | 決めたこと |
|---|---|
| レコード | 常に 1 件。保存前は「defaultValue が入った空の 1 件」として扱う(式も初期値を読む) |
| フォーム | `screen.entity` が single の画面は、開くと**既存の 1 件の編集**(無ければ初期値の新規)。保存は追加でなく上書き。削除ボタン・「続けて追加」は出さない。開き方は toolbar / button の `navigate`(recordId は要らない) |
| 読む | 式 `{"setting":"<fieldId>"}`(expressions.md §2)。行の無い所(`stat.value`・`progressBar.value`・通知の `when`)でも、別の entity の computed からでも読める。1 つの field id は 1 つの single entity にだけ置く |
| 使えない所(backend が拒否) | single の entity を `list` / `table` / `stat`・`progressBar` などの `entity`、式の `agg`、ref / refs の `refEntity`、`deleteRecord` / `export` に使う。`scope` shared / public・`retentionDays`・`match`・`hidden` の field |
| 保存の形 | appconfig は single の entity の `POST .../records` を **clientId によらず固定の doc id**(`IDFromClientID("single")`)に置き、既にあればその data で上書きする(201 でなく 200)。private の entity だけなので置き場は利用者ごとに 1 つ。iOS の作り方・同期は他の entity と同じ(2 台が同時に初回保存しても同じ doc に入り、後の保存が勝つ) |
| 2 件以上ある時 | (古い版が「追加」した等)`createdAt` が最も新しい 1 件(同じなら後の方)を使う。式・フォームとも同じ |

- AI: `backend/internal/ai/single_rules.go`。ai-eval の `single` / `setting`(`single_cases.json`、`m0disc_cases.json` の `d_period`)。
- 古い iOS(#302): single を知らない版は、設定のフォームを**開くたびに新しい 1 件を追加**し(データが増える)、`{"setting":...}` を含む computed を読めない(その field が入力欄として出る・`stat.value` は「—」)。そこで iOS は `supportedFeatures` に `singleEntity` を入れ、`allowedComponents` を送るのにこれを含めない版には AI が single / setting を使わない(残れば single を外し、setting を `{"agg":"latest","entity":…,"field":…}` に置き換える・§5.3)。保存済みの DSL を古い版で開いた時は防げないので、`MIN_IOS_BUILD` を上げるまでの既知の制限(backend が固定の doc id に上書きするので、サーバー上の件数は増えない)。

### 3.12 serverRules = サーバーの自動の動き(#340・M4)

「誰かが出欠をつけたら、予定を作った人に通知」のような、**端末を開いていない人にも届く動き**をアプリのトップの `serverRules[]` に宣言する(方式 A = 宣言だけ・AI がコードを書かない。設計は `docs/decisions/336`〜`339`)。**iOS は描画しない**(未知のキーとして無視する)。動かすのはサーバーの runner(#341・#342)。

```json
{"id":"notify_rsvp","on":{"record":"created","entity":"rsvps"},
 "if":{"op":"eq","args":[{"ref":"answer"},"yes"]},
 "do":[{"notify":{"to":{"kind":"authorOf","field":"event"},"title":"出欠","body":"{$author}さんが参加にしました"}},
       {"increment":{"ref":"event","field":"yes_count"}}]}
```

| 部分 | 語彙 | 決まり(`Validate()`・`backend/pkg/dsl/server_rules.go`) |
|---|---|---|
| きっかけ `on` | `record`: `created` / `updated`(+ `entity`、更新は `fields` で「この field が変わった時」に絞れる)。`every`: `daily` / `weekly`(`weekday`)/ `monthly`(`day` 1〜28)+ `time` HH:MM(作成者のタイムゾーン)。時刻のきっかけは `each`: `<entity>` でそのレコード 1 件ずつに動かせる | どちらか 1 つ。削除のきっかけ・1 日 1 回より細かい時刻・外からの呼び出しは無い。`each` は時刻のきっかけだけ・ルールと同じ側の entity だけ |
| 条件 `if` | Expr(行 = きっかけのレコードか `each` のレコード。`"<refField>.<field>"` で ref 先、更新では `"$prev.<field>"` で前の値)。`agg` で同じアプリの記録を集計できる(下の「記録を読むルール」) | 真偽であること。`streak` / `doneInDays`・computed field は使えない。`each` の無い時刻のきっかけには行が無い(`ref` 不可・`agg` は可) |
| 突き合わせ `lookup` | `{op: exists / notExists, entity, field, equals}`: 別の entity に、`field` の値が行の `equals` の値と同じレコードがある / 無い時だけ動く(`if` と両方)。`{op: overlaps / notOverlaps, entity, field, fieldEnd?, equals, equalsEnd?}` は期間(date / datetime)が重なるレコードがある / 無い時。下の「記録を読むルール」 | 1 ルールに 1 つ・等しいか、期間が重なるかだけ。行が要る(`each` の無い時刻のきっかけは不可) |
| 似ている記録 `similar` | `{entity, fields, min?}`: `fields` の値が行と同じ数(共通点)が一番多いレコードを探し、`min`(省略 = 1)以上の時だけ動く。`do` で `$similar`・`$similar.<field>`・`$similarScore` を読める。下の「記録を読むルール」 | 1 ルールに 1 つ・`fields` 1〜5 個。行が要る。`if` では結果を読めない |
| おすすめ `pick` | `{entity, order: random / oldest / newest, filter?, exclude?}`: `entity` のレコードから 1 つ選び、選べた時だけ動く。`do` で `$pick`・`$pick.<field>`・本文の `{$pick.<field>}` を読める。下の「記録を読むルール」 | 1 ルールに 1 つ・`similar` とは一緒に書けない。行は要らない(`each` の無い時刻のきっかけでも可)。`if` では結果を読めない |
| 割り当て `pick` の `count` | `order: fewest / most` + `count {entity, field}`: 候補(`pick.entity`)ごとに `count.entity` のレコードが `field`(ref / refs)で指している数を数え、一番少ない(多い)ものを選ぶ。同じ数なら先に作った候補(順番に割り当てると交代になる)。`most` は 1 つも無い候補を選ばない。`exclude.match {field, equals}` で `exclude` を行と値が同じレコードだけに絞る(その日に休みの人)。`do` で `updateRecord` の `{"ref":"$pick"}` を行の ref field に書く(`backend/pkg/dsl/server_rules_assign.go`) | 行のレコード自身と、その実行が(`each` ではどの行のためにでも)作ったレコードは数えない(やり直しで同じ候補)。`each` では同じ実行で先に割り当てた行も数える。レコードのきっかけでは runner がイベントを順に 1 件ずつ(前の書き込みまで終えてから)動かすので、続けて保存したレコードは前の割り当てを数える。ただし runner のインスタンスが 2 つ同時に tick した時など、ほぼ同時の保存は互いの割り当て前を数えて同じ候補になることがある((app, rule) ごとの直列化はしていない)。`count.entity` は同じアプリ・同じ側(宛先付き・hidden・computed は不可)。`count` のあるルールは entity を 3 つまで読める(読む件数の上限・1 日の上限はそのまま) |
| 処理 `do`(1〜5 個) | `notify`{`to`, `title`, `body`} / `createRecord`{`entity`, `values`} / `updateRecord`{`ref`?, `values`} / `increment`{`ref`, `field`, `by`?} | 1 つのオブジェクトに 1 種類だけ。書けるのはきっかけと同じ側の entity(private どうし・shared / public どうし)。computed・photo・video・location の field には書かない。createRecord は required の field を全部書く |
| 宛先 `to.kind` | `owner`(作成者)/ `author`(きっかけのレコードを書いた人)/ `authorOf` + `field`(ref 先のレコードを書いた人)/ `members`(参加者全員)/ `recipients` + `field`(宛先付きの記録の宛先の人・§3.14) | private は `owner` だけ、**public は `owner` と `authorOf` だけ**、shared は全部。きっかけを起こした本人には送らない。任意の uid・メール・電話・外部の宛先は無い |
| 通知の本文 | 決まった枠 + `{<field>}`(きっかけ・`each` のレコードの値)・`{$author}`(書いた人の表示名・shared / public)・`{<名前>}`(`with` の数・日付)・`{$pick.<field>}`(`pick` で選んだ記録の値) | **題 30 字・本文 80 字**(差し込みの `{…}` も数える)。差し込む値は実行の時に 30 字で切る。URL・ドメイン・メールアドレス・電話番号・「公式」「運営」などの名乗り・制御文字・見えない文字は不可。ref・refs・photo・video・location・hidden・computed の field は差し込めない |
| 連鎖 | 処理が書いたレコードは次のきっかけを起こさない | 別のルール(自分も含む)のきっかけになる書き込み(作るなら created、書き換え・increment は同じ field の updated)を断る |
| 上限 | ルール 20・時刻のルール 5・処理 5(Pro の値)。1 つのルールが読む entity(`each` + `agg` + `lookup` + `similar` + `pick`)は 2 つまで・`with` は 3 つまで | 無料の値(3・1・3)は `App.ServerRuleQuotaProblems(pro)` で、プランが分かる所(appconfig)が見る。読む件数の上限は runner が持つ(下) |
| 使えないアプリ | 仲間探し(`entity.match`)のアプリ | `serverRules` ごと断る |
| single の entity(§3.11) | 「常に 1 件」なので `createRecord` の行き先にできない。きっかけにするのと、きっかけの 1 件の `updateRecord`(上書き)はよい | `createRecord` の entity が single なら断る |
| `{"setting":…}` | 設定は利用者ごとの値で、どの人の設定かが決まらないので、条件・値の式で使えない(private のルールでも) | 式に `setting` があれば断る |

#### 記録を読むルール(集計・まとめ・期限・順位)

決定 #336 で「後で(費用の単位を決めてから)」とした `agg` の条件と、定期の集計のやり直しを、読む件数の上限つきで入れた。外部への送信・任意の宛先・処理の連鎖は今までどおり無い。

| 語彙 | 中身 | 例 |
|---|---|---|
| `agg` を `if` / `values` / `by` / `with` で | 同じアプリの、**ルールと同じ側**(private どうし・shared / public どうし)の entity のレコードを集計(count / sum / avg / min / max / latest / earliest・filter の期間 `thisWeek` などや比較)。filter の値の `ref` は行のレコード | 「今月の支出が 5 万円を超えたら知らせて」: `if` に `{"op":"gt","args":[{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"date","op":"thisMonth","value":null}},50000]}` |
| `on.each`(時刻のきっかけ) | その entity のレコード 1 件ずつにルールを動かす(行 = そのレコード)。`if` で絞り、`{<field>}`・`author`・`updateRecord`(ref 省略 = そのレコード)・`increment` が使える。値が変わらない `updateRecord` は書かない | 締め切りの 3 日前: `{"every":"daily","time":"08:00","each":"tasks"}` + `if` `daysUntil(due) == 3`。順位: `each` + `updateRecord` `rank = count(score > 自分の score) + 1` |
| `notify.with` | `{名前: 数か日付の Expr}`(3 つまで)。本文に `{名前}` で差し込む。文字の値は不可。数は利用者の値から来ることがあるので、差し込む時に電話番号らしい桁(10 桁以上)を伏せる | 毎週のまとめ: `"body":"今週は {count} 件・{total} 円でした"`, `"with":{"count":{"agg":"count",…},"total":{"agg":"sum",…}}` |
| `lookup`(突き合わせ) | `entity` のレコードのうち `field`(string / text / enum / integer / number / date / ref か `$id`)の値が、行の `equals`(field か `$id`)の値と同じものがある(`exists`)/ 無い(`notExists`)時だけ動く。文字と数は全角半角・大文字小文字・前後の空白を区別せず文字として比べる(取り込んだ名簿の表記の揺れ)。日付は日付どうし、ref と `$id` は同じ entity の ID どうしで、そのまま比べる。行の値が空ならどちらの op でも動かない。`entity` が行と同じなら行のレコード自身は数えない(重複の検出)。`if` の後に確かめる(`if` が偽なら読まない) | 名簿との照合(tk_2413): `each` registrations + `{"op":"notExists","entity":"roster","field":"student_id","equals":"student_id"}` + `updateRecord` `mismatch = true`。まだ行っていない店: `each` shops + `{"op":"notExists","entity":"visits","field":"shop","equals":"$id"}` |
| `lookup` の `overlaps` / `notOverlaps`(期間の重なり) | `entity` のレコードの期間(`field`〜`fieldEnd`)が、行の期間(`equals`〜`equalsEnd`)と重なるものがある(`overlaps`)/ 無い(`notOverlaps`)時だけ動く。4 つとも date / datetime の stored field(`$id`・computed は不可)で、開始と終了は同じ型(行の側と探す側は date と datetime を混ぜてよい)。date はその日全体(作った人のタイムゾーン・終わりの日も含む)、datetime は `[開始, 終了)`(続けて並ぶ 10:00〜11:00 と 11:00〜12:00 は重ならない)。`fieldEnd` / `equalsEnd` を省くか値が空なら開始の日 / 時刻だけ。開始が空・形が違う・終了が開始より前なら期間でない(行ならどちらの op でも動かず、探す側のレコードは数えない)。同じ entity なら行のレコード自身は数えない。読み方・上限は `lookup` と同じ(1 回の実行で 1 度だけ一覧し、期間をメモリで比べる)。重なりを見つけるだけで、空きを探して予定を組む・ずらすことはしない | 予約の重なり: `created` bookings + `{"op":"overlaps","entity":"bookings","field":"start","fieldEnd":"end","equals":"start","equalsEnd":"end"}` + `notify`。作業できない日(ng_2412): `each` tasks + `{"op":"overlaps","entity":"days_off","field":"day","equals":"from","equalsEnd":"due"}` + `notify`。全員が空いている候補日(D_2210): `each` candidates + `notOverlaps` busy |
| `similar`(一番似ている記録) | `entity` のレコードのうち、`fields`(行の entity と `entity` の両方に**同じ id である** string / text / enum / integer / number / date / ref / refs の field)の値が行と同じ数が一番多いもの。string / text は `、` `,` `/` `・` 改行で区切った項目ごと(1 つの値は 20 項目まで)、refs は ID ごと、ほかは値 1 つ(`lookup` と同じく全角半角・大文字小文字・前後の空白を区別しない)。同じ数が `min`(1〜20・省略 = 1)以上の時だけ動き、`do` で `{"ref":"$similar"}`(その記録の ID・ref として書ける)・`{"ref":"$similar.<field>"}`・`{"ref":"$similarScore"}`(同じ数)・本文の `{$similarScore}`・`to` `{kind: authorOf, field: $similar}`(その記録を書いた人)が使える。行のレコード自身と、shared / public では行を書いた人の記録は選ばない。同じ数なら新しい記録。hidden・computed の field は比べず、`$similar.<hidden>` も読めない。`if`・`lookup` の後に確かめる | 似ている人の推薦(F_2201・hs_2405): `updated` profiles + `{"entity":"profiles","fields":["hobbies","level"],"min":2}` + `updateRecord` `best_match = $similar`・`match_score = $similarScore`。合う学生へのスカウト(os_2403): `created` circles + `{"entity":"students","fields":["tags"]}` + `notify` `to authorOf $similar`(メンバーへの通知は今までどおり本人の同意が要る) |
| `pick`(おすすめを 1 つ選ぶ) | `entity` のレコードから、`filter`(`agg` の filter と同じ書き方・3 つまで・hidden / computed の field は不可)に合い、`exclude` `{entity, field}` の entity のレコードが `field`(選ぶ entity への ref / refs)で指していないものを 1 つ選ぶ。`order` は `random`(実行ごとに決まる。実行の ID から決めるので、同じ実行のやり直しでは同じ記録を選び、二重に送らない。その実行が作ったレコードは `exclude` に数えない)・`oldest` / `newest`(作った時刻)。選べた時だけ動き、`do` で `{"ref":"$pick"}`(その記録の ID・ref として書ける)・`{"ref":"$pick.<field>"}`・本文の `{$pick.<field>}`(文字・数・日付などの field)・`to` `{kind: authorOf, field: $pick}`(その記録を書いた人)が使える。選んだ記録は書き換えない(出した記録は別の entity に `createRecord` で残し、`exclude` で次から除く)。行のレコード自身は選ばない。行が無くてもよく、`each` の無い時刻のきっかけでは宛先は `owner` / `members`。hidden の field は読めない(`$pick.<hidden>`・`{$pick.<hidden>}`)。`if`・`lookup` の後に確かめる | 毎朝のおすすめ(D_2212): `every daily` + `{"entity":"menus","order":"random","exclude":{"entity":"eats","field":"menu"}}` + `createRecord` picks `menu = $pick`・`name = $pick.name` + `notify` members「まだ食べていない {$pick.name} はどうですか」。献立の提案(ng_2413): `filter` `kind eq main` の `random`。順番に出す: `oldest` + `exclude` 出した記録 |
| `saveFeed`(公開データを記録に残す・処理) | each の無い時刻のきっかけだけ。`provider`(weather / pressure / warnings)と `region`(feed と同じ府県予報区のコード)のその時の値を `entity` に 1 件足す。`fields` は残す値 → field(`date` 実行の日・`summary` 文・`min` / `max` 数・`drop` 気圧の急な低下)。書く先は `createRecord` と同じ決まり(同じ側・single と宛先付きは不可・連鎖の検出)。required の field は date か summary だけ。アプリに 3 つまで。止めた provider・取れない日は書かない([external-feeds.md](../backend/features/external-feeds.md#記録に残すサーバーの自動の動き-savefeed)) | 体調と気圧(tk_a_2518): `{"every":"daily","time":"07:00"}` + `saveFeed` `{provider: pressure, region: "130000", entity: pressure_log, fields: {date: day, summary: note, min: low, max: high, drop: falling}}` |

```json
{"id":"due_soon","on":{"every":"daily","time":"08:00","each":"tasks"},
 "if":{"op":"and","args":[{"op":"neq","args":[{"ref":"done"},true]},
       {"op":"eq","args":[{"op":"daysUntil","args":[{"ref":"due"}]},3]}]},
 "do":[{"notify":{"to":{"kind":"owner"},"title":"締め切り","body":"{title} の締め切りまであと 3 日です"}}]}
```

- 例の全体は `backend/pkg/dsl/testdata/server_rules_reads.json`(家計の集計・週のまとめ・期限)と `server_rules_ranking.json`(shared の順位)、`server_rules_lookup.json`(名簿との照合・まだ行っていない店)、`server_rules_similar.json`(似ている人・スカウト)、`server_rules_pick.json`(まだ食べていないメニューのおすすめ)、`server_rules_overlap.json`(予約の重なり・作業できない日)。
- **費用と上限**(runner の `quota`): 読んだレコード 1 件 = Firestore の読み取り 1 回(1,000 件 ≈ 0.0004 USD・#338 の原価)。1 回の実行で読めるのは Pro 500 件(`RUNNER_LIMIT_RUN_READS_PRO`)、アプリごとに 1 日 Pro 20,000 件(`RUNNER_LIMIT_APP_READS_PRO`・カウンタ `rr_app_{appId}`)。同じ entity は 1 回の実行で 1 度だけ読む(`each` の行どうしで共有)。**超えたらその回は実行しない**(`skipped_quota`・`limit` に `run_reads` / `app_reads`)。一部の記録だけで計算した誤った合計・順位は出さない。`runner.usage` の `reads` と `est_cost_usd` に入る。値は出発点で、#338 と同じく実測で直す。
- 読める範囲(#337 の層 3 を広げた): きっかけのレコード・その ref 先に加えて、`each` / `agg` / `lookup` / `similar` / `pick`(と `exclude`)の entity の**同じアプリ・同じ側**のレコードと、その ref 先。取っ手(`scope.Handle.List`)が appId と側を確かめる。`lookup` は `agg` と同じ取っ手でその entity を 1 回の実行で 1 度だけ一覧し(上の件数の上限・費用に数える)、値で引く表を作って突き合わせる。`similar` も同じく 1 度だけ一覧し、比べる field の項目の表を作ってメモリで数える(計算は 行 × 一覧の件数 × 比べる項目(5 field × 20 項目まで)で、件数の上限で抑えられる)。`agg` の count + filter で読めるものより多くは読まない。宛先付きの entity(§3.14)は `each` / `agg` / `lookup` / `similar` / `pick` / ref 先の読み取りの対象にできない。`pick` も 1 度だけ一覧し、絞り込み・除く ID の表・並べ替えをメモリで行う。
- 1 回の書き込みの上限(Pro 10)は `each` の全部の行で数える。順位を 10 件より多く書き換える日は、残りがその日は書かれない(次の日に続きが書かれる)。
- まだ作らない(決定の範囲の外か、宣言で表せない): 履歴から好みを学ぶ推薦・文章の意味の近さ・全員を組にする自動の組み合わせ(「まだ行っていない」「名簿に無い」は `lookup`、共通の項目の数で一番似ている 1 件は `similar`、ランダム・まだのもの・古い順の 1 件のおすすめは `pick` で書ける)、複数人への割り当て・日程の自動調整(重なりを見つけて知らせる・印を付けるのは `lookup` の `overlaps`)、距離での自動のグループ分け、外部のデータの取得。

- 共有データに効くのは作成者の公開 DSL(`apps_public/{appId}`)の宣言だけ。参加者が自分のコピーを書き換えても効かない(#337・runner の側で守る)。
- 人が読める説明は `App.DescribeServerRule`(例: 「出欠」が作られた時(条件に合えば)、「予定」を作った人に通知・「予定」の「参加の数」を数える)。「このアプリの自動の動き」の一覧(#347)で使う。
- 古い iOS: `AppDSL` はトップの未知キーを読まないので、`serverRules` があっても今までどおり開ける。
- AI の生成(#343): 生成用スキーマ(`ai.GenerationSchema()`)には入れず、依頼が自動の動き(通知・定期・数える など)を含む時だけ、UI の生成の後に別の小さな呼び出し(`backend/internal/ai/server_rules_pass.go`・専用スキーマ `ServerRulesGenerationSchema()`・`createRecord` / `updateRecord` の `values` はそこだけ `[{field, value}]` の配列)で `serverRules` 全体を書く。結果は `Validate()` を通し、通らなければ問題を添えて再生成、最後まで通らなければルール無しで保存して summary で断る。作ったルールは `DescribeServerRule` の文で summary に出る。`AI_SERVER_RULES=1` の時だけ動く(既定は無効)。

### 3.13 管理者の権限と重複の防止(#272 #273・M4)

「店長だけが確定できる」「削除は管理者だけ」「同じ時間に 2 人予約できないように」「1 人 1 回答」を宣言で書く。守るのはサーバー(appconfig)で、iOS は表示を合わせるだけ。実装: `backend/pkg/dsl/data_rules.go`(検証)・`backend/internal/appconfig/data_rules.go`(権限と 409)・`backend/internal/store/unique.go`(一意の予約)。

| 書く場所 | 形 | 意味 | 制約(`Validate()`) |
|---|---|---|---|
| 管理者(DSL 外) | `PATCH /apps/{appId}/members/{uid}` `{"admin": true}` | 作った人が最初の管理者(外せない)。管理者は参加者を管理者にでき、自分でやめられる(`me`)。`GET /members` の各行と最上位に `admin` | — |
| `field.editableBy` | `"admins"` | その項目を入れる・変えるのは管理者だけ(作成で既定値・空以外を入れる / 更新で値を変える → 403 `details.reason: admins_only`)。管理者は**他人のレコードでもその項目だけ**変えられる(entity の `edit` によらない) | shared / public の保存する field |
| `entity.deleteBy` | `author`(省略時)/ `admins` / `authorOrAdmins` | 誰が消せるか。全件削除(`DELETE .../records`)は、管理者なら全員の分・`admins` で管理者でなければ 403 | shared / public・match / single 以外 |
| `button.visibleTo` | `"admins"` | 管理者にだけボタンを出す(表示だけ。守る値には editableBy / deleteBy も付ける) | button だけ・shared / public の entity があるアプリ |
| `field.type: "member"` | 参加者の uid(文字列) | 担当者・当番。入力は picker(参加者の表示名が並ぶ)、一覧は表示名。参加者でない uid は 422 | scope `shared` の entity だけ・defaultValue 不可・入力は picker |
| `entity.unique` | `["slot"]`・`["day","slot"]`・`["$author"]`・`["event","$author"]`(1〜3 個) | 全部が同じレコードは 1 件だけ。違反は **409 `duplicate_record`**(`details.reason: duplicate`・`details.fields`) | field は string / integer / number / currency / boolean / date / datetime / enum / ref / member。`$author` は shared / public。single / match には書かない |

- **一意の守り方**: 値の組を hash した鍵を `entities/{e}/unique/{key}`(`{recordId}`)にレコードと同じトランザクションで置く(2 人が同時に押しても片方だけ通る)。レコードは自分の鍵を `uniqueKey` に持ち、更新で値が変われば鍵を移し、削除・全件削除で外す。`unique` を足す前からあるレコード(鍵が無い)とも比べるため、鍵を取る時に同じ値のレコードをクエリでも探す(読み取り +1)。鍵の持ち主のレコードが無い・別の鍵を持つ時は古い鍵として取り直す。空の項目があるレコードは比べない。値の変わらない更新は比べ直さない(前からある重複を編集で 409 にしない)。
- **サーバーの自動の動き(§3.12)の書き込み**は権限と一意を見ない(作成者の宣言で動くため)。一意の比較の相手にはなる。
- iOS: `supportedFeatures` に `dataRoles` を送る。送らない古い版には AI が member と visibleTo を使わない(member は string に・visibleTo は外す・§5.3)。古い版でも editableBy / deleteBy / unique はサーバーが守る(断られた時の文言が汎用になるだけ)。iOS は 409 `duplicate` を「同じ内容の記録がもうあります」、403 `admins_only` を「管理者だけが変更できます」と出す。
- AI: `backend/internal/ai/data_roles_rules.go`。

### 3.14 宛先を選んで送るメッセージ(1 対 1・グループ宛て・#271・M4)

「1 対 1 のメッセージ」「先生にだけ質問」「選んだ何人かに連絡」を、宛先の人と送った人だけが読める記録で作る。守るのはサーバー(appconfig)で、iOS は宛先を選ぶ入力と表示を持つだけ。実装: `backend/pkg/dsl/addressed.go`(検証)・`backend/internal/appconfig/addressed.go`(読める人の判定)・`backend/internal/store/watch_visible.go`(リアルタイム更新)。

| 書く場所 | 形 | 意味 | 制約(`Validate()`) |
|---|---|---|---|
| `entity.addressedTo` | 宛先の field id | この entity のレコードは**作った人と宛先の人だけ**が読める。ほかの参加者(アプリの作成者・管理者を含む)には、無いのと同じ | scope `shared` だけ。宛先の field は type `member`(1 人)か `members`(複数)・computed / editableBy 不可。`edit:"members"`・`single`・`match` とは書けない。`unique` を書くなら `$author` を含める |
| `field.type: "members"` | 参加者の uid の配列 | グループ宛ての宛先。入力は picker(参加者を複数選ぶ)、一覧は表示名を並べる。参加者でない uid は 422 | scope `shared` の entity だけ・defaultValue 不可・入力は picker |

- **読める人の決め方**: レコードを作る時に appconfig が「作った人 + 宛先」を `readers` としてレコードに持たせる(API には出さない)。宛先は送った後で変えられない(PATCH で変えると 422)。宛先が空の作成も 422。`readers` を持つレコードは、後から DSL の `addressedTo` を外しても `readers` の人にしか見えない(古いメッセージが開かない)。
- **見えない人に対して**(すべて 404・中身も有無も返さない): 一覧(`GET .../records` から除く)・取得・PATCH・DELETE・印(`PUT/DELETE .../marks`)・通報(`POST /reports` の record)。全件削除(`DELETE .../records`)は今までどおり自分の分だけ(`deleteBy` で管理者が消せる entity なら管理者は全員の分を消せる。中身は読めない)。リアルタイム更新(`GET /events`)は、宛先付きの entity では自分が読める記録が変わった時だけ `records` を送る。
- **集計**: 一覧が読める分だけなので、端末の集計(stat・agg・未読の数)も自分が読める記録だけになる。
- **自動の動き(§3.12)**: 宛先付きの entity をきっかけにするルールは、通知の宛先が `author` と `recipients` だけ・書き換えはその記録だけ(`createRecord` / `increment` / ref の `updateRecord` は不可)。どのルールも宛先付きの entity に `createRecord` できず、宛先の field を書き換えられない。
- **ルールが読めるもの**: 宛先付きの entity は `on.each`・`agg`(`if` / `values` / `by` / `with` のどこでも)・`lookup`・`{"ref":"<refField>.<field>"}` の対象にできない(`Validate()` で断る)。runner も実行の時に同じ表で確かめ、合わないルールは `skipped_stopped`(`addressed_rule`)で動かさない。`readers` を持つレコードは、後から `addressedTo` を外しても `each` / `agg` / ref 先の読み取りに出てこず、きっかけにもならない。
- **宛先の人への通知**: `{"to":{"kind":"recipients","field":"<addressedTo の field>"}}`。きっかけが宛先付きの entity のレコードの時だけ。届くのは実行の時に参加している宛先の人だけで、送った本人・同意していない人・ブロックの関係にある人には届かない(`members` と同じ確かめ)。本文に `{<field>}` を差し込める(受け取る人はその記録を読める)。
- **未読**: 既存の印(§3.5 `marks`)の `read` を使う。受信箱の `list.item.mark: "read"` で既読の印、`{"field":"$marks.read","op":"neq","value":{"me":true}}` で未読だけ。受信箱は `$author` neq me(読めるのは自分宛てだけなので、これで自分宛て)、送信済みは eq。
- iOS: `supportedFeatures` に `addressedMessages` を送る(審査用の版は共有を外しているので送らない)。送らない版には AI が addressedTo と members を使わない(addressedTo は外し、members は string に・§5.3)。
- AI: `backend/internal/ai/messages_addressed_rules.go`。ai-eval の `addressed` / `addressedGroup` / `addressedUnread`(`addressed_cases.json`)。

### 3.15 通知の alarm = 止めるまで鳴るアラーム

「目覚まし」「7 時に起こして」「止めるまで鳴らして」は、時刻の通知に `"alarm": true` を付ける。iPhone の時計のアラームと同じく、ロック中も全画面で鳴り、止めるまで鳴り続ける(iOS 26.1 以降の AlarmKit)。

```json
{"id":"wake","title":"起きる時間です","body":"","repeat":"daily","entity":"alarm","field":"time","titleField":"label","enabledField":"on","alarm":true}
```

| 項目 | 決めたこと |
|---|---|
| 形 | 新しいアクションは足さず、`Notification` の真偽 1 つ(`alarm`)。時刻・くり返し・ラベル・オン/オフは通知と同じキー(固定の `time` / `weekday`、レコードの `"HH:mm"` の field + `titleField` + `enabledField`)。`scheduleNotification` のボタンにも付けられ、取消は `cancelNotification` |
| 付けられないもの | `when`(条件の通知)とは組み合わせない(backend が拒否)。服薬・予定の N 分前などのリマインドには付けない(AI の規則) |
| くり返し | daily = 毎日・weekly = その曜日(AlarmKit の `.relative` の `.weekly`)、once = 次のその時刻(`.fixed`)。monthly / yearly は AlarmKit にくり返しが無いので次の 1 回だけ予約し、アプリを開いた時の予約し直しで次へ延ばす |
| 落とし方 | iOS 26.1 未満(26.0 は廃止予定の止めるボタンが必須なので使わない)・アラームの許可が無い・審査用の版・予約の上限で失敗した時は、**同じ時刻の普通のローカル通知**で届ける。`alarm` を知らない古い iOS もこのキーを無視して普通の通知にするので、`supportedFeatures` の印は要らない |
| 許可 | アラームの通知を初めて予約する時に AlarmKit の許可(`AlarmManager.requestAuthorization`・Info.plist の `NSAlarmKitUsageDescription`)。アラームで届ける分は普通の通知を出さない(二重に鳴らさない) |
| 予約の id | 通知の識別子(`dsl.<appId>.…`)・見出し・時刻から決まる UUID。中身が変われば古いアラームを消して予約し直す。今鳴っている最中のアラームは予約し直しでは止めない |
| 後片付け | アプリの削除(`removeApp`)はそのアプリのアラームを鳴っていても消す。サインアウト・アカウント削除(`removeAll`)はこのアプリのアラームを全部消す |

- iOS: 組み立ては `AlarmPlanner` / `AlarmSync`(MetamonServices・AlarmKit を差し替えてテスト)、AlarmKit の呼び出しは `Metamon/App/AlarmKitAlarms.swift`。審査用の版はコードごと外し、`NSAlarmKitUsageDescription` も消す(`docs/ios/review-build.md`)。
- AI: `backend/internal/ai/alarm_rules.go`。ai-eval の `alarm` / `alarmRecord`(`alarm_cases.json`)。
- カウントダウン(スヌーズ・タイマー)は使わないので、AlarmKit 用の Live Activity(ウィジェット拡張の `ActivityConfiguration(for: AlarmAttributes…)`)は用意していない。鳴った時の画面は OS の標準のもの(実機で確かめる)。

### 3.16 通知の near = 場所に着いたら知らせる・jitterMinutes = 時刻をずらす

「降りる駅に近づいたら知らせて」「お店の近くを通ったら」「危ない場所に近づいたら警告」は、`app.notifications[]` に `near` を書く。場所は `near.entity` の各レコードの location field(`"lat,lng"`)。「ランダムな時間に」「期限を内緒で前倒し」は、時刻の通知に `jitterMinutes` を足す。

```json
{"id":"arrive","title":"もうすぐ到着","body":"降りる準備を","repeat":"once","near":{"entity":"stations","locationField":"spot","radiusM":800}}
{"id":"photo","title":"写真の時間","body":"","repeat":"daily","time":"20:00","jitterMinutes":60}
```

| 項目 | 決めたこと |
|---|---|
| near の形 | `{entity, locationField, radiusM?}`。`entity` は private(shared / public は不可=自分の場所だけを見張る)、`locationField` は hidden でない location 型。`radiusM` は 100〜2000(省略 200) |
| 引き金は 1 つ | `near` があればそれだけが引き金。`time` / `weekday` / `entity` / `field` / `offsetMinutes` / `titleField` / `enabledField` / `when` / `alarm` / `jitterMinutes` とは組み合わせない。`repeat` は `once`(最初に着いた 1 回だけ)か `daily`(着くたび)。`scheduleNotification` には書けない(`app.notifications` だけ)。backend が拒否(`backend/pkg/dsl/notification_near.go`) |
| 判定 | iOS の `UNLocationNotificationTrigger`(`CLCircularRegion`・入った時だけ `notifyOnEntry`)。**判定は端末の中**で、位置を backend に送る処理は足していない(レコードの場所は今までどおりレコードとして保存される) |
| 上限 | iOS の予約は 1 アプリ 64 件・見張れる領域は約 20。全アプリ合わせて **15 か所**(現在地が OS に残っていれば近い順、無ければ新しく保存した順)。残りの枠(60 − 場所の数)を時刻の通知が近い順に使う。レコードが変わる・アプリを開くたびに選び直す |
| 1 回きり | 予約したのに iOS の予約から消えていれば届いたとみなし、予約し直さない(端末の UserDefaults に記録)。場所(緯度・経度・半径)を直したら見張り直す |
| 許可 | 位置情報は「使用中のみ」(`NSLocationWhenInUseUsageDescription`)。場所を保存したアプリの予約の時に、未決定なら 1 回だけ聞く。拒否されたら聞き直さず、設定画面の通知の欄に「位置情報が許可されていないため、場所に着いた時の通知は届きません」と出す(上のバーは出さない) |
| jitterMinutes | 1〜60。時刻の通知(`time` か `entity` + `field`)の回ごとに ±N 分ずらす。ずらし幅は通知の識別子と回の時刻から決まり(FNV-1a)、予約し直しても同じ回は同じ時刻。くり返し(daily / weekly / monthly / yearly)は回ごとに時刻が違うので、近い回から具体的な日時で予約する(毎日 7・毎週 4・毎月 3・毎年 2 回分。識別子 `<id>.j<回の分>`)。「いつも早めに」は `offsetMinutes: -N` と `jitterMinutes: N`(0〜2N 分前)。`when` / `near` / `scheduleNotification` には付けない |

- 古い iOS: `near` を知らない版は残りを**次の 9:00 の 1 回きりの通知として予約してしまう**ので、iOS は `supportedFeatures` に `notification.near` を入れ、`allowedComponents` を送るのにこれを含めない版には AI が `near` / `jitterMinutes` を使わない(生成されたら near の通知ごと・`jitterMinutes` のキーを落とす・§5.3)。審査用の版も送る(位置の判定は端末の中だけ)。保存済みの DSL を古い版で開いた時は防げない(`when` と同じ既知の制限)。
- iOS: 組み立ては `NotificationPlanner+Near.swift`(純関数)、予約は `UserNotificationScheduler`、1 回きりの記録と許可は `RegionNotifications.swift`(MetamonServices)。
- AI: `backend/internal/ai/notify_near_rules.go`。ai-eval の `notifyNear` / `notifyNearRepeat` / `jitter`(`notify_near_cases.json`)。
- できないこと: 残りの距離を通知に出し続ける・経路の途中で鳴らす(道案内は `routeGuide`)・ほかの人が足した場所を、この端末が同期する前に見張る。

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

## 5. 版と互換性(#302)

App Store に出した後は古い iOS が残り続ける。backend は「今いちばん古い、まだ使われている iOS」でも壊れないアプリだけを返す。

### 5.1 DSL の版(`schemaVersion`)

| 変更 | 扱い | 例 |
|---|---|---|
| 部品(Container / Leaf)・Action・Field 型・op を**足す** | 互換。`schemaVersion` は上げない | `routeGuide` を足す |
| 省略できるキー・enum の値を**足す** | 互換。古い iOS は未知キーを無視し、未知 enum は既定に落とす(§1「iOS は寛容」) | `photoField.source` に新しい値 |
| 既存のキー・値の**意味を変える**・必須にする・消す・名前を変える | 非互換。`schemaVersion` を上げる(`backend/pkg/dsl.SchemaVersion` と iOS の `AppDSL.supportedSchemaVersion` を揃えて更新) | `stat.value` の書き方を変える |

- 意味を変えたくなったら、まず「新しい名前で足して古い方を残す」(互換)で済まないかを考える。版を上げるのは最後の手段。
- 版を上げた時は、古い版の iOS には古い版の DSL を返し続けるか、§5.4 の最低対応版を上げて古い iOS を止めるかを同じ PR で決める。

### 5.2 iOS が毎回送るもの

| どこに | 中身 | 使い道 |
|---|---|---|
| generate / chat の body `allowedComponents` | その版が描画できる部品 type の全部(`ComponentType` から自動) | backend はここに**無い** type を生成結果に入れない(下) |
| generate / chat の body `supportedFeatures` | 部品以外で実装済みの DSL の動作(`recordRetention`・#482、`matching`・#507、`notification.when`・#265、`singleEntity`・#267、`dataRoles`・#272、`addressedMessages`・#271、`notification.near`、`aiButton.long`、`aiButton.inputPhoto`、`aiButton.saveTo`、`aiButton.outputs`、`action.lookupBook`、`text.highlight`(text の {{fieldId}} も)、`feed.v2`(feed の warnings / pressure)、`expr.addMinutes`(計算式の op `addMinutes`)、`aiChat.inputEntity`(aiChat が自分の記録を読む)。審査用の版は `matching`・`dataRoles`・`addressedMessages` を送らない) | `allowedComponents` を送るのにここに無い動作は AI に使わせない(v1.0 = 記録の自動削除なし) |
| 全リクエストのヘッダ `X-Client-Platform` / `X-Client-Version` / `X-Client-Build` / `X-Client-DSL-Version` | `ios` / `CFBundleShortVersionString` / `CFBundleVersion` / 描画できる最大の `schemaVersion` | ログ(どの版がまだ使われているか = 最低対応版を上げる判断) |

### 5.3 backend が古い iOS に新しい部品を出さない仕組み

`backend/internal/ai/restrict.go`(#245 の審査用ビルドと同じ仕組み)。`allowedComponents` に無い既知の type は、AI に渡す部品カタログ・索引・生成スキーマから外し、生成結果に残れば再生成させ、最後の試行でも残れば取り除く。取り除くと成り立たない時はエラー(`generation_failed`)にして、その部品は送らない。

- `allowedComponents` に backend の知らない type(iOS の方が新しい)があっても無視する。全既知 type を送った時は制限なし(プロンプトは byte 同一)。
- `allowedComponents` を送らない(v1.0 より前の開発版)クライアントは制限なし。
- 部品でない DSL のキー(`Entity.retentionDays`・#482)は `supportedFeatures` で同じことをする: 無い版にはスキーマ・索引・ルールから外し、新しく付けた分は再生成させ、残れば取り除く。
- 計算式の op も同じ: `addMinutes`(`expr.addMinutes`)を知らない版は未知の op を「—」として評価する(落ちない)ので、無い版にはスキーマの `ExprOp.op` の値・ルールから外し、新しく使えば再生成させ、残れば式を `""`(その版の表示と同じ「—」)に置き換える。
- 試験: `backend/internal/ai/compat_test.go`。v1.0 の部品一覧は `testdata/ios_v1_components.json` に凍結してあり、**部品を足しても書き換えない**(足した部品が v1.0 に出ないことを試験が確かめる)。例の DSL ごとに「その部品を知らない 1 つ前の版」を作り、生成結果にその部品が出ないこと・DSL として開けることを確かめる。
- 保存済みの DSL(別の端末で作った・シェアで取り込んだ)は `GET /dsl` でそのまま返る。古い iOS は未知の type をプレースホルダで描くので、画面は壊れない(§1)。

### 5.4 最低対応版と強制アップデート

appconfig `GET /v1/client-config`(認証不要・`docs/backend/openapi-appconfig.yaml`)が環境変数から返す。

| 環境変数(appconfig) | 意味 | 未設定 |
|---|---|---|
| `MIN_IOS_BUILD` | これ未満の `CFBundleVersion` は閉じられない「アップデートしてください」画面 | 強制しない |
| `LATEST_IOS_BUILD` | これ未満は閉じられるお知らせ | 出さない |
| `IOS_APP_STORE_URL` | 更新ボタンの行き先(`https://apps.apple.com/app/id…`) | iOS の Info.plist `MetamonAppStoreURL`、それも無ければ App Store で「Tailor It」を検索 |

iOS は起動時と前面に戻った時に読む。**通信失敗・不正な応答では何もしない**(fail open: backend が落ちていてもアプリは使える)。

### 5.5 API の破壊的な変更

- パスの `/v1` は互換な変更(エンドポイント・省略できるフィールド・enum の値を足す)の間は変えない。クライアントは未知のフィールドを無視する。
- フィールドを消す・意味を変える・必須にする変更は `/v2` を新しく作る。`/v1` と `/v2` は**並走**させ、`/v1` を使う版が `MIN_IOS_BUILD` で止められるまで(目安: `/v2` を使う版の公開から 90 日以上、かつログの `clientBuild` で旧版の利用が十分に減るまで)消さない。
- 並走中の `/v1` は直すだけで機能を足さない。消す前に `MIN_IOS_BUILD` を `/v2` を使う最初の版に上げる。
