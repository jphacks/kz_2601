# DSL JSON スキーマ v0 提案(iOS 側)

> **【旧案・参考】** 2026-10-24 昼に backend 案とすり合わせ、統一版 **DSL v1** を `docs/dsl/README.md` + `docs/dsl/dsl.schema.json` に確定した。構造は backend 案(非再帰2段・entity/field 束縛)、命名は本案(camelCase・フラット)を採用。本案の `{{式}}` / `state` / `sequence` / `native` / `screens` object は **v1 に無い**(タブは `app.tabs[]` として v1 に採用)。画面内配置は v1 で**グリッド座標 `area`**(README §3.4)になった。実装(`ios/Morph/DSL/Model`)は v1 に更新済み。

> backend(@2410as)案とすり合わせる前提。最後の節に「譲れない / 柔軟」を明記。
> 実装: `ios/Morph/DSL/Model/*.swift`、サンプル: `ios/Morph/Resources/Samples/*.json`。

## 1. ルートドキュメント

```json
{
  "schemaVersion": 1,
  "appId": "demo",
  "version": 12,
  "name": "かんたん家計簿",
  "theme": { "accent": "#FF7A1A", "colorScheme": "system" },
  "state": { "draft": { "title": "", "amount": "" } },
  "navigation": {
    "type": "tab",
    "tabs": [
      { "screen": "home", "title": "ホーム", "icon": "yensign.circle" },
      { "screen": "settings", "title": "設定", "icon": "gearshape" }
    ]
  },
  "screens": {
    "home": { "...": "Screen" },
    "settings": { "...": "Screen" }
  }
}
```

| キー | 型 | 必須 | 意味 |
|---|---|---|---|
| `schemaVersion` | int | ○ | 1 固定。非互換変更時にインクリメント |
| `appId` | string | ○ | Firestore ドキュメント id |
| `version` | int | ○ | 単調増加。iOS はこれが進んだ時だけ再描画(ホットリロードの基準) |
| `name` | string | ○ | アプリ名(表示用) |
| `theme.accent` | `#RRGGBB` | - | tint 色 |
| `state` | object | - | 初期 state。差し替え時にリセットされる |
| `navigation.type` | `"stack"` \| `"tab"` | ○ | |
| `navigation.root` | screen id | stack 時○ | |
| `navigation.tabs[]` | `{screen,title,icon}` | tab 時○ | `icon` は SF Symbols 名 |
| `screens` | `{ [id]: Screen }` | ○ | **object(id をキー)** |

## 2. Screen

```json
{
  "title": "{{params.title}}",
  "scroll": true,
  "onAppear": [ { "type": "entity.list", "entity": "expense", "into": "state.expenses" } ],
  "toolbar": [ { "type": "button", "icon": "plus", "style": "plain",
                 "action": { "type": "present", "screen": "new" } } ],
  "body": { "type": "vstack", "children": [ "...Component" ] }
}
```

| キー | 型 | 既定 |
|---|---|---|
| `title` | string(式可) | "" |
| `scroll` | bool | true(`aiVoice`/全画面 map は false) |
| `onAppear` | Action[] | [] |
| `toolbar` | Component[] | [](右上に並ぶ。button 推奨) |
| `body` | Component | 必須 |

## 3. Component(全14種)

全ノード共通: `type`(必須)、`id`(任意・安定キー)、`visible`(bool or 式)、`padding`(number)。

