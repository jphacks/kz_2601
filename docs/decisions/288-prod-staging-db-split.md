# #288 本番と staging の DB の分け方

> 状態: **提案(オーナーの判断待ち)**。2026-10-30 作成。
> 対象 Issue: [#288](https://github.com/jphacks/kz_2601/issues/288)(関連: [#289](https://github.com/jphacks/kz_2601/issues/289)・[#290](https://github.com/jphacks/kz_2601/issues/290)・[#291](https://github.com/jphacks/kz_2601/issues/291)・[#292](https://github.com/jphacks/kz_2601/issues/292)・[#293](https://github.com/jphacks/kz_2601/issues/293)・[#294](https://github.com/jphacks/kz_2601/issues/294))
> 調べた日: 2026-10-30。GCP は読み取りの `gcloud` だけで確かめた(何も変更していない)。料金は変わるので、決める前に公式の料金ページを見ること。

## 目次

1. [結論(推奨)](#結論推奨)
2. [選択肢の表](#選択肢の表)
3. [根拠](#根拠)
4. [進め方(順番と、人 / エージェントの分担)](#進め方順番と人--エージェントの分担)
5. [決めてほしいこと](#決めてほしいこと)
6. [参考](#参考)

## 結論(推奨)

**プロジェクトを分ける(案 A)。ただし「今の `jphacks-kz2601` を本番としてそのまま残し、stg だけを新しいプロジェクトへ出す」形(案 A2)にする。**

- これから App Store に出す審査用の版(Review 構成)は、**`jphacks-kz2601` の Firebase(`GoogleService-Info.plist`)と、プロジェクト番号入りの prod の URL をバイナリに焼き込む**。本番を新しいプロジェクトへ移すと、出した版のログインと接続先が切れる(アップデートの強制が要る)。だから**本番は動かさず、stg のほうを出す**のが一番安全。
- この形なら**本番のデータ・Firebase Auth のユーザー(uid)・URL は一切変わらない**。#293 は「本番を移す」作業ではなく、「本番に混ざった stg のゴミを掃除する」作業に縮む。
- **分離はリリースの後でよい**(下の根拠 1)。ただし**リリース前に、本番の DB に Point-in-time recovery(PITR)と削除保護を入れ、エクスポートを 1 回取る**ことを条件にする(#294 の一部を前倒し)。今はどちらも無い。
- リリースから分離までは、develop の backend の変更が本番のデータに直接効く。**分離が 2 週間を超えて遅れそうなら、つなぎとして stg を同じプロジェクトの名前付き DB(`stg`)へ逃がす**(案 B をつなぎに使う)。
- 案 B(名前付き DB だけ)を最終形にはしない。Auth のユーザー・Secret・IAM・SA が共有のままで、stg のビルドで本物のユーザーとしてログインでき、stg の SA が本番の DB に書けるため。

## 選択肢の表

| 選択肢 | 意味 | 本番への影響 | Auth のユーザー | 費用 | 作業量 |
|---|---|---|---|---|---|
| A1. プロジェクトを 2 つ新しく作り、本番も移す | `tailorit-stg` / `tailorit-prod` を作り、今の本番のデータとユーザーを新しい本番へ移す | **大**。出した版の plist と URL が旧プロジェクトを指すので、強制アップデートと Auth のユーザーの移し替え(`firebase auth:export/import`)が要る | 移し替えは可能だが、匿名ユーザーの端末のトークンは旧プロジェクトのまま(**要確認**) | 新プロジェクト 2 つ分。Firestore の無料枠はプロジェクトごとの `(default)` に付く(**要確認**) | 特大 |
| **A2. 今のプロジェクトを本番に残し、stg だけ新しいプロジェクトへ(推奨)** | `jphacks-kz2601` = 本番。新しく `tailorit-stg`(仮)を作り、Firebase・Firestore・Secret・SA・WIF を丸ごと分ける | **なし**(本番は触らない。最後に本番から stg のサービスを消すだけ) | 本番のユーザーはそのまま。stg は空から始まる(試験用に匿名で入り直す) | 新プロジェクト 1 つ分。ほぼ無料枠の範囲の見込み(**要確認**) | 中(大半はエージェント。プロジェクト作成・課金・Apple の設定は人) |
| B. 同じプロジェクトで名前付きの DB | `jphacks-kz2601` に Firestore の `stg` を足し、stg のサービスだけ `FIRESTORE_DATABASE=stg` で切り替える | 小(本番の DB は触らない) | **共有のまま**。stg のビルドで本物のユーザーとしてログインでき、同じ uid で stg に書く | 名前付き DB は無料枠の対象外の見込み(**要確認**)。使う量は小さい | 小(コードの変更 + DB を 1 つ作る) |
| C. しばらく共有のまま | 何も分けない | develop の push・stg のスモーク・開発者の試験が本番のデータに入り続ける。develop のバグが本物のユーザーのデータを壊しうる | 共有 | 0 | 0 |

## 根拠

### 1. 「リリースの前に分ける必要があるか」→ A2 なら後でよい

- **出す版が依存しているのは本番側だけ**。Review 構成の接続先は `https://ai-prod-378953239426.asia-northeast1.run.app` / `https://appconfig-prod-378953239426.asia-northeast1.run.app`(`ios/project.yml` の Review の configs)で、Firebase は `jphacks-kz2601` の plist(`docs/infra/gcp-setup.md` 冒頭の表・`ios/scripts/sync-firebase-plist.sh`)。A2 は本番を動かさないので、**リリースの前でも後でも、出した版に影響しない**。
- 逆に、**A1 を選ぶならリリースの前に済ませないと高くつく**(出した後に本番を移すと、旧版の利用者のログインと接続先が切れる)。A1 にしない理由の一つ。
- 後にする代わりに負うリスク: リリースから分離までの間、develop の backend(stg)が本番と同じ `(default)` を読み書きする。開発者の stg のビルドは本物のユーザーと同じ Auth を使い、stg の backend は本物のユーザーのアプリを読める。**このリスクは PITR(7 日)と削除保護で「戻せる」状態にしてから負う**。
- 審査の期間(〜11/15)は本番を動かさない方針(`docs/roadmap/roadmap-m1-m6.md` M1)。PITR と削除保護の有効化はデータもサービスも止めない設定の変更なので、この方針と両立する見込み(**要確認**: 有効化で本番に影響が無いことを公式の記載で確認してから実施)。

### 2. 今の状態(2026-10-30 に読み取りで確認)

| 項目 | 実測 |
|---|---|
| Firestore | `jphacks-kz2601` に `(default)` の 1 つだけ。`asia-northeast1`。**PITR: 無効、削除保護: 無効**。スケジュールバックアップ: 0 件 |
| Cloud Run | `appconfig-stg` / `ai-stg` / `appconfig-prod` / `ai-prod` が同じプロジェクト。4 つとも実行 SA は `run-backend@jphacks-kz2601`、`GOOGLE_CLOUD_PROJECT=jphacks-kz2601`、違うのは `APP_ENV`(stg / prod)だけ。`FIRESTORE_DATABASE` のような DB を選ぶ変数は無い |
| Secret | `ANTHROPIC_API_KEY` の 1 つを stg と prod で共有 |
| SA | `run-backend` / `github-deployer` / `purge-job` / `purge-scheduler` ほか。deployer は 1 つで、develop からも main からも同じプロジェクトにデプロイできる(#291) |
| その他 | Cloud Run Job `account-purge` と Scheduler `account-purge-daily`(毎日 4:30)がある。`docs/infra/account-purge.md` には「まだ実行していません」とあり、記述が古い可能性(**要確認**) |
| GitHub | Environments `staging` / `production` はあるが、Variables は repo 単位(`GCP_PROJECT_ID` ほか 5 つ)で両環境が共有 |
| コード | `backend/internal/store/firestore.go` と `backend/cmd/ai/main.go`(回数制限)は `firestore.NewClient(ctx, project)` だけで DB を選べない。回数制限は `APP_ENV` を文書 ID の頭に付けて stg / prod を分けている(`ratelimit.go`「staging and production share one Firestore」)。iOS は Firestore を直接使っていない(FirebaseAuth のみ) |

### 3. Firebase Auth と iOS の設定

- Auth のユーザーはプロジェクトに付く。**同じプロジェクトのままでは stg と prod のユーザーは分けられない**(案 B の弱点)。
- A2 では、本番のユーザー(リリース前に作られた審査員・デモ・開発者の分も含む)は全部本番に残る。stg は新しい Firebase で空から始まる。stg のユーザーを本番へ移す必要は無い。
- iOS は #292 で Staging 構成を作り、**Staging の Bundle ID を `jp.kz2601.metamon.stg`(ウィジェットは `jp.kz2601.metamon.stg.widget`、App Group は `group.jp.kz2601.metamon.stg`)にする**案。同じ端末に stg 版と本番版を並べられる。Apple Developer に App ID を足して Sign in with Apple を付けるのは人の作業。
- 本番の Bundle ID を `jp.kz2601.metamon` のまま出すかは #310 の判断。**変えるならリリースの前**(出した後は実質変えられない)。変える場合は本番の Firebase に iOS アプリを足し直し、plist を作り直す。
- Sign in with Apple の Firebase 側の設定が本番で済んでいるかは今回確かめていない(**要確認**。`gcp-setup.md` の 2026-10-24 時点では未設定)。

### 4. Cloud Run の SA と WIF

- A2 では stg のプロジェクトに `run-backend` / `github-deployer` / WIF のプール・プロバイダ / Artifact Registry / Secret を新しく作る。**stg の SA は本番のプロジェクトに権限を持たない**ので、stg から本番の DB に書けなくなる(案 B では書けてしまう)。
- GitHub の Variables を Environment ごとに分ける(staging → 新プロジェクト、production → `jphacks-kz2601`)。本番の WIF は `refs/heads/main` に縛る(#291)。本番の IAM の変更なので、審査の期間の後に人の承認で行う。
- #306(Organization への移行)で WIF の条件 `assertion.repository == 'jphacks/kz_2601'` は書き換えが要る。stg を新しく作るときは、移行後のリポジトリ名でも通る条件にしておくと二度手間が減る。
- 将来、本番の URL を独自ドメイン(例: `api.<ドメイン>`)の裏に置けば、プロジェクトを動かしても出した版が壊れない。今回の範囲外(**要確認**: ドメインの有無)。

### 5. データの移し替え(#293)と、分離の前に作られたユーザー

- A2 では本番のデータを移さない。#293 は次の 3 つに縮む:
  1. 作業の前に本番を全体エクスポート(#294 と同じ手順)。
  2. 本番に混ざった stg 由来のデータを消す: スモークの `smoke-*` アプリ、回数制限の `stg_` で始まる文書(`rateLimits`)、開発者が stg のビルドで作った試験のアプリ。スクリプトはドライランを先に出し、消す一覧を人が確認してから実行。
  3. stg の Cloud Run(`appconfig-stg` / `ai-stg`)を本番のプロジェクトから消す。
- 注意: **今の文書には「stg で作ったか」の印が無い**(`store` に環境の記録が無い)。`smoke-*` と回数制限以外は、開発者の uid ごとに人が見て判断する。開発者が同じ uid で本番版も使っているなら、そのデータは消さない。
- 分離の後、開発者の端末の stg 版は新しい Firebase で別の uid になる。分離の前に stg で作ったアプリは stg からは見えなくなる(本番版からは引き続き見える)。

### 6. バックアップ(#294)

- **リリースの前**: 本番の `(default)` に PITR(7 日)と削除保護を入れ、Cloud Storage へのエクスポートを 1 回取る。
- **リリースの後**: 日次のスケジュールバックアップと保持期間を決める。
- 復元の練習は、**本番のデータを stg のプロジェクトへコピーしない**ほうがよい(本物の利用者の個人データが stg に入る)。本番のプロジェクトの中で一時的な名前付き DB(例: `restore-drill`)へ復元し、確かめたら消す形を推奨。プロジェクトをまたいだ復元の可否は**要確認**。
- このために `store` が DB の ID を受け取れるようにしておく(#290 の `FIRESTORE_DATABASE`)。A2 でも無駄にならない。

### 7. 費用

- 新しい stg のプロジェクト: Cloud Run は最小 0 台、Firestore は `(default)` の無料枠の範囲に収まる見込みで、**ほぼ 0 円**(無料枠の単位は**要確認**)。予算アラートは新プロジェクトにも付ける(#289)。
- PITR: 有効にすると保持分の保存に料金がかかる。今の DB は小さいので月数十円程度の見込み(**要確認**)。
- スケジュールバックアップ・エクスポート: 保存した容量の分だけ。同じく小さい見込み(**要確認**)。
- Anthropic のキーを環境ごとに分けると、stg の使いすぎが本番の上限を食わなくなる(#289)。

## 進め方(順番と、人 / エージェントの分担)

本番を動かす手順は 0 と 7 だけで、どちらもデータとサービスを止めない。

| 順 | いつ | やること | 誰 | Issue |
|---|---|---|---|---|
| 0 | **リリースの前** | 本番の `(default)` に PITR と削除保護を入れ、エクスポートを 1 回取る(バケットの作成を含む)。手順はエージェントが書き、実行は人(か、人の承認を得てエージェント) | 人(承認・実行) | #294 |
| 1 | リリースの前 | Review 構成が本番の plist と prod の URL を使うことを確かめる(今の `ios/project.yml` は prod) | エージェント | #452 G7 |
| 2 | リリース | 共有の DB のまま出す | 人 | — |
| 3 | リリースの後すぐ | stg のプロジェクトを作り、課金をつなぎ、Firebase を足し、Auth(匿名・Apple)を有効化。Apple Developer に `jp.kz2601.metamon.stg` の App ID。stg 用の Anthropic のキーと予算アラート | **人** | #289 |
| 4 | 3 と並行 | backend: `FIRESTORE_DATABASE` を受け取る。起動時に `APP_ENV=stg` かつ本番のプロジェクト ID なら止める。`/health` に環境と接続先を出す | エージェント | #290 |
| 5 | 3 の後 | stg のプロジェクトに SA・WIF・Artifact Registry・Secret を作る手順を書き、人の承認で実行。GitHub の Variables を Environment ごとに分け、`deploy-cloudrun.yml` を直す | エージェント(手順・PR)+ 人(承認・GCP の実行) | #291 |
| 6 | 3 の後 | iOS: Staging 構成(Bundle ID・plist・URL・アイコン) | エージェント | #292 |
| 7 | 5・6 の後 | develop の push が新しい stg に出ることを確かめてから、本番のプロジェクトの `appconfig-stg` / `ai-stg` を消す。stg のゴミを掃除(ドライラン → 人の確認 → 実行) | エージェント + 人(確認) | #293 |
| 8 | 7 の後 | 日次のバックアップ、復元の練習、本番の WIF を main に縛る | エージェント + 人(承認) | #294・#291 |

- 3 が 2 週間以上遅れそうなら、つなぎとして本番のプロジェクトに名前付き DB `stg` を作り、4 の `FIRESTORE_DATABASE` で stg のサービスだけ向け先を変える(Auth は共有のまま)。
- 審査の期間(〜11/15)の扱い: 0 は設定だけなので審査中でも可(要確認の上で)。7 の「本番のプロジェクトから stg のサービスを消す」と 8 の本番の IAM の変更は、審査の期間の後にする。

## 決めてほしいこと

| # | 決めること | 推奨 |
|---|---|---|
| 1 | 方式(A1 / A2 / B / C) | **A2**(今のプロジェクトを本番に残し、stg を新しいプロジェクトへ) |
| 2 | 分離をリリースの前にするか後にするか | **後でよい**。条件はリリースの前に「PITR・削除保護・エクスポート 1 回」 |
| 3 | 0(PITR・削除保護・エクスポート)を誰がいつ実行するか | リリースの前に、人が実行(またはエージェントに実行を許可) |
| 4 | stg のプロジェクト ID と課金アカウント | `tailorit-stg`(空いているかは**要確認**)。課金は本番と同じアカウント(個人か法人かは #309) |
| 5 | stg の Bundle ID | `jp.kz2601.metamon.stg`。本番の Bundle ID を変えるか(#310)はリリースの前に決める |
| 6 | 分離が遅れたときのつなぎ(名前付き DB `stg`)を使うか | 3 が 2 週間以上遅れるなら使う |
| 7 | 本番のスモーク(Claude 2 回)を残すか | 残す。ただし専用のスモーク用ユーザーに限り、`smoke-*` を必ず片付ける(今の `-cleanup` を維持)(#291) |
| 8 | 復元の練習の行き先 | 本番のプロジェクト内の一時的な名前付き DB。本番のデータを stg にコピーしない |

## 参考

- [Firestore: Manage databases(名前付き DB)](https://cloud.google.com/firestore/docs/manage-databases)
- [Firestore: Point-in-time recovery](https://cloud.google.com/firestore/docs/pitr)
- [Firestore: Backups / Restore](https://cloud.google.com/firestore/docs/backups)
- [Firestore: Export and import](https://cloud.google.com/firestore/docs/manage-data/export-import)
- [Firestore の料金](https://cloud.google.com/firestore/pricing)
- [Firebase: 複数の環境のためのプロジェクト構成](https://firebase.google.com/docs/projects/dev-workflows/general-best-practices)
- [Firebase CLI: auth:export / auth:import](https://firebase.google.com/docs/cli/auth)
- リポジトリ内: `docs/infra/gcp-setup.md`、`docs/infra/account-purge.md`、`docs/roadmap/roadmap-m1-m6.md`(M1)、`docs/release/m0-early-release-readiness.md`(G7)、`.github/workflows/deploy-cloudrun.yml`、`backend/internal/store/firestore.go`、`backend/internal/ratelimit/ratelimit.go`、`ios/project.yml`
