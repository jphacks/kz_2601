# アプリの一覧の「ひと目の数字」(glanceStat・link-10)

アプリの一覧のカードに、アプリの中の数字を 1 つ出す(例「今月 ¥38,200」)。サーバーが持つのは**どの数字を出すか**(`glanceStat`)だけで、数字そのものは端末が数える。選んだ数字はアカウントに残るので、同じ人の別の端末でも同じものが出る。

## 目次
- [1. 保存](#1-保存)
- [2. いつ変わるか](#2-いつ変わるか)
- [3. API](#3-api)
- [4. 各 OS の見せ方](#4-各-os-の見せ方)

## 1. 保存

| 置き場 | 中身 |
|---|---|
| `users/{uid}/apps/{appId}` の `glanceStat`(文字列) | 出す `stat` 部品の id・`"-"`(出さない)。既定のアプリは項目ごと無い |

- 値は 3 通り(`store.App.GlanceStat`)。

  | 値 | 意味 |
  |---|---|
  | 無い(`""`) | 既定。画面の順で最初の `stat`(初期画面 → タブの順 → 残りの画面) |
  | `"-"` | 出さない |
  | 部品の id(`^[a-z][a-z0-9_-]{0,39}$`) | その `stat` |

- 既定に戻す時は項目を消す(Firestore は `firestore.Delete`・メモリの store は `""`)。
- サーバーは**形だけ**確かめる(`validGlanceStat`・DSL の部品の id と同じ形)。DSL に無い id でも受け取り、端末が既定に戻す。DSL を直して部品が消えても書き直さない。
- 数字の値はサーバーに置かない。端末が自分の記録で数え、端末に残す(下の「各 OS の見せ方」)。

## 2. いつ変わるか

- 変えるのは `PATCH /v1/apps/{appId}` に `glanceStat` を送った時だけ。
- DSL の書き込み(新しい版)・名前の変更・並べ替え・アーカイブでは変わらない。
- `glanceStat` を変えても `currentVersion` も `updatedAt` も進めない(並べ替えの `sortIndex` と同じ・`store.SetAppGlance`)。一覧の「最後に変えた日」が数字の選び直しで動かないようにするため。
- アプリを消すと(`DELETE /v1/apps/{appId}`)、文書ごと消える。

## 3. API

[`openapi-appconfig.yaml`](../openapi-appconfig.yaml) の tag `apps`。

| メソッド | パス | 中身 |
|---|---|---|
| GET | `/v1/apps`・`/v1/apps/{appId}` | `AppSummary.glanceStat`。既定(項目が無い)の時は返さない |
| PATCH | `/v1/apps/{appId}` | 本文 `{"glanceStat": "<id>" \| "-" \| ""}`。`""` で既定に戻す。答えは `AppSummary` |

- 形が違う値は 400 `bad_request`(`glanceStat must be a component id, "-" or empty`)。無いアプリは 404。
- `name`・`status` と一緒に送ってよい。`glanceStat` だけなら版は積まない。
- 実装: `backend/internal/appconfig/apps.go`(`patchApp`・`validGlanceStat`)・`backend/internal/store/firestore.go` / `memory.go`(`SetAppGlance`)。

## 4. 各 OS の見せ方

| OS・場所 | 今 |
|---|---|
| iOS の変身一覧のカード | 出す。カードの 2 行目に「ラベル 値」。1 時間より古い値は薄く出し、まだ数えていないカードはスケルトン。カードの「…」のメニュー「一覧に出す数字を選ぶ」で選び直す(「おまかせ(最初の数字)」・画面の順の `stat`・「出さない」) |
| Mac のサイドバー | まだ出さない(`ios/MetamonMac` は `glanceStat` を読まない)。メニューバーは別の決まりで、前にあるアプリの先頭 3 個の `stat` を出す(`MacMenuBarStats`) |
| ウィジェット(iOS・Mac) | `glanceStat` を使わない。DSL の先頭 3 個の `stat` を出す(`WidgetSnapshotWriter`)。数え方は同じなので、同じ部品なら数字は一致する |
| Android・デスクトップ | まだ出さない(`glanceStat` を読まない) |

iOS の数え方(`ios/Metamon/App/AppGlance.swift`・`AppRuntime+Glance.swift`):

- 数えるのは端末の中だけ。使用中のアプリは画面の記録、ほかは端末に残っている DSL と記録で数える。一覧を開くたびに記録の API は呼ばない。
- 数え方はウィジェット(`WidgetSnapshotWriter.statText`)と同じなので、アプリの中の `stat` と同じ数字になる。
- 暗号化した項目を使う `stat` は数えず「🔒」を出す(一覧の数字は端末に平文で残るため)。
- その端末でまだ開いていないアプリは記録が空なので、「0 件」と言い切らず前の値のまま(無ければ出さない)。
- 数字は端末(`UserDefaults` の `metamon.appGlance.v2`)に残し、次に一覧を開いた時はすぐ出す。同じアプリは 60 秒の間は読み直さない。サインアウト・アカウント削除で消し、アプリを消すとその分を消す。
- 選び直しは先に一覧に出し、保存に失敗したら元に戻して知らせる。
- ショートカット(`StatIntents`)は、端末に記録が無い時、同じ部品ならこの数字を使う。
