# 生成前の対話フェーズ(`POST /v1/chat`・Issue #74 / G17・2026-10-24 実装)

> [バックエンド設計](../architecture.md) から分けた機能の節(旧 §6.6)。

初回生成の前に数ラリーで要件を固める。`/generate` は無変更で、iOS(#75)は `ready.transcript` を既存の create 経路に渡す。実装は `backend/internal/ai/chat.go`、eval は [ai-eval/2026-10-24-chat.md](../ai-eval/2026-10-24-chat.md)。

```mermaid
flowchart LR
  U[user: 家計簿] --> S{go-ahead 語?<br/>もう作って / お任せ / just build it}
  S -- yes & 直前の brief あり --> R0[ready source=shortcut<br/>Claude 不使用 0ms]
  S -- no --> T{ラリー ≤ 3?}
  T -- no --> F[Claude: 強制 ready]
  T -- yes --> C[Claude 構造化出力<br/>{kind,text,suggestions,brief,transcript}]
  C -- question --> Q[1 問 + 選択肢 ≤3<br/>brief/transcript は保持]
  C -- ready --> R[ready: brief + transcript]
  F --> R
  Q --> U2[user: はい] --> S
  R --> G[POST /v1/generate text=transcript]
  R0 --> G
```

| 項目 | 決定 |
|---|---|
| Claude 呼び出し | `Completion.Structured=true` で小さな `ChatSchema`(全 required・closed)を `output_config.format` に送る(DSL 用の大きなスキーマは従来どおりプロンプト埋め込み)。system prompt は固定で約 1.4K tok(キャッシュ)、user ターンに `## 表示言語(locale)` / `## ラリー n/3` / 会話全文。`max_tokens` 1024。JSON が壊れていれば 1 回だけ問題点付きで再要求、2 回失敗で 502 `generation_failed` |
| 判定基準(プロンプト) | 「目的・記録する項目・見たい一覧/集計・必要なネイティブ機能(camera/notification/calendar/map/location/share)」が推定できれば ready。常識で埋まる点(家計簿なら金額・カテゴリ・日付)は聞かない。曖昧なら**最重要の 1 点だけ**、20 字程度、選択肢 2〜3 個。transcript にユーザーが言っていない機能を足さない |
| 即 ready | `ai.wantsBuildNow`: 短い発話(ja ≤12 字 / en ≤32 字)に go-ahead 語(もう作って / それでいい / お任せ / よろしく / just build it / go ahead / that's fine …)を含む → 直前ターンで Claude が書いた brief / transcript をそのまま返す。brief がまだ無い 1 発目(「お任せで家計簿」)は Claude を 1 回呼び強制 ready |
| 上限 | Claude が質問できるのは 3 ラリー。4 ラリー目は `## 強制 ready` を付けて呼び、質問が返っても ready に寄せる(`source=forced`) |
| 会話状態 | ai プロセス内 `map[appId]`(直近 10 メッセージ・TTL 30 分・アクセス時に lazy sweep)。`messages` は全履歴でも最新 1 件でもよい(2 件以上=置換、1 件=追記)。`DELETE /v1/chat/{appId}` でクリア。**永続化(appconfig `conversations/{appId}`・再起動で続きから)は後段**(Cloud Run `min-instances=1` の単一インスタンスなら 30 分内は同じプロセスに当たる前提) |
| 既存アプリがある時 | `/chat` は呼ばれない想定(iOS が初期状態の時だけ対話 UI を出す)。`/generate` の `mode: auto` 判定は従来どおり |
| 速度(#147) | chat は生成と別設定(`AI_CHAT_MODEL` / `AI_CHAT_EFFORT` / `AI_CHAT_THINKING`、既定 Sonnet 5.5 / low / adaptive)。1 回目の呼び出しはストリーミングし、`kind=question` で `text` と `suggestions` が出そろった時点で**質問を先に返す**(スキーマ順が kind→text→suggestions→brief→transcript)。brief / transcript は裏で最後まで生成して会話に保存し、同じ appId の次のターンはそれを待つ。ready は全体が必要なので従来どおり。実測は [ai-eval/2026-10-24-chat-latency.md](../ai-eval/2026-10-24-chat-latency.md) |
| 実測(Sonnet 5.5 / low・6 ケース 8 ステップ全通過) | Claude 1 回 **p50 2.4s**(初回キャッシュ作成 7.5s を除くと 2.05〜2.6s)、即 ready は 0ms。Haiku 5.5 は速くならず(2.2 / 2.8s)英語の具体発話で質問に戻ったので採用せず |

## 環境変数

| 変数 | appconfig | ai | 既定 / 備考 |
|---|---|---|---|
| `AI_CHAT_MODEL` | - | o | 未設定 = `claude-sonnet-5-5`。生成前の対話(§6.6)だけのモデル。速さ優先なら `claude-haiku-5-5`(+`AI_CHAT_THINKING=disabled`) |
| `AI_CHAT_EFFORT` | - | o | 未設定 = `low` |
| `AI_CHAT_THINKING` | - | o | 未設定 = 送らない(adaptive)。`disabled`(Haiku)/ `between_tools`(Sonnet 5.5)で思考オフ。#147 の eval では速くならなかった |
