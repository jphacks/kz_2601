# Firestore のバックアップと復元(#294)

本番の Firestore(`jphacks-kz2601` の `(default)`、`asia-northeast1`)が誤削除やバグで壊れたときに戻すための、守りの一覧・復元の手順・復元の練習(drill)の手順です。

> **本書の復元・エクスポートはまだ一度も実行していません。** GCP への書き込み(restore / export / import / create / delete / update)は、承認を得てから人が行います。

障害対応の中での順番(誰が・いつ復元を始めるか)は [障害対応の手順書 §7](../ops/incident-runbook.md#7-手順-c-firestore-を復元する)(#333・PR #500)にあります。本書はその詳細です。

## 目次
- [今の守り(2026-11-02)](#今の守り2026-11-02)
- [どれで戻すか](#どれで戻すか)
- [費用の見積もり](#費用の見積もり)
- [Terraform との分担(#307)](#terraform-との分担307)
- [復元の手順](#復元の手順)
  - [共通の安全ルール](#共通の安全ルール)
  - [共通の変数](#共通の変数)
  - [ケース 1: 一部の文書を過去の時点から戻す](#ケース-1-一部の文書を過去の時点から戻す)
  - [ケース 2: バックアップを新しい DB に復元して戻す](#ケース-2-バックアップを新しい-db-に復元して戻す)
  - [サービスを復元した DB に向ける](#サービスを復元した-db-に向ける)
  - [確かめ方](#確かめ方)
  - [後片付け](#後片付け)
- [「stg に復元」はなぜ新しい DB なのか](#stg-に復元はなぜ新しい-db-なのか)
- [長く残したいとき(GCS へのエクスポート)](#長く残したいときgcs-へのエクスポート)
- [復元練習の runbook(#294 の完了条件)](#復元練習の-runbook294-の完了条件)
- [退会データの消去とプライバシーポリシー](#退会データの消去とプライバシーポリシー)
- [参考(公式)](#参考公式)

## 今の守り(2026-11-02)

2026-11-02 に人が確かめた状態です。いつでも [`scripts/firestore-backup-status.sh`](../../scripts/firestore-backup-status.sh)(読み取りだけ)で見直せます。

| 守り | 状態 | 戻せる範囲 | 防げること |
| --- | --- | --- | --- |
| 削除保護 | 有効 | — | DB ごとの誤削除(`databases delete` が拒否される) |
| PITR(point-in-time recovery) | 有効 | 直近 **7 日**。1 時間より前は **1 分単位**、1 時間以内は任意の時刻 | 直近の誤更新・誤削除。一部の文書だけ読んで戻せる |
| 日次のスケジュールバックアップ | 有効・保持 **14 日** | 過去 14 日の各日の 1 時点(時刻は選べない) | 7 日より前からの破損に後で気づいたとき |
| GCS へのエクスポート | **作っていない** | 自分で決めた期間 | 14 日より長く残したいとき([下](#長く残したいときgcs-へのエクスポート)) |

- PITR は有効にした時点からしか取れません。`earliestVersionTime` が 7 日前まで伸びるのは有効化から 7 日後です。
- バックアップには **TTL ポリシーとセキュリティルールは入りません**(インデックスの設定は入る)。新しい DB を使い続けるときは作り直します。
- 容量は約 14 MB(0.013 GiB、2026-10-29 の Cloud Monitoring `storage/data_and_index_storage_bytes`)。

## どれで戻すか

| 気づいたのは壊れてから | 使うもの | 手順 |
| --- | --- | --- |
| 7 日以内・一部の文書だけ | PITR の `readTime` で読んで、その文書だけ書き戻す | [ケース 1](#ケース-1-一部の文書を過去の時点から戻す) |
| 7 日以内・広い範囲 | PITR の時点を `export --snapshot-time` → 新しい DB に `import` | [ケース 1](#ケース-1-一部の文書を過去の時点から戻す) の B |
| 7〜14 日前 | 日次バックアップを新しい DB に `restore` | [ケース 2](#ケース-2-バックアップを新しい-db-に復元して戻す) |
| 14 日より前 | 戻せない(エクスポートを作っていれば import) | — |

`gcloud firestore databases clone`(PITR の時点から新しい DB を 1 回で作る)は、手元の gcloud 542.0.0 の GA には無く(`databases --help` のコマンドは `connection-string / create / delete / describe / list / restore / update` だけ)、本書では export/import で代わりにしています。使える版になったら B の 2〜3 を置き換えられます。

## 費用の見積もり

**見積もりです。** 単価は Cloud Billing Catalog API(Cloud Firestore・Tokyo・Standard・USD)から 2026-10-30 に取った値で、2026-11-02 に[料金ページ](https://cloud.google.com/firestore/pricing)を読み直そうとしましたが、こちらの環境からは表を読めず再確認できていません。GCS の単価は Standard・Tokyo を $0.023/GiB・月と仮定しています。

| 項目 | 単価 |
| --- | --- |
| 保存データ | $0.115/GiB・月(`(default)` は 1 GiB まで無料。名前付き DB は無料枠の対象外の見込み) |
| PITR の保存 | $0.115/GiB・月 |
| バックアップの保存 | $0.038/GiB・月 |
| バックアップからの復元 | $0.256/GiB |
| 読み取り(エクスポートで使う) | $0.38/100 万件(1 日 5 万件まで無料) |

| 構成 | 今(0.013 GiB) | 10 GiB に増えたとき |
| --- | --- | --- |
| PITR 7 日(容量はデータ量の 1 倍と仮定。実際は 7 日ぶんの変更で増える) | 約 $0.0015/月 | 約 $1.2/月 |
| 日次バックアップ・保持 14 日(常に 14 個) | 0.013 × 14 × 0.038 ≒ **$0.007/月** | 約 $5.3/月 |
| 復元 1 回(ケース 2) | 約 $0.003 | 約 $2.6 |
| PITR export + import 1 回 | 文書数 × 読み取り単価(1 万件で $0.004)+ GCS 数 MB | 文書数次第 |
| 練習で作る名前付き DB を 1 日置く | $0.0001 未満 | 約 $0.04 |

**今は合計で月 $0.01 未満、練習 1 回も $0.01 未満**です。有効化後の実際の量は Cloud Monitoring の `firestore.googleapis.com/storage/pitr_storage_bytes` と `.../backups_storage_bytes` で確かめます。

## Terraform との分担(#307)

| 対象 | 持ち主 |
| --- | --- |
| PITR・削除保護 | Terraform: `infra/data.tf` の `point_in_time_recovery_enablement` / `delete_protection_state`(`var.firestore_pitr_enabled` / `var.firestore_delete_protection`) |
| DB 自体 | Terraform(`infra/data.tf`) |
| 日次バックアップのスケジュール | 今は gcloud で作ったもの。`google_firestore_backup_schedule` で Terraform に取り込むのは **#307 の担当**(本書では扱わない) |
| 復元・PITR エクスポート・練習で作る DB | 手作業(本書)。**Terraform では扱わない**。練習用の DB を Terraform の state に入れない |

## 復元の手順

### 共通の安全ルール

- **生きている `(default)` を消したり、まるごと上書きしたりして戻さない。** 戻し先は必ず新しい名前付き DB(`restore-<YYYYMMDD>` など)。中身を確かめてから必要なものだけ戻す。
- `(default)` への `import` は「同じ ID の文書を上書きし、余分な文書は消さない」動きです。行うときは `--collection-ids` で範囲を絞り、**2 人で確認してから**。
- 戻したデータには **退会済みの人のデータが含まれうる**。消去より前の時点から戻すと、消したデータがよみがえる。戻した後は消去ジョブを `-dry-run` で流し、対象が出たら消す([account-purge.md](account-purge.md))。
- 時刻は RFC 3339 の UTC(例 `2026-11-01T03:15:00Z`)。`export --snapshot-time` は分単位に丸めた過去の時刻で、`earliestVersionTime` より後。
- 戻す前に、壊した原因(悪いデプロイ・ジョブ)を止める。止めないと戻した後にまた壊れる。

### 共通の変数

```bash
export PROJECT_ID=jphacks-kz2601
export LOCATION=asia-northeast1
export DST=restore-<YYYYMMDD>            # 新しい DB の ID(小文字・数字・ハイフン、4〜63 文字)
export BUCKET=gs://<復元用のバケット>     # ケース 1 の B だけ。asia-northeast1 に作る
```

### ケース 1: 一部の文書を過去の時点から戻す

PITR で、7 日以内の破損に気づいたとき。

1. 壊れる前の時刻 `T` を決める(ログ・利用者の報告から)。
2. `earliestVersionTime` を確かめ、`T` がそれより後であることを見る。

```bash
gcloud firestore databases describe --database='(default)' --project="$PROJECT_ID" \
  --format='value(earliestVersionTime,versionRetentionPeriod)'
```

**A. 数件だけなら: 読んで書き戻す。** REST の `readTime` で過去の版を読みます(Go なら読み取り専用トランザクションに `firestore.ReadTime(T)`)。

```bash
TOKEN=$(gcloud auth print-access-token)
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://firestore.googleapis.com/v1/projects/$PROJECT_ID/databases/(default)/documents/<COLLECTION>/<DOC_ID>?readTime=<T>"
```

読んだ中身を、承認を得てから backend 経由(または小さな Go の使い捨てコード)で書き戻します。トークンは端末に表示しない・残さない。

**B. 広い範囲なら: 時点 T を新しい DB に入れる。**

```bash
# 1. 時点 T をエクスポート(範囲を絞るなら --collection-ids=a,b)
gcloud firestore export "$BUCKET/pitr-<T>" --project="$PROJECT_ID" \
  --database='(default)' --snapshot-time='<T>'

# 2. 空の新しい DB を作る(場所は (default) と同じ)
gcloud firestore databases create --project="$PROJECT_ID" --database="$DST" \
  --location="$LOCATION" --type=firestore-native

# 3. 新しい DB に import
gcloud firestore import "$BUCKET/pitr-<T>" --project="$PROJECT_ID" --database="$DST"

# 4. 終わるまで待つ(export / import は長時間の操作)
gcloud firestore operations list --project="$PROJECT_ID" --database="$DST"
```

その後は[サービスを向ける](#サービスを復元した-db-に向ける)か、`(default)` に一部だけ戻す(`gcloud firestore import "$BUCKET/pitr-<T>" --database='(default)' --collection-ids=<壊れたもの>`、2 人で確認してから)。

同じプロジェクトのバケットなら Firestore のサービスエージェントの権限は通常足りています。権限エラーが出たら[エクスポートとインポート](https://cloud.google.com/firestore/native/docs/manage-data/export-import)の「権限」を見て付けます。

### ケース 2: バックアップを新しい DB に復元して戻す

7 日より前(14 日以内)の破損、または PITR が使えないとき。

```bash
# 1. バックアップを選ぶ(壊れる前で一番新しいもの)
gcloud firestore backups list --location="$LOCATION" --project="$PROJECT_ID" \
  --format='table(name.basename(),database,snapshotTime,state)'

# 2. 新しい DB に復元(戻し先は存在しない ID であること。既存の DB には復元できない)
gcloud firestore databases restore --project="$PROJECT_ID" \
  --source-backup="projects/$PROJECT_ID/locations/$LOCATION/backups/<BACKUP_ID>" \
  --destination-database="$DST"

# 3. 終わるまで待つ
gcloud firestore operations list --project="$PROJECT_ID" --database="$DST"
gcloud firestore databases describe --project="$PROJECT_ID" --database="$DST"
```

その後の戻し方:

| 戻し方 | 手順 | 向く場面 |
| --- | --- | --- |
| 一部だけ戻す(推奨) | `$DST` から `gcloud firestore export "$BUCKET/from-$DST" --database="$DST" --collection-ids=...` → `(default)` に `import --collection-ids=...`(2 人で確認してから) | 特定のコレクション・利用者だけ壊れた |
| サービスを向け替える | [次の節](#サービスを復元した-db-に向ける) | DB 全体が壊れ、`(default)` を使い続けられない |

### サービスを復元した DB に向ける

backend は接続先を環境変数 `FIRESTORE_DATABASE` で選びます(既定 `(default)`、`backend/internal/fsenv`)。コードの変更は要りません。

```bash
for svc in appconfig-prod ai-prod; do
  gcloud run services update "$svc" --project="$PROJECT_ID" --region="$LOCATION" \
    --update-env-vars=FIRESTORE_DATABASE="$DST"
done
# 消去ジョブも同じ DB を見るようにする
gcloud run jobs update account-purge --project="$PROJECT_ID" --region="$LOCATION" \
  --update-env-vars=FIRESTORE_DATABASE="$DST"
```

- 実行 SA `run-backend` の `roles/datastore.user` はプロジェクト単位なので、名前付き DB にもそのまま届きます。
- **次のデプロイで `(default)` に戻ります**(`deploy-cloudrun.yml` が `FIRESTORE_DATABASE: '(default)'` を固定で渡す)。向け替えを続けるなら、同時にワークフローの値も直す PR を出す。
- 新しい DB には削除保護・PITR・日次バックアップが付いていません。使い続けるなら `databases update --delete-protection --enable-pitr` とスケジュールを足し、TTL(`rateLimits` の `expireAt`)も作り直す。Terraform(#307)の対象を新しい DB に移すのも忘れない。
- 戻すときは同じコマンドで `FIRESTORE_DATABASE='(default)'` にする。

### 確かめ方

| 見ること | 方法 |
| --- | --- |
| 復元が終わった | `gcloud firestore operations list --database="$DST"` で `done: true`、エラーなし |
| 目印の文書がある・中身が時点どおり | REST で `$DST` から読む(`.../databases/$DST/documents/<COLLECTION>/<DOC_ID>`)、またはコンソール |
| 文書数が揃う | 主なコレクションを、元の DB の同じ時点(`readTime`)と `$DST` で数える(REST の `runAggregationQuery` の `count`) |
| サービスがつながった | Cloud Run の起動ログに `using Firestore` と `database=<$DST>` が出る |
| 退会データがよみがえっていない | 消去ジョブを `$DST` に向けて `-dry-run` |

### 後片付け

```bash
# 練習・調査に使った DB を消す(削除保護を付けていなければすぐ消せる)
gcloud firestore databases delete --project="$PROJECT_ID" --database="$DST"
# エクスポートを消す
gcloud storage rm -r "$BUCKET/pitr-<T>" "$BUCKET/from-$DST"
```

残すと保存料がかかり、**消去済みの利用者データも残り続けます**(ポリシーの期限を守れない)。向け替えた環境変数を戻したかも確かめます。

## 「stg に復元」はなぜ新しい DB なのか

**今は stg と prod が同じ `(default)` を使っています**(stg だけ `ALLOW_SHARED_PROD_DB=1`)。分離は [#288 の判断](../decisions/288-prod-staging-db-split.md)(#288〜#293)で、まだ終わっていません。このため:

- 「stg に復元」= stg の DB に入れる、は **本番の DB に入れるのと同じ**です。行いません。
- 練習は、**本番のプロジェクトの中の使い捨ての名前付き DB(`restore-drill`)** に復元します。#288 の判断(論点 8「本番のデータを stg にコピーしない」)とも同じです。
- 復元した DB を stg のサービスから読みたいときは、`appconfig-stg` / `ai-stg` の `FIRESTORE_DATABASE` を `restore-drill` に向けます。`APP_ENV=stg` でも本番の DB ではないので起動の止め(fsenv の guard)には掛かりません。stg のビルドは本物の Auth でログインするので、**見えるのは本番と同じ範囲の人だけ**にし、終わったらすぐ戻します。

#288 の分離の後(stg が `tailorit-stg` などの別プロジェクトになった後)も、**本番のデータを stg のプロジェクトへコピーしない**方針は変えません。変わるのは次の点です。

| | 今(共有) | 分離の後 |
| --- | --- | --- |
| 練習の行き先 | prod のプロジェクトの `restore-drill` | 同じ(prod のプロジェクトの `restore-drill`) |
| stg のサービスで確かめる | stg を `restore-drill` に一時的に向けられる | stg は別プロジェクトなので向けられない。prod のプロジェクトで一時的な Cloud Run を立てるか、REST で確かめる |
| stg 自体の復元 | — | stg の DB にも PITR / バックアップを付けるなら、stg のデータで同じ手順を練習できる(本番のデータを使わない練習) |
| 管理されたバックアップの別プロジェクトへの復元 | — | 公式の文書に記載が見当たらない(**要確認**)。必要になったら export/import で行う |

## 長く残したいとき(GCS へのエクスポート)

今は作っていません(14 日より前に戻す必要が見えていない・退会データを長く残さないため)。必要になったら、手で 1 回取ります。

```bash
gcloud firestore export "$BUCKET/manual-<YYYYMMDD>" --project="$PROJECT_ID" --database='(default)'
```

- 定期にするなら Cloud Scheduler + 実行用 SA + バケットのライフサイクル規則が要ります。作る場合は Terraform(#307)で。
- 保持は **退会データの消去期限に収まる長さ**にします([下](#退会データの消去とプライバシーポリシー))。長く残すならポリシーの改定が先です。

## 復元練習の runbook(#294 の完了条件)

**誰が**: 人 2 人。操作役(gcloud を打つ・プロジェクトに書き込める権限を持つ人)と記録役(時刻・つまずきを書く)。エージェントは手順書の更新だけ。

**いつ**: 利用の少ない時間帯。所要 60 分の見込み(初回)。stg の `develop` デプロイを止めておく(向け替えが上書きされるため)。

**測る時間**: 判断を始めてから目印を確かめるまでの全体(RTO の目安)と、各操作の待ち時間。

| # | 手順 | 測る | 結果 |
| --- | --- | --- | --- |
| 0 | 承認を取る(本番のデータを `restore-drill` に入れること・費用 $0.01 未満・終わったら消すこと) | — | |
| 1 | `scripts/firestore-backup-status.sh` で PITR・削除保護・スケジュール・最新バックアップを確かめる | — | |
| 2 | 目印を作る: テストアカウントで既知のアプリを作り、時刻 `T`(UTC)を控える。数分後にそのアプリを変更する | — | T = |
| 3 | [ケース 2](#ケース-2-バックアップを新しい-db-に復元して戻す): 最新の日次バックアップを `restore-drill` に復元 | restore の開始〜`done` | |
| 4 | `restore-drill` で、バックアップ時点に存在した文書を 1 つ読んで確かめる | — | |
| 5 | [ケース 1 の B](#ケース-1-一部の文書を過去の時点から戻す): 時点 `T` を export → `restore-drill-pitr` に import | export / import それぞれ | |
| 6 | `restore-drill-pitr` の目印のアプリが **変更前**の中身であることを確かめる | — | |
| 7 | 主なコレクションの文書数を、`(default)` の `readTime=T` と `restore-drill-pitr` で比べる | — | |
| 8 | (任意)`appconfig-stg` を `restore-drill-pitr` に向け、起動ログの `database=` と目印のアプリを stg のビルドで確かめる → すぐ `(default)` に戻す | 向け替え〜確認 | |
| 9 | 消去ジョブを `restore-drill-pitr` に向けて `-dry-run`(よみがえった退会データが対象に出るか) | — | |
| 10 | [後片付け](#後片付け): 2 つの DB・エクスポートを消す。環境変数を戻したか確かめる。stg のデプロイを再開 | — | |
| 11 | 結果を下の表に書き、つまずいた点を本書に反映して #294 を閉じる | — | |

### 結果の記録(写して埋める)

| 項目 | 値 |
| --- | --- |
| 実施日・実施者(操作 / 記録) | |
| 使ったバックアップ(`snapshotTime`) | |
| 時点 `T` | |
| restore(ケース 2)の所要 | |
| export / import(ケース 1 の B)の所要 | |
| 判断開始〜目印を確かめるまで(全体) | |
| 文書数の一致(コレクションごと) | |
| 退会データの dry-run の結果 | |
| 後片付けの確認(DB・エクスポート・環境変数) | |
| つまずいた点・本書の直し | |

## 退会データの消去とプライバシーポリシー

消去ジョブ(`account-purge`)は論理削除から 29 日を過ぎたものを毎日消し、ポリシーの「30 日以内に消去」に収めています。バックアップと PITR があると、**消した後もしばらく過去の版に残ります**(期限で自動で消える)。

| 仕組み | 消去後に残る期間 | 論理削除からの最長 |
| --- | --- | --- |
| PITR 7 日 | 最大 7 日 | 約 37 日 |
| 日次バックアップ・保持 14 日 | 最大 14 日 | 約 44 日 |

[プライバシーポリシー](../legal/privacy-policy.md)は「バックアップには最大 14 日間残り、その後自動的に消えます」「復旧したときは消去済みのデータを改めて消去します」に改定済みです。保持を 14 日より長くする・エクスポートを長く残すときは、先にポリシーを直します。

## 参考(公式)

- [Firestore の料金](https://cloud.google.com/firestore/pricing)
- [スケジュールされたバックアップ](https://cloud.google.com/firestore/native/docs/backups)(保持は最大 14 週・日次と週次は各 1 つまで・時刻は選べない・TTL とセキュリティルールは含まれない)
- [PITR の使い方](https://cloud.google.com/firestore/native/docs/use-pitr)(`readTime`・`export --snapshot-time`)
- [PITR の概要](https://cloud.google.com/firestore/native/docs/pitr)
- [エクスポートとインポート](https://cloud.google.com/firestore/native/docs/manage-data/export-import)
- [名前付き DB の管理](https://cloud.google.com/firestore/docs/manage-databases)
- 関連: [account-purge.md](account-purge.md)・[gcp-setup.md](gcp-setup.md)・[`infra/`](../../infra/README.md)(#307)・[#288 の判断](../decisions/288-prod-staging-db-split.md)・[障害対応の手順書](../ops/incident-runbook.md)(#333)

gcloud のコマンドとフラグは 542.0.0 の `--help` で確かめました(2026-11-02)。
