# DSL v1(統一版・正典)

> **正典はこのフォルダ**: `dsl.schema.json`(JSON Schema draft 2020-12)+ `examples/{initial,kakeibo,training,walk}.json`(デモ3本+初期状態)。
> Go 型は `backend/pkg/dsl`(`schema.json` を埋め込み・`make schema-sync` で同期・CI で byte 一致を検証)、Swift 型は `ios/Morph/DSL/Model`(examples をそのままバンドルしデコードテスト)。
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
| 6 | ナビゲーション | `initialScreen` + `navigate{screen,presentation:push/sheet/fullScreen}` | `navigation{type:stack/tab, tabs[]}` + `navigate/present/dismiss` | **backend 案**。タブ無し。`back` が pop と dismiss を兼ねる | デモ3本はタブ不要。アクション数を減らす。タブは P1 で `app.tabs[]` を**追加**すれば互換 |
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
| 22 | 編集ターンの形 | `$defs/PatchDocument{patch,summary}`(構造化出力) | — | **backend 案**。`$defs/PatchValue` で value を閉じる | iOS には無関係(常にフル DSL) |

## 3. 構造

```
AppDSL { schemaVersion:1, app:AppMeta, initialScreen, entities[], screens[] }
AppMeta { name, description?, icon?, theme?{primaryColor,mode}, voiceEntry: fullscreen|floating|none, notifications[]? }
Entity  { id, name, titleField?, fields[]: Field{ id, name, type, required?, options?, defaultValue?, unit? } }
Screen  { id, title, entity?, toolbar[]?: {label, icon?, action}, components[]: Container|Leaf }
Container { type: vstack|hstack|section, id?, title?, spacing?, children[]: Leaf }
Leaf (15 種) / Action (9 種) / Notification { id, title, body, repeat: once|daily|weekly, time?, weekday? }
API 封筒(appconfig): DSLEnvelope { appId, version, dsl: AppDSL, source?, createdAt? }
```

### 3.1 コンポーネント(18 種 = Container 3 + Leaf 15)

| type | 必須 | 任意 | 要 `screen.entity` | P |
|---|---|---|---|---|
| `vstack` / `hstack` | `children` | `spacing` | | P0 |
| `section` | `children` | `title`, `spacing` | | P0 |
| `text` | `text` | `style`(title/headline/body/caption), `align` | | P0 |
| `button` | `label`, `action` | `icon`, `style`(primary/secondary/destructive/plain) | | P0 |
| `textField` | `field` | `label`, `placeholder`, `multiline` | ○ | P0 |
| `numberField` | `field` | `label`, `placeholder`, `unit` | ○ | P0 |
| `datePicker` | `field` | `label`, `includeTime` | ○ | P0 |
| `picker` | `field`(enum) | `label`, `style`(menu/segmented/wheel) | ○ | P0 |
| `toggle` | `field`, `label` | | ○ | P0 |
| `photoField` | `field`(photo) | `label`, `source`(camera/library/both) | ○ | P0 |
| `image` | `field`(photo) | `height`(small/medium/large) | ○ | P0 |
| `list` | `entity`, `item{title}` | `item.subtitle/trailing/icon/image`, `sort`, `filter`, `emptyText`, `onItemTap`, `swipeToDelete` | | P0 |
| `stat` | `label`, `entity`, `aggregate` | `field`(count 以外必須), `filter`, `format`(plain/currency/percent) | | P0 |
| `chart` | `entity`, `kind`(bar/line/pie), `groupBy`, `aggregate` | `title`, `field`, `bucket`(day/week/month), `filter` | | P1 |
| `voiceInput` | | `prompt`, `size`(compact/large) | | P0 |
| `spacer` | | `size`(small/medium/large/flexible) | | P0 |
| `divider` | | | | P0 |

### 3.2 アクション(9 種)

| type | フィールド | iOS の挙動 |
|---|---|---|
| `navigate` | `screen`, `presentation?`(push/sheet/fullScreen) | 遷移。`list.onItemTap` からは **recordId を暗黙に渡し**遷移先フォームが編集モードになる |
| `back` | | スタックがあれば pop、モーダル最下層なら dismiss |
| `saveRecord` | `entity`, `screen?` | ドラフトを POST(新規)/PATCH(編集)→ レコード再取得 → `screen` へ、無ければ `back` |
| `deleteRecord` | `entity`, `confirmMessage?` | 現在の recordId を DELETE → back。list の長押し削除も同じ |
| `openVoice` | | 音声シートを開く |
| `openUrl` | `url` | Safari で開く |
| `resetForm` | | ドラフトを既定値に戻す |
| `scheduleNotification` | `notification` | ローカル通知を予約(`dsl.<id>`) |
| `cancelNotification` | `notificationId` | 予約を取消 |

`confirmMessage` はどのアクションにも付けられ、先に確認ダイアログを出す。

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

`defaultValue` は date/datetime で `"now"` を許可。

## 4. 変更手順(Day2 に部品を足す時)

1. `docs/dsl/dsl.schema.json` を編集(Leaf を足す = `$defs` に object を足し `Leaf.anyOf` と `PatchValue` に追加)。
2. `cd backend && make schema-sync && go test ./...`(`dsl.go` の `ComponentType` と `validate.go` の per-type 検証も更新)。
3. `ios/Morph/DSL/Model/Component.swift` の `ComponentType` に case 追加 + `DSLView` に View 1 個。
4. `examples/*.json` に使用例を足す(backend の `TestExamplesRoundTrip` と iOS の `DSLDecodingTests` が両方で検証)。
5. ai のプロンプトカタログ(コンポーネント表)を更新。
