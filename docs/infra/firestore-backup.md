# Firestore のバックアップと復元(#294)

本番の Firestore(`jphacks-kz2601` の `(default)`、`asia-northeast1`)を、誤削除やバグで壊れたときに戻せるようにするための設計と手順です。

> **本書の設定・復元はまだ実行していません。** GCP の変更は承認を得てから人が行います。2026-10-30 に行ったのは読み取り(describe / list / Cloud Monitoring・Cloud Billing Catalog の参照)だけです。

## 目次
- [今の状態(2026-10-30 読み取り)](#今の状態2026-10-30-読み取り)
- [有効にするもの](#有効にするもの)
- [方式の比較と推奨](#方式の比較と推奨)
- [費用の見積もり](#費用の見積もり)
- [設定の手順(人の作業)](#設定の手順人の作業)
- [Terraform との分担(PR #476)](#terraform-との分担pr-476)
- [復元の手順](#復元の手順)
  - [共通の安全ルール](#共通の安全ルール)
  - [ケース 1: 一部の文書を過去の時点から戻す](#ケース-1-一部の文書を過去の時点から戻す)
  - [ケース 2: バックアップを新しい DB に復元して戻す](#ケース-2-バックアップを新しい-db-に復元して戻す)
  - [ケース 3: stg プロジェクトに復元する(練習)](#ケース-3-stg-プロジェクトに復元する練習)
- [復元練習の runbook(#294 の完了条件)](#復元練習の-runbook294-の完了条件)
- [退会データの消去とプライバシーポリシー](#退会データの消去とプライバシーポリシー)
- [参考(公式)](#参考公式)

## 今の状態(2026-10-30 読み取り)

`gcloud firestore databases describe --database='(default)' --project=jphacks-kz2601` ほかで確かめた値です。

| 項目 | 値 | 意味 |
| --- | --- | --- |
| `pointInTimeRecoveryEnablement` | `POINT_IN_TIME_RECOVERY_DISABLED` | PITR は無効 |
| `versionRetentionPeriod` | `3600s` | 過去の版は 1 時間ぶんだけ(`earliestVersionTime` もほぼ 1 時間前) |
| `deleteProtectionState` | `DELETE_PROTECTION_DISABLED` | DB ごと消せてしまう |
| バックアップのスケジュール | 0 件(`gcloud firestore backups schedules list`) | なし |
| バックアップ | 0 件(`gcloud firestore backups list --location=asia-northeast1`) | なし |
| エクスポート用の Cloud Scheduler | なし(あるのは `account-purge-daily` だけ) | なし |
| `databaseEdition` / `freeTier` | `STANDARD` / `true` | 無料枠つきの Standard |
| データ+インデックスの容量 | **約 14.2 MB(14,236,039 bytes ≒ 0.013 GiB)** | Cloud Monitoring `firestore.googleapis.com/storage/data_and_index_storage_bytes`(2026-10-29 19:59 UTC) |
| TTL ポリシー / 複合インデックス | 0 件 / 0 件 | 復元先で作り直すものは今は無い |

つまり **今、本番には 1 時間より前に戻す手段がありません。**

## 有効にするもの

| 対策 | 防げること | 設定 |
| --- | --- | --- |
| 削除保護 | DB ごとの誤削除 | `delete_protection_state = ENABLED` |
| PITR(7 日) | 直近 7 日の任意の時点(1 分単位)に戻す・読む | `point_in_time_recovery_enablement = ENABLED` |
| 管理されたスケジュールバックアップ(日次・保持 14 日) | 7 日より前からの破損に後で気づいたとき | `gcloud firestore backups schedules create` |

週次のバックアップ(保持 8 週)は、下の[プライバシーポリシーとの関係](#退会データの消去とプライバシーポリシー)から **今は入れません**。

## 方式の比較と推奨

| 観点 | A. 管理されたスケジュールバックアップ | B. Cloud Scheduler + `gcloud firestore export` を Cloud Storage へ |
| --- | --- | --- |
| 作る物 | スケジュール 1 つ(DB の設定) | Scheduler ジョブ・実行用 SA・バケット・ライフサイクル規則 |
| 費用 | 保存 $0.038/GiB・月、復元 $0.256/GiB | 毎回「文書数 × 読み取り単価($0.38/100 万件)」+ GCS の保管料 |
| 保持 | `--retention` で最大 14 週、期限で自動削除 | バケットのライフサイクルで消す(設定漏れで残り続ける危険) |
| 復元 | 新しい DB を作る(`databases restore`)。インデックス設定も入る | どの DB にも `import` できる(上書き。余分な文書は消えない)。**別プロジェクトにも入れられる** |
| 一部のコレクションだけ | 不可(DB まるごと) | `--collection-ids` で可 |
| 実行時刻 | 選べない | Scheduler で選べる |
| 失敗の検知 | 不要(Google が実行) | ジョブの失敗監視が要る |

**推奨: A(日次・保持 14 日)+ PITR 7 日 + 削除保護。B の定期実行は作りません。**

- 今のデータは 0.013 GiB と小さく、運用する物が少ない A で十分です。
- B の強み(別プロジェクトへの取り込み・一部だけ戻す)は、**必要なときに PITR の `export --snapshot-time` を手で 1 回叩けば** 得られます(ケース 1・3)。定期実行の仕組みは要りません。
- 管理されたバックアップを別プロジェクトに復元できるかは、公式の文書に記載がありません(**要確認**)。stg での練習は export/import で行います。

## 費用の見積もり

単価は Cloud Billing Catalog API(Cloud Firestore・Tokyo・Standard・USD、2026-10-30 取得)の値です。正式には[料金ページ](https://cloud.google.com/firestore/pricing)で確かめてください。

| 項目 | 単価(Tokyo・Standard) |
| --- | --- |
| 保存データ | $0.115/GiB・月(1 GiB まで無料) |
| PITR の保存 | $0.115/GiB・月 |
| バックアップの保存 | $0.038/GiB・月 |
| バックアップからの復元 | $0.256/GiB |
| clone(PITR から新 DB) | $0.2568/GiB |
| 読み取り(エクスポートで使う) | $0.38/100 万件(1 日 5 万件まで無料) |

| 構成 | 今の容量(0.013 GiB) | 10 GiB に増えたとき |
| --- | --- | --- |
| PITR 7 日(容量 ≒ データ + 7 日ぶんの変更。ここではデータ量の 1 倍と仮定) | 約 $0.0015/月 | 約 $1.15/月 |
| 日次バックアップ・保持 14 日(常に 14 個) | 0.013 × 14 × 0.038 ≒ **$0.007/月** | 約 $5.3/月 |
| (参考)週次・保持 8 週を足す(+8 個) | +$0.004/月 | +$3.0/月 |
| 復元練習 1 回(export/import) | 文書数 × 読み取り単価 + GCS 数 MB。**文書数は要確認**(1 万件でも $0.004) | — |

**今は合計で月 $0.01 未満**です。有効化後の実際の容量は Cloud Monitoring の `firestore.googleapis.com/storage/pitr_storage_bytes` と `.../backups_storage_bytes` で確かめます。

## 設定の手順(人の作業)

Terraform(PR #476)が入る前に手で行う場合の手順です。入った後は PITR と削除保護は Terraform で行います([下](#terraform-との分担pr-476))。

```bash
export PROJECT_ID=jphacks-kz2601

# 1. 削除保護と PITR(7 日)
gcloud firestore databases update --database='(default)' --project="$PROJECT_ID" \
  --delete-protection --enable-pitr

# 2. 日次バックアップ・保持 14 日(実行時刻は選べない)
gcloud firestore backups schedules create --database='(default)' --project="$PROJECT_ID" \
  --recurrence=daily --retention=14d

# 3. 確かめる
gcloud firestore databases describe --database='(default)' --project="$PROJECT_ID" \
  --format='value(pointInTimeRecoveryEnablement,deleteProtectionState,versionRetentionPeriod)'
gcloud firestore backups schedules list --database='(default)' --project="$PROJECT_ID"
gcloud firestore backups list --location=asia-northeast1 --project="$PROJECT_ID"   # 翌日以降に 1 件
```

- フラグは gcloud 542.0.0 の `--help` で確かめました(`schedules create` は `--database`・`--retention`・`--recurrence=daily|weekly`・週次は `--day-of-week`)。
- PITR を有効にしても、過去にさかのぼっては取れません。`earliestVersionTime` は有効化の時点から少しずつ 7 日前まで伸びます。
- **プライバシーポリシーの改定と同時に行います**(今のポリシーは「別保存(バックアップ)は取っていません」「最大 1 時間」と書いているため)。[下](#退会データの消去とプライバシーポリシー)の文案を先に反映します。

## Terraform との分担(PR #476)

| 対象 | 持ち主 |
| --- | --- |
| PITR | Terraform: `var.firestore_pitr_enabled`(`infra/data.tf` の `point_in_time_recovery_enablement`)。`prod.tfvars.example` は `true`、`stg.tfvars.example` は `false` |
| 削除保護 | Terraform: `var.firestore_delete_protection`(`delete_protection_state`)。prod / stg とも `true` |
| DB 自体 | Terraform(`prevent_destroy` + `deletion_policy = "ABANDON"`) |
| バックアップのスケジュール | **今は gcloud(本書)**。後で `google_firestore_backup_schedule`(`retention = "1209600s"`、`daily_recurrence {}`)を `infra/data.tf` に足し、`import` で取り込むことを提案 |
| 復元・PITR エクスポート | 手作業(本書の runbook)。Terraform では扱わない |

PR #476 の README では、import 直後に差分 0 を確かめるため一時的に `firestore_pitr_enabled = false`・`firestore_delete_protection = false` にしてから、有効化を別の apply で行います。**本書の手順で先に手で有効化した場合は、tfvars を `true` のまま import すれば差分 0 になります。**

## 復元の手順

### 共通の安全ルール

- **生きている `(default)` を削除したり、まるごと上書きしたりして戻さない。** 復元は必ず新しい DB(`restore-YYYYMMDD` など)か stg に入れ、中身を確かめてから必要なものだけ戻す。
- 本番の `(default)` への `import` は「同じ ID の文書を上書きし、余分な文書は消さない」動きです。行うのはケース 1・2 の最後の段だけで、必ず `--collection-ids` で範囲を絞り、人の承認を取る。
- 戻したデータには **退会済みの人のデータが含まれうる**。消去済み(墓標になった)より前の時点から戻すと、消したデータがよみがえる。戻した後は消去ジョブを `-dry-run` で流し、対象が出たら消す([account-purge.md](account-purge.md))。
- TTL ポリシーとセキュリティルールはバックアップに入らない。新しい DB を使う場合は作り直す(今は TTL 0 件。`rateLimits` の TTL を有効にした後は再設定が要る)。
- 時刻は RFC 3339 の UTC。PITR は 1 時間より前は **分単位**(`2026-11-01T03:15:00Z`)。export の `--snapshot-time` は常に分単位。

### ケース 1: 一部の文書を過去の時点から戻す

PITR が有効で、7 日以内の破損に気づいたとき。

1. 壊れる前の時刻 `T` を決める(ログ・利用者の報告から)。`T` は `earliestVersionTime` より後であること。
2. 中身を見るだけなら、REST の `readTime` で読む(Go なら読み取り専用トランザクションに `firestore.ReadTime(T)`)。モバイル SDK からは読めない。

```bash
TOKEN=$(gcloud auth print-access-token)
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://firestore.googleapis.com/v1/projects/jphacks-kz2601/databases/(default)/documents/<COLLECTION>/<DOC_ID>?readTime=<T>"
```

3. 戻す場合は、`T` の時点を必要なコレクションだけエクスポートする(PITR が要る)。

```bash
export PROJECT_ID=jphacks-kz2601
export BUCKET=gs://<復元用バケット(asia-northeast1)>
gcloud firestore export "$BUCKET/pitr-<T>" --project="$PROJECT_ID" --database='(default)' \
  --snapshot-time=<T> --collection-ids=<COLLECTION_ID>
```

4. まず新しい DB(または stg)に `import` して中身を確かめる。
5. 承認を得て、本番 `(default)` に同じ範囲だけ `import` する。

```bash
gcloud firestore import "$BUCKET/pitr-<T>" --project="$PROJECT_ID" --database='(default)' \
  --collection-ids=<COLLECTION_ID>
```

所要時間: 今の容量なら数分と見込むが **要確認**(練習で測る)。7 日より前ならケース 2。

### ケース 2: バックアップを新しい DB に復元して戻す

```bash
export PROJECT_ID=jphacks-kz2601
export LOCATION=asia-northeast1

# 1. 使うバックアップを選ぶ(snapshotTime を見る)
gcloud firestore backups list --location="$LOCATION" --project="$PROJECT_ID" \
  --format='table(name.basename(),snapshotTime,state)'

# 2. 新しい DB に復元(既存の DB ID は使えない。(default) は指定しない)
gcloud firestore databases restore --project="$PROJECT_ID" \
  --source-backup="projects/$PROJECT_ID/locations/$LOCATION/backups/<BACKUP_ID>" \
  --destination-database=restore-<YYYYMMDD>

# 3. 終わるまで待つ(終わるまで DB は使えない)
gcloud firestore operations list --database=restore-<YYYYMMDD> --project="$PROJECT_ID"
```

4. 中身を確かめる(コンソールで `restore-<YYYYMMDD>` を開く、または REST で読む)。
5. 戻し方を選ぶ。

| 戻し方 | 手順 | 向く場面 |
| --- | --- | --- |
| 一部だけ戻す(推奨) | 新 DB から `gcloud firestore export "$BUCKET/from-restore" --database=restore-<YYYYMMDD> --collection-ids=...` → 承認後に `(default)` へ `import --collection-ids=...` | 特定のコレクション・利用者だけ壊れた |
| 切り替える | backend は `(default)` 固定(`firestore.NewClient`)なので、DB ID を変えるにはコード変更(`NewClientWithDatabase`)と再デプロイが要る。消去ジョブも同じ | DB 全体が壊れ、`(default)` を使い続けられない |

6. 終わったら新 DB を消す(`gcloud firestore databases delete --database=restore-<YYYYMMDD> --project="$PROJECT_ID"`)。残すと保存料がかかり、消去済みのデータも残る。

所要時間: 公式に目安の記載なし。**要確認**(練習で測る)。費用: 復元 $0.256/GiB(今なら $0.004 未満)。

### ケース 3: stg プロジェクトに復元する(練習)

stg は PR #466(#288)の判断で別プロジェクトに移る予定です。移った後に行います。管理されたバックアップを別プロジェクトに復元できるかは記載がないため(**要確認**)、**PITR エクスポート → stg に import** で行います。

```bash
export PROD=jphacks-kz2601
export STG=<stg のプロジェクト ID>
export BUCKET=gs://<prod 側の復元用バケット(asia-northeast1)>
export T=<戻したい時刻・分単位の UTC>

# 1. stg の Firestore サービスエージェントに、バケットの読み取りを付ける
STG_NUM=$(gcloud projects describe "$STG" --format='value(projectNumber)')
gcloud storage buckets add-iam-policy-binding "$BUCKET" \
  --member="serviceAccount:service-${STG_NUM}@gcp-sa-firestore.iam.gserviceaccount.com" \
  --role=roles/storage.objectViewer

# 2. prod の時刻 T をまるごとエクスポート
gcloud firestore export "$BUCKET/drill-$T" --project="$PROD" --database='(default)' --snapshot-time="$T"

# 3. stg の DB に import(stg の DB は空か、捨ててよい状態にしておく)
gcloud firestore import "$BUCKET/drill-$T" --project="$STG" --database='(default)'

# 4. 終わるまで待つ
gcloud firestore operations list --project="$STG"
```

- `import` は上書きで、stg にだけある文書は残ります。完全に揃えたいときは、stg に空の新しい DB を作ってそこへ入れます。
- **本番の利用者データが stg に入ります。** stg を見られる人を本番と同じ範囲に限り、練習が終わったら stg の DB(または該当コレクション)とバケットの `drill-*` を消します(残すとポリシーの消去期限を守れない)。
- PITR がまだ無いときは `--snapshot-time` を外す(実行した時点になる)。

## 復元練習の runbook(#294 の完了条件)

stg プロジェクトができた後に 1 回行い、結果をこの表に書き足します。

| # | 手順 | 結果・所要時間 |
| --- | --- | --- |
| 0 | 承認を取る(本番データを stg に入れること・費用・ポリシー改定済みであること) | |
| 1 | prod で PITR・削除保護・日次バックアップが有効なことを確かめる([設定の手順](#設定の手順人の作業)の 3) | |
| 2 | 練習用の目印を prod に作る: テストアカウントで既知のアプリを作り、時刻 `T` を控える。その後そのアプリを変更する | |
| 3 | ケース 3 の 1〜4 で、時刻 `T` を stg に入れる。export と import の時間を測る | export: 要確認 / import: 要確認 |
| 4 | stg の backend から目印のアプリを読み、`T` の時点の中身(変更前)であることを確かめる | |
| 5 | 主なコレクションの文書数を、prod の `T` 時点(`readTime` で数える)と比べる | |
| 6 | ケース 2 も試す: 最新の日次バックアップを prod に `restore-drill` として復元し、時間を測る → 中身を確かめて消す | restore: 要確認 |
| 7 | stg で消去ジョブを `-dry-run` で流し、退会済みのデータが対象に出ることを確かめる | |
| 8 | 後片付け: stg のデータ、`drill-*` のエクスポート、`restore-drill` を消す。追加した IAM を外す | |
| 9 | 所要時間と詰まった点を本書に反映し、#294 を閉じる | |

## 退会データの消去とプライバシーポリシー

消去ジョブ(`account-purge`)は論理削除から 29 日(`-older-than=696h`)を過ぎたものを毎日 4:30 に消し、ポリシーの「30 日以内に消去」に収めています。バックアップと PITR を入れると、**消した後もしばらく過去の版に残ります**。期限が来れば自動で消えます。

| 仕組み | 消去後に残る期間 | 論理削除からの最長 |
| --- | --- | --- |
| 今(version retention 1 時間) | 最大 1 時間 | 約 30 日 |
| PITR 7 日 | 最大 7 日 | 約 37 日 |
| 日次バックアップ・保持 14 日 | 最大 14 日 | 約 44 日 |
| 週次バックアップ・保持 8 週(入れない) | 最大 8 週 | 約 86 日 |

推奨:

1. バックアップの保持は **30 日以下**(推奨 14 日)にし、週次・8 週は入れない。
2. ポリシーに「バックアップには最大 14 日残り、期限で自動的に消える」ことを書く。今のポリシーは「別保存(バックアップ)は取っていません」「最大 1 時間」と書いているので、**有効化と同時に改定が要ります**(本 PR ではポリシーは変えません)。
3. 「30 日以内」をバックアップまで含めて厳密に守る場合の代案: 消去ジョブを `-older-than=360h`(15 日)にして、15 日 + 1 日 + 14 日 ≦ 30 日に収める(利用者が退会を取り消せる期間が短くなる)。

### ポリシー改定の文案(提案)

日本語。保存期間の「Firestore の別保存(バックアップ)は取っていません。…最大 1 時間…」の行を置き換え:

> - 障害やデータの破損に備えて、Firestore のバックアップを取っています。直前 7 日間の任意の時点に戻すための記録と、1 日 1 回の複製(14 日間保持)があり、どちらも期限が来ると自動的に消えます。

削除の行(「…30 日以内に消去します。…」)の後に追加:

> - 消去したデータも、バックアップには最大 14 日間残り、その後自動的に消えます。バックアップは障害からの復旧にだけ使い、復旧したときは消去済みのデータを改めて消去します。

英語。Storage の「No separate Firestore backups are taken.」を置き換え、Deletion に 1 文を追加:

> - **Storage:** … We keep Firestore backups to recover from failures: point-in-time recovery data for the last 7 days and daily backups kept for 14 days. Both expire automatically.
> - **Deletion:** … Erased data may remain in backups for up to 14 more days until they expire. Backups are used only for disaster recovery; if we restore one, we erase previously deleted data again.

## 参考(公式)

- [Firestore の料金](https://cloud.google.com/firestore/pricing)
- [スケジュールされたバックアップ](https://cloud.google.com/firestore/native/docs/backups)(保持は最大 14 週・日次と週次は各 1 つまで・時刻は選べない・TTL とセキュリティルールは含まれない)
- [PITR の使い方](https://cloud.google.com/firestore/native/docs/use-pitr)(`readTime`・`export --snapshot-time`・`databases clone`)
- [PITR の概要](https://cloud.google.com/firestore/native/docs/pitr)
- [エクスポートとインポート](https://cloud.google.com/firestore/native/docs/manage-data/export-import)(別プロジェクトへの import とサービスエージェントの権限)
- 関連: [account-purge.md](account-purge.md)・[gcp-setup.md](gcp-setup.md)・`infra/`(PR #476)・PR #466(#288)

補足: `gcloud firestore databases clone`(PITR の時点から同じプロジェクト・同じ場所に新 DB を作る、$0.2568/GiB)は手元の gcloud 542.0.0 には無い(入った版は要確認)。使えれば、ケース 1 の「新しい DB に戻して確かめる」を 1 コマンドで行えます。
