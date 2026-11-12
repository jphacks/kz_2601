# 自己改善ループ(#364)

> [バックエンド設計](../architecture.md) から分けた機能の節。決めたこと: [#365 全体](../../decisions/365-self-improvement-loop.md)・[#366 紛れ込む指示](../../decisions/366-feedback-prompt-injection.md)・[#367 個人情報](../../decisions/367-feedback-pii.md)。エージェントの PR の手順は [feedback-loop-agent-pr.md](feedback-loop-agent-pr.md)。

## 目次

1. [流れ](#流れ)
2. [守っていること](#守っていること)
3. [置き場所](#置き場所)
4. [コマンド](#コマンド)
5. [環境変数](#環境変数)
6. [アプリで見る](#アプリで見る)
7. [振り分けの精度を測る](#振り分けの精度を測る)
8. [運用の手順](#運用の手順)
9. [ループを始める前に人がすること](#ループを始める前に人がすること)
10. [まだ無いもの](#まだ無いもの)

## 流れ

```mermaid
flowchart LR
  F[(feedback/{id}<br/>アプリ内 #319)] --> I
  R[App Store の<br/>レビュー] --> I
  C[クラッシュの急増<br/>手で記録] --> I
  TF[TestFlight の<br/>フィードバック] --> I
  GF[生成の失敗の急増<br/>ログから] --> I
  I[ingest<br/>伏せ字・同意の版] --> X[(loop_inbox)]
  X --> T[triage<br/>LLM・道具なし・JSON だけ]
  T --> X
  X --> D[draft<br/>決まった形・出口の検査]
  D --> Q[(loop_drafts<br/>非公開の待ち行列)]
  Q --> H{{人: drafts approve<br/>承認 1}}
  H --> G[GitHub の Issue<br/>要約だけ]
  G --> A{{人: agent:ready<br/>#370}}
  A --> P[エージェントの PR]
  P --> M{{人: レビューとマージ<br/>承認 2}}
  M --> N[fixed<br/>お知らせ・リリースノートの下書き]
```

自動なのは「取り込み → 振り分け → 下書き」と「直ったお知らせ」だけ。**Issue を作るのは人の `drafts approve` だけ**で、エージェントの PR を起動するのも、マージするのも人。定期の実行(Cloud Scheduler・workflow)はまだ足していない([まだ無いもの](#まだ無いもの))。

## 守っていること

| 決まり | どこで |
|---|---|
| 公開リポジトリの Issue にフィードバックの原文・個人情報・内部の ID を載せない | 下書きは要約と件数だけの決まった形(`internal/loop/draft`)。出口の検査(`draft.Check`)を保存の時・承認の時・投稿の直前に通す。原文と 16 文字以上一致したら出さない |
| 伏せ字は保存の時に、LLM の前で | `internal/loop/pii`(決まった形: メール・電話・URL・秘密・番号・住所・位置・ID・名前の敬称の形・金額)。`loop_inbox` には伏せ字済みの文しか無い |
| 同意の版で AI に回すかを決める | アプリ内のフィードバックは `LOOP_CONSENT_VERSIONS` に入った版だけ。既定は空 = アプリ内のフィードバックは AI に回らない。ストアのレビュー(公開の文)とクラッシュは同意の版を見ずに AI に回す(2026-11-10 決定) |
| 伏せ字が多すぎる・出力が外れたら人へ | 本文の 3 割を超える伏せ字・スキーマや検査器 1 に外れた出力・個人情報の疑いは `held`(週の手作業の振り分けへ) |
| 振り分けの LLM は読んで分類を返すだけ | 道具を渡さない(`ai.Completion` に道具の口が無い)。呼び出しごとの乱数の区切りの中にデータとして入れ、`<` `>` は全角にする。出力はスキーマ + 許可された値 + 要約の検査(URL・メンション・コード・HTML・ラベルらしい文字・個人情報) |
| 段ごとに止められる・既定は止まっている | `LOOP_<段>=on` の時だけ動く。`loop_control/switches` の `all` / 段が `false` なら止まる。読めなければ止まる |
| 何が起きたか後からたどれる | すべての段が `loop_audit` に記録(ID と理由のコードだけ)。書けなければ段を止める。`feedbackId` と Issue の対応はここにだけある |
| 承認の後に書き換わったら出さない | 承認した題名と本文のハッシュを残し、投稿の直前に比べる |
| 費用の上限 | 振り分けは 1 回の件数の上限(`-max`)と、月のトークンの上限(`LOOP_MONTHLY_TOKEN_CAP`)。上限に届いたら次の呼び出しの前に止まる |

## 置き場所

書くのは `backend/cmd/feedback-loop` だけ。アプリは Firestore を直接読まない(Firestore のルールで許可しないこと)。`loop_notices` だけは appconfig の API(`GET /v1/me/notices`・本人の分だけ)を通して読み、既読の印(`readAt`)を付ける。

保存期間: **元のフィードバック(`feedback/{id}`)は 180 日**(同意文に書いた期間・`store.FeedbackRetention`。変えない)。**ループの写しはすべて 90 日**(`expiresAt`)。写しは元より長くは残さない(元の残りが 90 日より短ければ元と同時に消える)。

| 集まり | 中身 | 消えるとき |
|---|---|---|
| `loop_inbox/{lin_…}` | 伏せ字済みの本文・出どころ・版・OS の大きい版・スクショ/DSL の「あり」の印・振り分けの結果。アプリ内は `uid`(LLM には渡さない) | 90 日 / 元の `feedback` が消えた時 / アカウントを削除した時(`purge`) |
| `loop_drafts/{drf_…}` | Issue の下書き(題名・本文)・状態・承認した人・本文のハッシュ・Issue と PR の番号 | 90 日 |
| `loop_notices/{ntc_…}` | 「あなたの声で直りました」(決まった文 + 承認された題名)・push したか・既読(`readAt`・アプリが付ける) | 90 日 / アカウントを削除した時 |
| `loop_audit/{aud_…}` | 段・操作・人・ID・理由のコード・トークン数 | 90 日 |
| `loop_control/switches` | 段ごとの停止 | — |
| `loop_control/usage-YYYY-MM` | 振り分けのトークン数 | — |
| `loop_control/cursor-{source}` | 取り込みの続きの位置 | — |

スクショと DSL はループに入れない。クラッシュは種類・件数・版だけ。生成の失敗はコードと件数だけ。ストアのレビューはニックネームと国を捨てる。TestFlight はコメント・ビルド番号・OS の大きい版・日時だけを API に頼む(メール・名前・テスター・端末・スクショ・クラッシュログは取らない)。

## コマンド

`backend/` で `go run ./cmd/feedback-loop <command>`(ローカルは `make -C backend run-feedback-loop LOOP_ARGS="..."` でエミュレータに向く)。

| コマンド | 段 | すること |
|---|---|---|
| `ingest [-source in_app\|app_store_review\|testflight\|gen_failure]` | intake | 新しいフィードバックを伏せ字して `loop_inbox` へ。ストアのレビューは `LOOP_SOURCE_APPSTORE=on`、TestFlight は `LOOP_SOURCE_TESTFLIGHT=on`、生成の失敗の急増は `LOOP_SOURCE_GENFAIL=on` の時だけ |
| `inbox [-source S] [-status S] [-limit N]` | (止めない・読むだけ) | すべての出どころ(アプリ内・ストアのレビュー・クラッシュ・TestFlight・生成の失敗)を 1 つの一覧で見る(#368)。ID・出どころ・状態・`held` の理由・アプリの版・OS・届いた日時だけ出し、**本文は出さない**。取り込んだ古い順。`-source` は `in_app` / `app_store_review` / `crash` / `testflight` / `gen_failure`、`-status` は `new` / `held` / `triaged` / `spam` / `drafted`、`-limit` は既定 100(最大 2000) |
| `ingest-crash -issue ID -type T -count N -version V` | intake | Crashlytics の急増を 1 件として記録(人が Crashlytics の画面を見て打つ) |
| `triage [-max 200] [-batch 20]` | triage | 新しい項目を分類(`bug` / `feature` / `question` / `praise` / `spam`)・束ね・優先度(P1〜P3)付け。同じ文は 1 回だけ送る |
| `triage-score -labels FILE` | — | 人が付けた正解(1 行 `itemId,category`)との一致率(#376 の完了の条件: 8 割) |
| `triage-eval -set FILE [-model rules\|ai]` | (`ai` の時だけ triage) | 正解付きの組を、メモリの中だけで振り分け → 下書きまで通し、分類・優先度・束ねの一致率を出す([振り分けの精度を測る](#振り分けの精度を測る))。Firestore には書かない |
| `draft [-max-new 20]` | draft | 束ごとに下書きを作る / 開いている下書きに足す。束にもう出した Issue(まだ直っていないもの)があれば、その Issue への追記の下書きにする([既存の Issue への追記](#既存の-issue-への追記)) |
| `drafts list [-status pending]` / `show <id>` | — | 待ち行列を見る。`show` は出口の検査の結果と、承認の前に見る点を出す |
| `drafts edit -by NAME [-title T] [-body-file F] <id>` | — | 再現の手順などを人が書く(出口の検査を通す) |
| `drafts approve -by NAME <id>` | publish | **承認 1**。`LOOP_ISSUE_REPO` があれば Issue を作る(追記の下書きはその Issue へのコメント)。無ければ dry run(作られる Issue / コメントを表示するだけ) |
| `drafts reject -by NAME [-reason R] <id>` / `publish <id>` | publish | 却下 / dry run の後で投稿 |
| `fixed [-dry-run]` | notify | 出した Issue がマージされた PR で閉じたかを見て、送り主にお知らせを作り、リリースノートの下書きを表示する |
| `notices -uid UID` | — | ある人のお知らせを見る |
| `purge [-dry-run]` | (止めない) | 期限切れ・元が消えた写し・削除したアカウントの写しを消す |
| `switch show` / `switch set -by NAME <stage\|all> <on\|off>` | — | 停止スイッチ |
| `audit [-since 168h] [-draft ID] [-item ID] [-summary]` | — | 監査ログ。`-summary` は週の振り返り(段と操作ごとの件数・トークン) |

段: `intake` / `triage` / `draft` / `publish` / `notify`。

## 環境変数

秘密の値は Secret Manager から Cloud Run の環境変数に入れる(リポジトリ・ログに置かない)。

| 変数 | 既定 | 意味 |
|---|---|---|
| `GOOGLE_CLOUD_PROJECT` / `FIRESTORE_DATABASE` / `APP_ENV` | — | 接続先([internal/fsenv](../../../backend/internal/fsenv)) |
| `LOOP_INTAKE` / `LOOP_TRIAGE` / `LOOP_DRAFT` / `LOOP_PUBLISH` / `LOOP_NOTIFY` | 空 = 止まっている | `on` の時だけその段が動く |
| `LOOP_PAUSED` | 空 | `1` で全段を止める |
| `LOOP_CONSENT_VERSIONS` | 空 | AI に回してよいアプリ内の同意の版(カンマ区切り) |
| `LOOP_AI_SOURCES` | `in_app,app_store_review,crash` | AI に回してよい出どころ。既定はすべて(ストアのレビューも回す・2026-11-10 決定)。外した出どころは `held`(`source_not_allowed`)。アプリ内は同意の版の確認も通す |
| `LOOP_ANTHROPIC_API_KEY` | — | 振り分け専用のキー(アプリの `ANTHROPIC_API_KEY` とは分ける) |
| `LOOP_TRIAGE_MODEL` | `claude-haiku-5-5` | 振り分けのモデル |
| `LOOP_MONTHLY_TOKEN_CAP` | `3000000` | 振り分けの月のトークンの上限 |
| `LOOP_SOURCE_APPSTORE` | 空 | `on` でストアのレビューを取り込む |
| `LOOP_SOURCE_TESTFLIGHT` | 空 | `on` で TestFlight のフィードバック(スクショ付き・クラッシュの送信)を取り込む。キーは下の `LOOP_ASC_*` を共用。既定の `LOOP_AI_SOURCES` に入っていないので、AI に回すなら `testflight` を足す |
| `LOOP_SOURCE_GENFAIL` | 空 | `on` で生成の失敗の急増を取り込む。`LOOP_GENFAIL_LOGS`(`gcloud logging read --format=json` で書き出したファイル・カンマ区切り。`cmd/ai-session-report -logs` と同じ形)の `ai.generate` の行から、失敗のコード(`outcome:failed` / `outcome:refused` / `error:<error_code>` / `retry:<retry_reasons>` / `validation:<validation_codes>`)ごとに、直近 `LOOP_GENFAIL_WINDOW`(既定 `24h`)の割合をその前の `LOOP_GENFAIL_BASELINE`(既定 `168h`)と比べる。件数が `LOOP_GENFAIL_MIN_COUNT`(既定 5)以上・割合が `LOOP_GENFAIL_MIN_RATE`(既定 0.05)以上・基準の `LOOP_GENFAIL_FACTOR`(既定 2)倍以上なら、コード 1 つ・1 日に 1 件。項目はコードと件数だけ(入力文・uid・アプリの id・ハッシュは持たない)。既定の `LOOP_AI_SOURCES` に入っていないので、AI に回すなら `gen_failure` を足す |
| `LOOP_ASC_ISSUER_ID` / `LOOP_ASC_KEY_ID` / `LOOP_ASC_PRIVATE_KEY` / `LOOP_ASC_APP_ID` | — | App Store Connect API のキー(役割は Customer Support で足りる)。鍵は `.p8` の中身そのもの |
| `LOOP_ISSUE_REPO` | 空 = dry run | Issue を作るリポジトリ(`owner/name`) |
| `LOOP_GITHUB_TOKEN` | — | Issue の作成・コメント・読み取りだけの権限(GitHub App のインストールのトークン、または fine-grained の PAT の `issues: write`)。マージ・ラベル・コードの権限は持たせない |
| `LOOP_ISSUE_LABELS` | 空 | 作る Issue に付けるラベル(カンマ区切り)。`agent:` で始まるラベル(`agent:ready` など)が入っていたら `drafts show` / `edit` / `approve` / `reject` / `publish` は起動を拒む(人が付ける・ループは付けない) |
| `LOOP_NOTIFY_PUSH` | 空 | `on` で「直りました」を push でも送る(本人の同意 `users/{uid}/notifyConsent/_feedback_fixed` と週 1 回の上限つき) |
| `APNS_AUTH_KEY` / `APNS_KEY_ID` / `APNS_TEAM_ID` / `APNS_TOPIC` / `LOOP_APNS_ENVIRONMENT` | — | push の時だけ。runner と同じ APNs のキー |

## アプリで見る

| もの | どこ |
|---|---|
| お知らせの一覧 | iOS の設定 →「お知らせ」(`Features/Notices`)。新しい順・未読の点・引っぱって更新。一覧を閉じた時に見た分を既読にする。設定の行に未読の数のバッジ。API は `GET /v1/me/notices`・`POST /v1/me/notices/{id}/read`(`docs/backend/openapi-appconfig.yaml`・本人の分だけ。他の人の ID は 404) |
| push の同意 | フィードバックの画面の「直ったら知らせてほしい」(**既定はオフ**)。オンで送った後に `PUT /v1/me/notify-consent/feedback-fixed {enabled: true}` を送り、`users/{uid}/notifyConsent/_feedback_fixed` に入る(`fixed` が push の前に読む所)。通知の許可がまだ決まっていない時だけ OS の確認を出す(拒否済みなら出さない)。オフに戻すのはお知らせの画面のトグル |
| 審査用の版 | フィードバックと一緒に外す(`docs/ios/review-build.md`)。API は残るが、審査用の版からは呼ばない |

アプリ内のお知らせは同意なしで作る(送り主本人にだけ見える・push はしない)。

## 既存の Issue への追記

同じ束の Issue を二重に出さない(#369)。

| 場面 | 下書き | 承認したら |
|---|---|---|
| 束にまだ Issue が無い / 前の Issue は直った(`fixed` が記録した) | 新しい Issue の下書き(症状・ほかの報告・件数・再現の手順・完了の条件) | Issue を作る |
| 束に出した Issue があり、まだ直っていない | 追記の下書き(`drafts list` に `(adds to owner/name#N)`、`show` に `adds to:`)。中身は**前回の後に届いた分だけ**の件数・入口・版・OS と要約(代表 1 + 最大 3) | その Issue に**コメント**を足す(`POST /repos/{repo}/issues/{n}/comments`)。題名は出さない。監査は `issue_appended` |

- 追記の下書きも同じ決まった形・同じ出口の検査(`draft.Check`: 個人情報・URL・内部の ID・原文の一致)を、保存の時・承認の時・投稿の直前に通す。`uid`・アプリの ID・`lin_`/`fb_` は載せない。
- 既定は新しい Issue と同じ: `LOOP_ISSUE_REPO` が無ければ dry run(足されるコメントを表示するだけ)、投稿は人の `drafts approve` / `publish` だけ。
- 投稿の直前に、相手の Issue がまだ束の直っていない Issue か・`LOOP_ISSUE_REPO` と同じリポジトリかを確かめる。違えば出さない(`drafts reject` して `draft` をやり直す)。
- 直った後に同じ束が来たら、再発として新しい Issue の下書きになる。
- 追記の下書きも `fixed` の対象(Issue が閉じたら送り主にお知らせ)。リリースノートの下書きは Issue ごとに 1 行。
- GitHub の上で人が手で閉じた(直していない)Issue は、ループには分からないので追記の先に残る。その時は追記の下書きを却下する。

## 振り分けの精度を測る

#376 の完了の条件(人の判断と 8 割以上一致)を測る口は 2 つ。

| 口 | 正解 | 使う時 |
|---|---|---|
| `triage-score -labels FILE` | 本番の `loop_inbox` の項目に人が付けた分類(`itemId,category`) | 1 週間分を振り分けた後、人が同じ項目に分類を付けて比べる |
| `triage-eval -set FILE` | 手で書いた架空のフィードバックの組(JSON の行: `id` `text` `category` `priority` `bundle`) | モデル・プロンプトを変える前後。本物のフィードバックを使わずに比べられる |

`triage-eval` は本物の `Triager` と `draft.Builder` をメモリの店に対して回す。数えるもの:

- 分類: `category` が正解と同じ項目の割合
- 優先度: 下書き(= Issue に出る)の優先度が正解と同じ割合。束にならない `praise` / `spam` は項目の優先度
- 束ね: 正解で同じ `bundle` の 2 件が同じ束になった割合(together)と、違う `bundle` の 2 件が別の束になった割合(apart)

`held` や外れた出力で分類が付かなかった項目は外れとして数える。`-model rules`(既定)はキーワードの基準線(`internal/loop/triage/eval/rules.go`・費用なし)。`-model ai` は振り分けのモデルを呼ぶので、`triage` と同じく段のスイッチと月の上限を見て、トークンを数える(監査は `eval_call`)。

正解の組は `backend/internal/loop/triage/eval/testdata/labeled.jsonl`(28 件・すべて手で書いた架空の文)。テスト(`TestEvalRulesOnLabelledSet`)は基準線でこの組を回し、分類・優先度・束ねのどれかが 8 割を下回ったら落ちる。基準線の今の値は分類 26/28・優先度 27/28・束ね together 14/14・apart 364/364。

コマンドの入口は他と同じなので、接続先の環境変数は要る(読み書きはしない。ローカルは `make -C backend run-feedback-loop LOOP_ARGS="triage-eval -set ..."` でエミュレータに向く)。

```sh
go run ./cmd/feedback-loop triage-eval -set internal/loop/triage/eval/testdata/labeled.jsonl
LOOP_TRIAGE=on LOOP_ANTHROPIC_API_KEY=... \
  go run ./cmd/feedback-loop triage-eval -set internal/loop/triage/eval/testdata/labeled.jsonl -model ai
```

### 似たフィードバックの束ね

| 段 | どう束ねる |
|---|---|
| 振り分けの前 | 全角半角・大文字小文字・空白・記号を落として同じになる文を 1 つにまとめ、モデルには 1 回だけ送る(`groupItems`)。同じ判定を全件にコピーする |
| 振り分け | モデルが問題ごとの `cluster`(英小文字の短い名前)を付ける。待っている下書きの `cluster` を「既知の束」として渡し、同じ問題なら使い回させる |
| 下書き | `cluster` が同じ項目を 1 つの下書きにまとめ、件数を足す。束にもう出した Issue があれば、その Issue への追記になる([既存の Issue への追記](#既存の-issue-への追記)) |

埋め込み(ベクトルの近さ)での束ねはまだ無い([まだ無いもの](#まだ無いもの))。

### 束の優先度

項目の優先度(P1〜P3)は振り分けの時に 1 件ずつ付く(`triage.Priority`: 分類・影響・領域・同じ文の件数)。下書き(= Issue)の優先度は束の全部の項目から付け直す(`draft.BundlePriority`):

| 順 | 決まり |
|---|---|
| 1 | 束の分類・領域(代表の項目の)、束の項目の中で一番高い影響(`high` > `medium` > `low`)、束の件数で `triage.Priority` を計算する |
| 2 | 件数が `draft.ManyReports`(10)以上なら 1 段上げる。P1 より上にはしない |
| 3 | `spam` と `praise` はいつも P3 で、上げない(そもそも下書きにならない) |

- 影響はモデルの `impact` を項目に残したもの(`loop_inbox` の `impact`)。代表の P3 から影響を戻すことはしない(前は `feature` の `medium` が `low` に落ち、正解の組の g01・g02 が P3 になっていた)
- `impact` を残す前に振り分けた項目は、その項目の優先度から影響を戻す(P1 → `high`・P2 → `medium`・P3 → `low`)
- 件数は束の報告の数(クラッシュの急増は 1 件の中の件数を足す)

## 運用の手順

定期の実行を有効にするまでは週 1 回、人が回す(有効にした後も `drafts` の承認・`switch`・`audit` は人が回す):

```sh
cd backend
export GOOGLE_CLOUD_PROJECT=jphacks-kz2601 LOOP_INTAKE=on LOOP_TRIAGE=on LOOP_DRAFT=on
go run ./cmd/feedback-loop switch show
go run ./cmd/feedback-loop ingest
go run ./cmd/feedback-loop inbox                     # 届いた分を出どころを問わず 1 つの一覧で(本文は出ない)
go run ./cmd/feedback-loop triage
go run ./cmd/feedback-loop draft
go run ./cmd/feedback-loop drafts list
go run ./cmd/feedback-loop drafts show <id>          # 出口の検査と、承認の前に見る点
go run ./cmd/feedback-loop drafts edit -by <名前> -body-file steps.md <id>
LOOP_PUBLISH=on LOOP_ISSUE_REPO=jphacks/kz_2601 LOOP_GITHUB_TOKEN=... \
  go run ./cmd/feedback-loop drafts approve -by <名前> <id>
LOOP_NOTIFY=on LOOP_GITHUB_TOKEN=... go run ./cmd/feedback-loop fixed
go run ./cmd/feedback-loop purge
go run ./cmd/feedback-loop audit -summary
```

承認(`drafts approve`)の前に人が見ること:

- 本文に、エージェントへの操作の命令(マージ・ラベル・秘密・設定)が無い
- 課金・認可・データの削除・`.github/` の作業になっていない(なっていれば却下し、人が自分で直す)
- 個人を特定できる所(名前・場所・健康・お金の具体の値・珍しい機種と言語の組み合わせ)が無い
- 再現の手順を自分の言葉で書いた

止める時: `go run ./cmd/feedback-loop switch set -by <名前> all off`(次の段の開始・振り分けの次の呼び出しから止まる)。それでも止まらない時は、Cloud Run の job の `LOOP_PAUSED=1`。

後で定期にする時のコマンド(Cloud Run の job の引数にする。workflow には足さない): `ingest` → `triage` → `draft` を 1 日 1 回、`fixed` と `purge` を 1 日 1 回。`drafts approve` は定期にしない(人の承認)。

## ループを始める前に人がすること

| すること | 理由 |
|---|---|
| 同意文の版を上げ、「伏せ字の上で AI(Anthropic・米国)が読む」「要約を公開の場に載せる」を書く。プライバシーポリシーとプライバシーの表示も直す([#367 の「ポリシーとプライバシーの表示で直すもの」](../../decisions/367-feedback-pii.md#ポリシーとプライバシーの表示で直すものループの開始の前)) | 今の同意(`feedback-2026-11`)は「第三者には渡しません」。直るまで `LOOP_CONSENT_VERSIONS` は空のまま |
| ループ専用の Anthropic の API キーと予算を作り、Secret Manager に入れる | プロダクトの費用と分ける(#365 の決定 7) |
| Issue だけを書ける GitHub のトークン(GitHub App か fine-grained の PAT)を作る | エージェントの権限と分ける |
| App Store Connect の API キー(Customer Support)を作る(ストアのレビューを取り込むなら) | 取り込みの口は作ってあり、既定は止めてある |
| Firestore のルールで `loop_*` をクライアントから読み書きできないことを確かめる。必要なら `expiresAt` の TTL を設定する | `purge` でも消えるが、二重にしておくと安全 |
| `loop_*` の集まりを、アカウントの削除(`PurgeAccount`)にも足すかを決める | 今は `purge` が、元が消えた写しと削除したアカウントの写しを消す |

## まだ無いもの

| もの | 状態 |
|---|---|
| 定期の実行(Cloud Scheduler・workflow) | 足していない。上のコマンドを job にする |
| Crashlytics を自動で読む | Crashlytics は安く読める API が無い。BigQuery のエクスポート(#296)ができたら `ingest.CrashSource` を実装する。それまでは `ingest-crash` で人が記録 |
| 生成の失敗の急増を自動で読む | 今はログを人が `gcloud logging read` で書き出して `LOOP_GENFAIL_LOGS` に渡す。Logging API か BigQuery から読む時は `ingest.GenLogSource` を実装する |
| Google の機密データの検出(#367 の選択肢 2-B) | GCP の設定が要るので後で |
| 埋め込みでの束ね(#376) | 束ねはモデルの `cluster` の名前に任せている。同じ問題に別の名前が付くと別の束になる(呼び出しをまたぐと特に)。既知の束として渡すのは待っている下書きの名前だけ |
| 本物の 1 週間分での一致率 | `triage-score` で人が測る(#376 の完了の条件) |