| type | 主な props | 役割 |
|---|---|---|
| `text` | `text`*, `style`(largeTitle/title/title2/headline/body/caption/footnote), `color`(#hex/secondary/red...), `align`, `bold` | 文字 |
| `image` | `url` or `systemName`, `height`, `cornerRadius` | 画像/SF Symbol |
| `button` | `label`, `icon`, `style`(primary/secondary/destructive/plain), `action`, `disabled`(式) | ボタン |
| `textField` | `bind`*, `placeholder`, `keyboard`(number/decimal/email/url/phone), `secure`, `multiline` | 入力 |
| `toggle` | `bind`*, `label`* | スイッチ |
| `picker` | `bind`*, `label`, `options[{label,value}]`* | 選択(4件以下は segmented) |
| `vstack` / `hstack` / `zstack` | `children`*, `spacing`, `alignment` | レイアウト |
| `list` | `source`*(配列の式), `item`*(行テンプレート), `emptyText`, `onTap`, `onDelete` | 一覧。行内で `{{item.x}}` `{{index}}` |
| `card` | `title`, `children`, `action` | 角丸カード |
| `section` | `title`, `footer`, `children` | 見出し付きグループ |
| `spacer` | `minLength` | |
| `divider` | - | |
| `progress` | `value`(0..1 or 式。省略で不定), `label` | |
| `map` | `lat`*, `lng`*, `span`, `pins[{lat,lng,title}]` | MapKit |
| `camera` | `bind`*, `label` | 撮影/選択 → base64 JPEG を bind 先へ(P0) |
| `aiVoice` | `prompt` | 音声入力画面(初期状態の唯一の部品) |

(* 必須)

```json
{ "type": "list", "source": "{{state.expenses}}", "emptyText": "まだありません",
  "onTap": { "type": "navigate", "screen": "detail", "params": { "id": "{{item.id}}" } },
  "onDelete": { "type": "sequence", "actions": [
      { "type": "entity.delete", "entity": "expense", "id": "{{item.id}}" },
      { "type": "entity.list", "entity": "expense", "into": "state.expenses" } ] },
  "item": { "type": "card", "children": [
      { "type": "hstack", "children": [
          { "type": "text", "text": "{{item.title}}", "style": "headline" },
          { "type": "text", "text": "¥{{item.amount}}", "align": "trailing" } ] },
      { "type": "text", "text": "{{item.date}}", "style": "caption", "color": "secondary" } ] } }
```

## 4. Action

| type | フィールド | 実行場所 |
|---|---|---|
| `navigate` | `screen`*, `params` | iOS |
| `back` | - | iOS |
| `present` | `screen`*, `params`, `style`(sheet/fullScreen) | iOS |
| `dismiss` | - | iOS |
| `setState` | `path`*, `value` | iOS |
| `entity.list` | `entity`*, `into`*, `query` | appconfig |
| `entity.create` | `entity`*, `data`* | appconfig |
| `entity.update` | `entity`*, `id`*, `data`* | appconfig |
| `entity.delete` | `entity`*, `id`* | appconfig |
| `native` | `feature`*, `args`, `into` | iOS(camera/notification/haptic/location...) |
| `openURL` | `url`* | iOS |
| `ai.prompt` | `text`* | ai |
| `sequence` | `actions[]`* | 直列。1つ失敗したら中断せず続行(v0) |

```json
{ "type": "sequence", "actions": [
  { "type": "entity.create", "entity": "expense",
    "data": { "title": "{{state.draft.title}}", "amount": "{{state.draft.amount}}" } },
  { "type": "setState", "path": "state.draft", "value": { "title": "", "amount": "" } },
  { "type": "entity.list", "entity": "expense", "into": "state.expenses" },
  { "type": "native", "feature": "notification.schedule",
    "args": { "title": "家計簿", "body": "記録しました", "inSeconds": 5 } },
  { "type": "dismiss" } ] }
```

## 5. バインディング式

| 形 | 結果 |
|---|---|
| `"{{state.expenses.count}}"` | 単独式 → 型付き値(number 2) |
| `"¥{{item.amount}}"` | 埋め込み → 文字列 |
| `"{{expenses}}"` | root 省略 = state |
| ルート | `state` / `item` / `index` / `params` / `user`(`user.uid` 等) |
| 配列 | `.0` 添字、`.count` |
| 未定義 | `null`(文字列中は空文字) |

v0 に**無い**もの: 演算・比較・否定・フォーマット(`sum`, `==`, `!`, 日付整形)。必要なら backend が計算済みフィールド(`summary.total` 等)を返す方が速い。

## 6. appconfig REST(iOS が叩く想定)

| メソッド | パス | 用途 |
|---|---|---|
| GET | `/v1/apps/{appId}` | 最新 DSL(`version` 含む)。ポーリングにも使う(`ETag` 歓迎) |
| GET | `/v1/apps/{appId}/entities/{entity}/records?k=v` | 一覧 → `{ "items": [ {...} ] }` |
| POST | `/v1/apps/{appId}/entities/{entity}/records` | 作成。body = data。応答 = 作成後レコード(`id`,`createdAt` 付与) |
| PATCH | `/v1/apps/{appId}/entities/{entity}/records/{id}` | 部分更新 |
| DELETE | 同上 | 削除 |
| POST | ai `/v1/morph` `{appId, currentVersion, transcript}` | 応答 = 適用後の最新 DSL 全体 |

全リクエストに `Authorization: Bearer <Firebase ID token>`。レコードは `uid` で所有者分離。

## 7. 譲れない / 柔軟

| 区分 | 項目 | 理由 |
|---|---|---|
| **譲れない** | 全ノードが `type` 文字列を持つ(1階層。`{"text": {...}}` 形式は不可) | Codable の分岐が1箇所で済む。未知 type を安全に無視できる |
| **譲れない** | `version` が単調増加の int | ホットリロードと `id(version)` アニメの基準 |
| **譲れない** | 未知の `type`/キー/enum 値があってもエラーにしない前提(iOS は無視する) | AI 出力の揺れ対策。backend は厳格検証してよいが、iOS が受ける JSON に「未知」が混ざっても落ちない |
| **譲れない** | 式は `{{...}}` のドットパス。ルート名 `state/item/index/params/user` | 実装済み。構文を増やす場合は追加のみ |
| **譲れない** | ユーザーデータは `entity.*` 経由でサーバーへ、iOS は `into` の state にキャッシュするだけ | 決定事項(1) |
| **譲れない** | iOS は JSON Patch を受けない。常に完全な DSL を受け取る | 決定事項(2) |
| 柔軟 | `screens` が object か array(`[{id,...}]`) | デコーダ1箇所の変更 |
| 柔軟 | props をフラットにするか `props: {}` に包むか | 包む場合は `Component.init` で1段潜るだけ。**フラット推奨**(Claude が書きやすい・短い) |
| 柔軟 | キー命名(`camelCase` 推奨。snake なら `keyDecodingStrategy`) | |
| 柔軟 | `children` の名前、`item` → `itemTemplate` 等 | |
| 柔軟 | REST パス・レスポンス包み(`items` の名前)・ETag | `HTTPAppConfigClient` のみ変更 |
| 柔軟 | コンポーネント追加(`datePicker`, `chart`, `stepper`, `avatar` 等) | enum に case 追加 + View 1個 |
| 柔軟 | action 名(`entity.list` vs `entities.list` vs `fetch`) | 文字列 switch の変更のみ |
| 柔軟 | `theme` の拡張(フォント・角丸・ダーク固定) | |

## 8. すり合わせの論点(backend と Day1 昼に決める)
1. `screens` object/array、props フラット/包み、キー命名 → **iOS 案(object・フラット・camelCase)で揃えたい**
2. 集計値(合計・件数以外)をどちらが計算するか → backend が `summary` を返す提案
3. `entity.list` の `query` 仕様(フィルタ/ソート/limit)と `items` レスポンス形
4. `POST /v1/morph` が同期で最新 DSL を返すか、`202` + ポーリングか(Claude 生成は 5〜20秒かかる。**Cloud Run のリクエストタイムアウトを 60s 以上にして同期返却**を希望)
5. `user` ルートに何を入れるか(`uid`, `displayName`)— ID トークンの claims からサーバーが埋めるか iOS が埋めるか
6. ホットリロード: ポーリング(iOS 実装済み)で良いか、Firestore onSnapshot を Day2 に試すか
7. 画像(base64 in record)を Firestore に入れて良いか、サイズ上限
