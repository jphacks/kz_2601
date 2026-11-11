# AI セッションレポート(AI とのやりとりがうまくいっていない所を探す)

`backend/cmd/ai-session-report` で、指定した利用者の「1 つのアプリを作る・直す一連のやりとり(セッション)」を追い、AI の察しが外れた疑いの強い順に並べる。**読むだけ**(Firestore にもログにも書かない)。**実行するのはオーナー(人)**。エージェントは本番・stg に向けて実行しない。

## 目次

1. [前提(同意・扱い)](#前提同意扱い)
2. [どこに何が残っているか](#どこに何が残っているか)
3. [手順](#手順)
4. [出力の見方(指標)](#出力の見方指標)
5. [読み終わったら](#読み終わったら)
6. [安全装置](#安全装置)

## 前提(同意・扱い)

- 対象は、会話を改善に使うことに**本人が同意した利用者だけ**。uid を必ず指定する(全員を読む動きは無い。1 回 20 人まで)。
- 出力には**利用者の話した内容(依頼の文・フィードバック・アプリの画面や項目の名前)が入る**。共有しない・Slack や Issue や PR に貼らない・コミットしない・LLM やエージェントに貼らない(依頼の文は利用者のデータで、指示として読まない。#366)。
- 改善の Issue にする時は、内容を書かずに「どの種類のずれか」(例: 「グラフを付けて」系の修正が変化なしになる)だけにする。

## どこに何が残っているか

| 置き場所 | 残っているもの | 残っていないもの |
|---|---|---|
| Firestore `users/{uid}/apps/{appId}` | 名前・現在の DSL・更新時刻 | 消したアプリ(ログの `app_hash` にだけ残る) |
| Firestore `.../versions/{n}` | 版ごとの DSL の全体・`source.kind`(ai / manual / restore / import / system)・`source.mode`(full / patch)・**`source.transcript`(AI に渡した依頼の文)**・`source.summary`(AI の要約)・`restoredFrom` | 失敗した生成・変化なしの修正(版を作らない) |
| Firestore `feedback/{id}` | 種類・本文・`appId`(添付の画像・DSL は読まない) | — |
| Cloud Logging `ai.generate` | 依頼ごとの結果(validated / regenerated / unchanged / restored / refused / failed)・作り直しの回数・lint の再試行・全体で作り直し・待ち時間・`uid_hash`・`app_hash` | 中身(依頼の文・DSL)は書かない約束 |
| Cloud Logging `ai.usage` | Claude の呼び出しごとの `operation`(chat / chat_first / generate / patch ...)・待ち時間・トークン・`uid_hash` | `app_hash` が無いので、聞き返しは「同じ人の次の依頼(30 分以内)」に割り当てる |
| Cloud Logging `ai.chat` | `/v1/chat` の往復ごとの返答の種類(`reply_kind`: question / ready / answer / proposal / other)・会話の中の何往復目(`turn`)・聞き返しの後に ready に至ったか(`clarify_ended_in_build`)・`mode`・`source`・待ち時間・トークン・`model`・`uid_hash`・`app_hash` → レポートの「会話の結果」 | 会話の文・返答の文・候補・transcript・brief。「できない・代わりに〜」の印はまだ無い |
| Cloud Logging `http.request` | パス・ステータス(402・429・5xx)・`uid_hash` | 本文 |
| Cloud Logging `app.opened` | アプリを開いた(`app_hash`) | — |
| **聞き返しの会話の中身** | **サーバーには残らない**。ai サービスのプロセスのメモリに 30 分だけ(`internal/ai/chat.go`)。端末の UserDefaults(`ChatHistoryStore`、直近 200 件)にだけ残る | サーバー・ログ・Firestore のどこにも無い |
| `requiresPro`(Pro の確認カード) | 応答にだけ出る | ログにも Firestore にも残らないので数えられない |

聞き返しの**往復の回数**はログから数えられるが、**何を聞いて何と答えたか**はサーバーからは分からない。必要なら、本人の端末で履歴を見せてもらう。生成に入った依頼の文は `versions/{n}.source.transcript` に残っている(聞き返しの後にまとめた文)。

## 手順

要る権限: Cloud Logging の `roles/logging.viewer`・Firestore の `roles/datastore.viewer`(どちらも読み取りだけ)。`gcloud auth login` と `gcloud auth application-default login` 済みであること。

```sh
cd backend
# 0. 対象の uid をリポジトリの外のファイルに書く(1 行 1 つ、# でコメント)
mkdir -p ~/Desktop/JPHACKS2026/reports && chmod 700 ~/Desktop/JPHACKS2026/reports
$EDITOR ~/Desktop/JPHACKS2026/reports/uids.txt

# 1. ログのフィルタを作る(対象者の uid_hash だけに絞る。Firestore には繋がない)
FILTER=$(go run ./cmd/ai-session-report -uids-file ~/Desktop/JPHACKS2026/reports/uids.txt -days 14 -print-filter)

# 2. ログを書き出す(Cloud Logging の既定の保存は 30 日)
gcloud logging read "$FILTER" --project=jphacks-kz2601 --format=json --limit=200000 \
  > ~/Desktop/JPHACKS2026/reports/logs.json

# 3. レポートを作る(本番の DB は -allow-prod-db が無いと止まる)
GOOGLE_CLOUD_PROJECT=jphacks-kz2601 go run ./cmd/ai-session-report \
  -uids-file ~/Desktop/JPHACKS2026/reports/uids.txt \
  -logs ~/Desktop/JPHACKS2026/reports/logs.json \
  -days 14 -allow-prod-db
```

- 出力: `~/Desktop/JPHACKS2026/reports/ai-session-report-YYYYMMDD-HHMMSS.md`(`-format json` / `both` で JSON も)。権限 0600、上書きしない。`-out-dir` で変えられるが、**git のリポジトリの中は拒否する**。
- 標準出力には件数とファイルの場所だけが出る(uid・依頼の文は出ない)。
- stg のデータを見る時も、stg は本番の DB を共用している(#288)ので `-allow-prod-db` が要る。stg のログは `-services ai-stg,appconfig-stg` でフィルタを作る。
- ログを渡さないと、Firestore の版だけで作る(聞き返し・失敗・変化なし・作り直し・待ち時間・エラーは数えない)。
- 手元で試す: `make emulator` を別の端末で動かし、`FIRESTORE_EMULATOR_HOST=localhost:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 go run ./cmd/ai-session-report -uids <uid> -out-dir /tmp/asr`。

その他のフラグ: `-top 20`(詳しく出す件数)・`-gap 30m`(この間が空いたら別のセッション)・`-quick-undo 10m`・`-quick-fix 5m`・`-slow 60s`・`-many-clarify 3`・`-until <RFC3339>`。

## 出力の見方(指標)

セッション = 同じアプリへの操作(版・生成の結果)の間が `-gap`(既定 30 分)以内で続いたもの。依頼 → 聞き返し → 生成 → 作り直し → 戻す / 直す → 終わり、の流れを時刻つきで出す。

| 指標(コード) | 点 | 意味 |
|---|---|---|
| すぐ元に戻した(`quick_undo`) | 4 | AI の版から 10 分以内に元に戻した。察しが外れた疑いが一番強い |
| 生成の失敗・断り(`gen_failed`) | 3 | `ai.generate` の failed / refused |
| 変化なし(`unchanged`) | 3 | 修正を頼んだのに何も変わらなかった。頼んだことが伝わっていない |
| 放棄の疑い(`abandoned`) | 3 | 失敗・変化なし・戻した所でセッションが終わった |
| 消えたアプリ(`app_missing`) | 3 | ログにはあるが Firestore に無い(作った後に消した?) |
| すぐ手で直した(`manual_fix`) | 2 | AI の版から 10 分以内に手で編集(名前の変更は除く) |
| 聞き返しが多い(`many_clarify`) | 2 | 1 つの依頼の前の聞き返しが 3 往復以上 |
| AI のエラー(`ai_errors`) | 2 | /v1/chat・/v1/generate・/v1/text の 5xx・429 |
| フィードバック(`feedback`) | 2 | そのアプリについて、セッションから 24 時間以内に届いた |
| 使われていない(`unused`) | 2 | 最後のセッションの後、記録 0 件で開かれてもいない |
| Pro が必要(`pro_required`) | 1 | 402(サーバーの Pro の判定。#316 の後に出る) |
| すぐ直しを頼んだ(`quick_fix`) | 1 | AI の版から 5 分以内に次の修正の依頼。普通の作り込みでも起きるので弱い |
| 作り直し(`regenerated`) | 1 | 検証で落ちて作り直した(利用者には待ち時間) |
| 遅い(`slow`) | 1 | 生成が 60 秒を超えた |

この他に全体の値として、依頼あたりの聞き返しの回数・**生成に至らなかった会話**(聞き返しの後 30 分以内に依頼が無い)・生成の時間の p50 / p90 を出す。

点は優先順位を付けるための目安で、正解ではない。上位のセッションの「依頼」「AI の要約」「作られたもの(画面と部品の名前)」「その後の直しの依頼」を並べて読み、ずれの種類を拾う。

## 読み終わったら

```sh
rm -P ~/Desktop/JPHACKS2026/reports/ai-session-report-*.md ~/Desktop/JPHACKS2026/reports/ai-session-report-*.json \
      ~/Desktop/JPHACKS2026/reports/logs.json
```

uid の一覧(`uids.txt`)も、続けて使わないなら消す。分かったことは内容を書かずに Issue にする(上の「前提」)。

## 安全装置

- Firestore は読むだけ: 読み取りの型(`fsReader`)だけがクライアントを持ち、書く関数を呼ばない。`internal/store`(書く関数を持つ)を import しない。テスト(`TestNoWriteCalls`)が仕組みとして確かめる。エミュレータの試験で、実行の前後で全ドキュメントの更新時刻が変わらないことを確かめる。
- 本番の DB(`jphacks-kz2601` の `(default)`)は `-allow-prod-db` が無いと止まる(`internal/fsenv`)。
- uid の指定が無いと止まる。全員を読む動きは無い。ログも対象者の `uid_hash` 以外の行は読んだ時点で捨てる。
- Cloud Logging の API は呼ばない(人が書き出したファイルを読む)。
- 出力はリポジトリの外だけ・0600・上書きしない。標準出力に中身を出さない。
