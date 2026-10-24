# ai eval: 生成前の対話(clarify)の待ち時間(2026-10-24・Issue #147)

`POST /v1/chat` の 1 回あたりの待ち時間を内訳まで測り、短くした記録。生 JSON は `raw/2026-10-24-chat-*-{before,early,early2}.json`(11 ケース 17 ステップ・Claude 呼び出し 15 回/周)。

```sh
cd backend
go run ./cmd/ai-eval -chat -env-file ../../.env -tag before                                              # Sonnet 5.5 / low / adaptive
go run ./cmd/ai-eval -chat -env-file ../../.env -chat-thinking between_tools -tag before                  # Sonnet 思考オフ
go run ./cmd/ai-eval -chat -env-file ../../.env -chat-model claude-haiku-5-5 -chat-thinking disabled -tag before
go run ./cmd/ai-eval -chat -env-file ../../.env -tag early                                                # 採用版(質問を先に返す)
```

## 1. 先に結論

- **staging の backend は温まっていれば 1 回約 2.3 秒**(`ai-stg` に直接 2 回 curl: question 2,281ms / ready 2,221ms、Claude がほぼ全部)。Issue の「質問 1 回 9.4 秒」は **backend の Claude 呼び出しでは説明できない**。残り約 7 秒は Cloud Run のコールドスタート(staging は `min-instances=0`)、iOS 側(初回は `DELETE /v1/chat` を待ってから `POST`、音声認識の終わり判定、読み上げ)、経路のどれか。iOS 側で区間ごとに測る必要がある(§5)
- backend で効いたのは **「質問を先に返す」**: 1 回目の呼び出しをストリーミングし、`kind=question` の `text` と `suggestions` が出そろった時点で返す。brief / transcript は裏で最後まで生成して会話に保存する。質問の「最初のトークン以降」の時間が **955ms → 340ms**。応答の中身は同じなので品質は変わらない
- 思考オフ(Sonnet `between_tools`)は速くならなかった。Haiku 5.5 は全体 p50 で 0.5 秒速いが、質問が長い・二択を一度に聞く・選択肢が 10 字を超えるなど指示の守り方が落ちる。**既定は Sonnet 5.5 / low / adaptive のまま**。`AI_CHAT_MODEL=claude-haiku-5-5 AI_CHAT_THINKING=disabled` で切り替えられる
- 目標「p50 3 秒以内」は改善前から満たしていた(p50 2.2 秒)。全ステップ 17/17 通過は全構成で同じ

## 2. 内訳(改善前・Sonnet 5.5 / low)

| 区間 | question | ready |
|---|---|---|
| 入力トークン | 約 1,450(うち system 約 1,370 はキャッシュ読み) | 約 1,480〜1,530 |
| 出力トークン p50 | 172 | 236 |
| 最初の出力トークンまで p50 | 1,046ms | 1,080ms |
| 最初のトークン以降 p50 | 955ms | 1,199ms |
| 合計 p50 | 2,087ms | 2,279ms |

- 入力は短く(生成用のカタログは chat に渡していない)、system はキャッシュされている(staging の 2 回目は `cacheReadInputTokens: 1368`)。入力を削っても効かない
- 半分は「最初のトークンまで」(API 往復+先読み)、半分は出力。question でも出力の 7 割は iOS に返さない brief / transcript(次の「もう作って」の材料)だった → ここを待たないようにした

## 3. 試した組み合わせ(Claude 1 回の ms)

| 構成 | 全体 p50 / p95 | question p50 | ready p50 | 最初のトークン p50 (Q / R) | 最初のトークン以降 p50 (Q / R) | 通過 | 品質メモ |
|---|---|---|---|---|---|---|---|
| Sonnet 5.5 / low / adaptive(改善前) | 2,237 / 2,627 | 2,087 | 2,279 | 1,046 / 1,080 | 955 / 1,199 | 17/17 | 基準 |
| Sonnet 5.5 / low / `between_tools`(思考オフ) | 2,322 / 2,930 | 2,322 | 2,244 | 803 / 793 | 1,168 / 1,231 | 17/17 | 同等。速くならない |
| Haiku 5.5 / low / `disabled` | 1,748 / 2,127 | 1,827 | 1,603 | 1,084 / 885 | 694 / 747 | 17/17 | 質問が長い(「記録したいのは種目ごとの重量・回数ですか?それとも日々の記録ですか?」)、選択肢が 10 字超(「いいえ、金額だけでいい」)、「あとで考える」のような答えにならない選択肢、言っていない「重量」を transcript に足す |
| **Sonnet 5.5 / low / adaptive + 質問を先に返す(採用)** 1 周目 | 2,225 / 7,880 | 1,867 | 2,225 | 1,478 / 991 | **345** / 1,067 | 17/17 | 同等(応答は同じもの) |
| 同 2 周目 | 2,467 / 3,011 | 1,847 | 2,577 | 1,446 / 1,240 | **340** / 1,231 | 17/17 | 同等 |

