# stg の試験データの掃除と、DB を分けた時の切り替え(#293)

本番の Firestore(`jphacks-kz2601` の `(default)`)に混ざった stg 由来の試験データを消す手順と、stg / prod の DB を分けた時(#288)の切り替え・戻し方です。

> **本書の手順はどれも人が実行します。** スクリプト(`backend/cmd/stg-cleanup`)はエージェントが作り、エミュレータでだけ試しています。stg / prod に向けての実行(ドライランを含む)はまだ一度もしていません。本番の DB への実行は**App Store の審査が終わって公開してから**、承認を得て行います。

## 目次
- [消すもの・消さないもの](#消すもの消さないもの)
- [安全装置](#安全装置)
- [掃除の手順](#掃除の手順)
  - [0. 前提](#0-前提)
  - [1. 退避を取る](#1-退避を取る)
  - [2. 試験ユーザーの uid を集める](#2-試験ユーザーの-uid-を集める)
  - [3. ドライラン](#3-ドライラン)
  - [4. 確認](#4-確認)
  - [5. apply](#5-apply)
  - [6. 後の確認](#6-後の確認)
  - [消しすぎた時](#消しすぎた時)
- [DB を分けた時の切り替え・戻し方](#db-を分けた時の切り替え戻し方)
  - [A2: stg を新しいプロジェクトへ出す(#288 の推奨)](#a2-stg-を新しいプロジェクトへ出す288-の推奨)
  - [B: つなぎに同じプロジェクトの名前付き DB `stg` を使う](#b-つなぎに同じプロジェクトの名前付き-db-stg-を使う)
  - [戻し方](#戻し方)
  - [分けた後の掃除](#分けた後の掃除)
- [エミュレータで試す](#エミュレータで試す)

## 消すもの・消さないもの

対象はフラグで選んだものだけです。「試験らしいものを全部」のような広い条件はありません。本物の利用者のデータは、その uid を自分で書かない限り対象になりません。

| フラグ | 対象 | 決め方 |
| --- | --- | --- |
| `-smoke` | `users/*/apps/{appId}`(版・記録・その uid が出した共有リンク・掲載・共有の場を含む)と `app_owners/{appId}` | appId が `cmd/smoke` の作る形 `smoke-<UNIX 秒>`・`-training`・`-walk`・`-probe`・`-noauth` に**完全に一致**するもの(`store.SmokeAppIDRe`)。`smoke-notes` のような名前は対象外。誰かが smoke の appId を開いて自分のツリーにできた写しも、同じ appId なので消える |
| `-stg-ratelimits` | `rateLimits/*` | 文書 ID が `stg_<日付>_uid_…` / `stg_<日付>_ip_…` / `stg_<日付>_global` に完全に一致するもの(`APP_ENV=stg` の回数制限)。`prod_` は対象外 |
| `-uids` / `-uids-file` | 書いた uid のデータ全部 | 退会の消去(`store.PurgeAccount`・[account-purge.md](account-purge.md))と同じ範囲 + Pro の権利と App Store の結びつき + `users/{uid}` 自体。1 回 100 件まで |

消さないもの:

- Firebase Auth のユーザー(スクリプトは Firestore だけを触る)。試験ユーザーの Auth を消す時は、人が Firebase コンソールで消す。
- 試験ユーザーが**他の人の共有アプリ**に投稿した記録。退会と同じく投稿は残し、表示名だけを消す。他の人が参加している試験ユーザーの共有アプリも残す(ログの `keptMirrors`)。
- `-uids` の uid は退会の墓標を残さず `users/{uid}` ごと消す。同じ uid で入り直すと、新しいユーザーとして使える。

## 安全装置

| 装置 | 動き |
| --- | --- |
| 既定はドライラン | 何を何件消すかを出すだけ。`-apply` を付けた時だけ消す |
| `-apply` には `-expect=<件数>` が要る | ドライランの最後の行の件数(`== targets: N`)と、apply の時に数え直した件数が違えば、何も消さずに止まる。ドライランの後にデータや uid の一覧が変わっても、確かめていないものは消えない |
| 本番の DB は `-allow-prod-db` が要る | 接続先が本番(`PROD_FIRESTORE_PROJECT` / `PROD_FIRESTORE_DATABASE`、既定 `jphacks-kz2601` / `(default)`)なら、フラグが無ければ接続する前に止まる(終了コード 2)。**分ける前は stg のデータも本番の DB にあるので、今はこのフラグが要る** |
| `APP_ENV=stg` の確認 | `APP_ENV=stg` で本番の DB を指すと、`ALLOW_SHARED_PROD_DB=1` が無ければ止まる(#290・`internal/fsenv`)。手で実行する時は `APP_ENV` を付けない |
| 対象の形の確認 | smoke の appId と stg の回数制限の ID は、消す直前にもう一度形を確かめる(形の違うものは消さない) |
| uid の形の確認 | 英数字・`_`・`-` だけ。`/` や `*` を含むもの・重複は拒否 |
| 出力 | smoke のユーザーの uid はハッシュで出す。回数制限は件数だけ出す(ID に uid が入るため) |

## 掃除の手順

### 0. 前提

- 実行する人の Google アカウントに、対象のプロジェクトの Firestore の読み書きの権限(`roles/datastore.user` 以上)がある。
- 手元の ADC で動かす: `gcloud auth application-default login`。
- 作業ツリーは最新の `develop`(または `main`)。`backend/` で実行する。
- App Store の審査が終わり、公開している。

```sh
cd backend
export GOOGLE_CLOUD_PROJECT=jphacks-kz2601
export FIRESTORE_DATABASE='(default)'
unset APP_ENV FIRESTORE_EMULATOR_HOST
```

### 1. 退避を取る

[firestore-backup-restore.md](firestore-backup-restore.md) の「長く残したいとき(GCS へのエクスポート)」で、作業の前に全体をエクスポートします。PITR(7 日)があるので、**apply を始める時刻(UTC)をメモ**しておきます([消しすぎた時](#消しすぎた時)に使う)。

### 2. 試験ユーザーの uid を集める

- 開発者が stg のビルドで使った uid を、本人に聞いて集めます。**同じ uid で本番版も使っている人の uid は書きません**(本番のデータごと消えるため。今の文書には「stg で作った」印が無い・#288 の判断の根拠 5)。
- uid の一覧はリポジトリの外のファイル(例: `~/stg-test-uids.txt`)に 1 行 1 件で書きます。`#` から後はコメント。コミットしません。

```text
# 2026-11 stg 試験ユーザー(持ち主・端末)
abcDEF123...   # 〇〇・iPhone 15
```

### 3. ドライラン

```sh
go run ./cmd/stg-cleanup -allow-prod-db -smoke -stg-ratelimits -uids-file ~/stg-test-uids.txt | tee ~/stg-cleanup-dryrun.txt
```

出力の形:

```text
target: jphacks-kz2601/(default) (emulator=false) mode=dry-run
== smoke apps: 4, app_owners: 2
  app smoke-1730000000 user=79a152956ec9 docs=4 [apps=1 entities=1 records=1 versions=1]
  app_owners/smoke-1730000000
== stg rate-limit counters: 2
== test users: 1
  uid abcDEF123 status="" docs=9 [app_owners=1 apps=1 feedback=1 records=1 shares=1 versions=1]
== targets: 9 (documents counted: 21)
dry run: nothing deleted. To delete exactly this: add -apply -expect=9
```

- `docs` は消える文書の数の目安です(`entities` は記録の親の、中身の無い文書も数える)。試験ユーザーの分は主なコレクションだけを数えます。apply のログに実際の件数が出ます。
- 何も消えていないことは、出力の最後の行で分かります。

### 4. 確認

ドライランの出力を見て、次を確かめます。迷うものがあれば uid の一覧から外してドライランをやり直します。

| 確かめること | 見る所 |
| --- | --- |
| 接続先が意図した DB | 1 行目の `target:` |
| smoke のアプリが smoke の形だけ | `app smoke-…` の行 |
| 試験ユーザーの uid が全部、本人に確かめたもの | `uid …` の行 |
| `(no data: typo?)` が出ていない | uid の打ち間違い |
| `(has Pro entitlement …)` の uid が試験の購入(Sandbox)だけ | 本物の購読なら外す |
| 件数が多すぎない | `== targets:` と `documents counted:` |

### 5. apply

ドライランと**同じフラグ**に `-apply -expect=<ドライランの件数>` を足します。

```sh
go run ./cmd/stg-cleanup -allow-prod-db -smoke -stg-ratelimits -uids-file ~/stg-test-uids.txt -apply -expect=9 | tee ~/stg-cleanup-apply.txt
```

- 件数がずれていれば `ERROR -expect=9 but 10 targets now` で止まり、何も消しません。ドライランからやり直します。
- 途中で失敗した対象は `FAIL` の行に出て、終了コードが 1 になります。どの段も残っているものだけを消すので、同じコマンド(件数は新しいドライランで取り直す)でやり直せます。

### 6. 後の確認

- 同じフラグでドライランをもう一度流し、`== targets: 0` になる(試験ユーザーは `docs=0 (no data: typo?)` と出る)。
- 審査員・デモのアプリが開ける: `make -C backend demo-check ENV=prod`(#171)。

### 消しすぎた時

PITR で、apply を始める前の時刻から消えた文書だけを読み戻します。手順は [firestore-backup-restore.md のケース 1](firestore-backup-restore.md#ケース-1-一部の文書を過去の時点から戻す)。7 日を過ぎたら日次バックアップ(ケース 2)か、手順 1 のエクスポートから戻します。

## DB を分けた時の切り替え・戻し方

接続先は `GOOGLE_CLOUD_PROJECT` と `FIRESTORE_DATABASE` の 2 つで決まり、どちらもデプロイが毎回決めます([runtime-config.md](runtime-config.md#firestore_database復元時))。**appconfig と ai は必ず同じ DB に向けます。** prod の接続先は変えません(#288 の A2: 本番のデータ・Auth・URL はそのまま)。

### A2: stg を新しいプロジェクトへ出す(#288 の推奨)

前提: 新しい stg のプロジェクト・Firebase・SA・WIF ができている(#289・#291)。

1. GitHub → Settings → Environments → **`staging`** → Variables に、stg のプロジェクトの値(`GCP_PROJECT_ID` ほか #291 で決めるもの)を入れる。**repo の Variables は変えない**(prod も変わる)。
2. stg の `ALLOW_SHARED_PROD_DB` を `0` にする(ワークフローの変更・#291)。分けた後は stg が本番の DB を指すと起動が止まる。
3. `workflow_dispatch` で staging をデプロイする。
4. 確かめる:
   - Cloud Logging の appconfig-stg / ai-stg の起動ログ `using Firestore` の `project` / `database` が新しい stg を指す。
   - stg のビルド(#292)でアプリを作り、新しい stg の DB に入り、本番の DB に増えていない。
   - 本番の appconfig-prod / ai-prod のログの接続先が `jphacks-kz2601` / `(default)` のまま。
5. 問題が無ければ、本番の DB に残った stg のデータを[掃除の手順](#掃除の手順)で消し、本番のプロジェクトの `appconfig-stg` / `ai-stg` を消す(#288 の進め方 7)。

### B: つなぎに同じプロジェクトの名前付き DB `stg` を使う

A2 が遅れる時のつなぎです。Auth のユーザー・SA は共有のままです。

1. 人が名前付き DB を作る(`gcloud firestore databases create --database=stg --location=asia-northeast1 --project=jphacks-kz2601`)。本番の DB に TTL ポリシーや手で足したインデックスがあれば、同じものを `stg` にも作る(`gcloud firestore indexes composite list` / `fields ttls list` で本番の設定を見る)。
2. GitHub → Environments → **`staging`** → Variables に `FIRESTORE_DATABASE` = `stg` を入れる。**production には入れない。**
3. `workflow_dispatch` で staging をデプロイし、A2 の 4 と同じく起動ログの `database` が `stg` であることを確かめる(stg の `ALLOW_SHARED_PROD_DB=1` は、名前付き DB を指している間は効かない)。

### 戻し方

| 戻したいもの | やること |
| --- | --- |
| A2 の stg を本番のプロジェクトに戻す | `staging` の Environment の Variables を消して(repo の値に戻る)、stg の `ALLOW_SHARED_PROD_DB` を `1` に戻し、`workflow_dispatch` で staging をデプロイ。本番のプロジェクトの `appconfig-stg` / `ai-stg` を消した後なら、このデプロイで作り直される |
| B の名前付き DB から `(default)` に戻す | `staging` の Environment 変数 `FIRESTORE_DATABASE` を消し、`workflow_dispatch` で staging をデプロイ(未設定なら `(default)`) |
| すぐに戻したい(次のデプロイまでで良い) | `gcloud run services update appconfig-stg …` と `ai-stg …` の `--update-env-vars FIRESTORE_DATABASE=…`([runtime-config.md](runtime-config.md#firestore_database復元時))。次のデプロイで Environment の値に戻る |

- 分けている間に新しい stg に書かれたデータは、戻しても本番の DB へは移しません(試験のデータなので捨てる)。
- prod の Environment(`production`)と prod のサービスは、切り替えにも戻しにも触りません。

### 分けた後の掃除

stg が自分の DB を持った後は、stg の DB に対して `-allow-prod-db` 無しで動きます。

```sh
# A2
GOOGLE_CLOUD_PROJECT=<stg のプロジェクト> go run ./cmd/stg-cleanup -smoke -stg-ratelimits
# B
GOOGLE_CLOUD_PROJECT=jphacks-kz2601 FIRESTORE_DATABASE=stg go run ./cmd/stg-cleanup -smoke -stg-ratelimits
```

## エミュレータで試す

```sh
make -C backend emulator                         # 別の端末
make -C backend run-stg-cleanup                  # 既定: -smoke -stg-ratelimits のドライラン
make -C backend run-stg-cleanup STG_CLEANUP_FLAGS="-uids u1,u2"
```

テストは `make -C backend test-emulator`(`cmd/stg-cleanup` の `TestFirestore*`。テストごとに別のエミュレータのプロジェクトを使う)。