読み方:

- 採用版の question の「最初のトークンまで」が 1.45 秒に増えているのは **eval の作りの影響**: 直前の質問の裏の生成がまだ同じ接続を使っている間に次のケースが始まるので、新しい TLS 接続を張る分(約 0.4 秒)が乗る。実機ではユーザーが答えるまで数秒あるので起きない。改善前と同じ 1.0〜1.1 秒なら **question は約 1.4 秒**の見込み
- 1 周目の p95 7.9 秒は API 側の揺れ(同じ時間帯に最初のトークンまでが 7.5〜8 秒の呼び出しが 2 本、ready の 4 秒も 1 本)。2 周目は 3.0 秒
- `-chat` の表で question の in/out トークンが 0 なのは、先に返した時点では usage がまだ無いため(裏の完了時にログには出る)

## 4. 品質(改善前 → 採用版)

| 観点 | 改善前 | 採用版 |
|---|---|---|
| 聞く必要がある時に 1 問だけ聞く(家計簿×2・筋トレ・日記・a diary・A habit tracker・買い物リスト・記録したい) | 8/8 question・各 1 問・選択肢 3 個 | 8/8 同じ(質問文もほぼ同じ) |
| 具体的な時はすぐ ready(家計簿の長文・水分補給の長文・英語の筋トレ) | 3/3 | 3/3 |
| 2 ラリー目まで曖昧(記録したい → 読書 → 詳細) | question → question → ready | 同じ |
| 「もう作って」「このまま作って」(shortcut・Claude 不使用) | 2/2・transcript あり | 2/2・**裏で保存した transcript が使われる**(「買い物リストアプリを作って。品名と数量を記録して、未購入の一覧を見たい…」) |
| ready の transcript が生成に十分か | 8/8(目的・項目・一覧/集計・通知を 1 段落) | 8/8 同等 |
| 言語(英語ケース 3 本) | 質問・選択肢・transcript とも英語 | 同じ |

## 5. 9.4 秒の残りについて(backend 以外)

| 候補 | 根拠 | 対策 |
|---|---|---|
| Cloud Run のコールドスタート | staging は `--min-instances=0`(`deploy-cloudrun.yml`)。eval 初回は 7.5 秒だった(#74 の記録) | デモ前に `min-instances=1`(`docs/infra/gcp-setup.md` §手順あり)。複数インスタンスに分かれると会話状態(プロセス内)も分かれる |
| iOS の直列待ち | 新しい会話の 1 発目は `resetChat`(DELETE)を await してから POST | DELETE を待たずに POST(2 件以上の履歴は置換される契約なので不要) |
| 音声認識の終わり判定・読み上げ | 「質問 1 回」の計測にどこまで入っているか不明 | iOS で「発話確定 → POST 開始 → 応答受信 → 表示」をログに出して区間を分ける |
| ready の後の待ち | ready を受けた後は「この内容で作る」のタップ待ち | 音声だけの流れなら ready で自動的に生成へ進む案(iOS) |

## 6. 実装(`backend/internal/ai/chat.go`)

- `ChatConfig`(`AI_CHAT_MODEL` / `AI_CHAT_EFFORT` / `AI_CHAT_THINKING`)。生成の `CLAUDE_MODEL` とは独立。`Completion` に呼び出しごとの `Model` / `Effort` / `Thinking` を追加(Haiku には `fallbacks` を送らない: Haiku 5.5 はサーバー側フォールバック非対応)
- 1 回目の呼び出しは常にストリーミング。`completeFields` が JSON の途中から「値が閉じたトップレベルのキー」だけを取り出し、`kind=question` + `text` + `suggestions` がそろったら返す。ready や強制 ready は全体を待つ。JSON が壊れていた時の再要求は従来どおり同期
- 裏の生成が終わったら会話の brief / transcript を更新(その間に `DELETE` された会話には書き戻さない)。同じ appId の次のターンは最大 `CallTimeout` までそれを待つ
- レスポンスに診断用の `firstTokenMs` を追加(`openapi-ai.yaml`)。iOS の契約は変更なし
