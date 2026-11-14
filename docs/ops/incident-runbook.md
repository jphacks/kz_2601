# 障害対応の手順書(#333)

一般に配信している Tailor It(iOS + Cloud Run `ai-*` / `appconfig-*` + Firestore)の障害を、**2 人で**止血・復旧・記録するための手順です。

- 監視・アラート・構造化ログ・Error Reporting は [`docs/infra/observability.md`](../infra/observability.md)(#295)。
- 本書のコマンドは **GCP の変更(Cloud Run の env・トラフィック)を含みます。実行するのは人です**(エージェントはしない・[`CLAUDE.md`](../../CLAUDE.md))。
- 共通の値: プロジェクト `jphacks-kz2601`、リージョン `asia-northeast1`、サービス `ai-prod` / `ai-stg` / `appconfig-prod` / `appconfig-stg`。

## 目次

1. [重さの区分(SEV)](#1-重さの区分sev)
2. [誰がいつ対応するか](#2-誰がいつ対応するか)
3. [最初の 15 分](#3-最初の-15-分)
   - [OS ごとに絞って見る](#os-ごとに絞って見る)
4. [道具: アプリ内のお知らせ](#4-道具-アプリ内のお知らせ)
5. [手順 A: Claude API の障害・上限](#5-手順-a-claude-api-の障害上限)
6. [手順 B: 悪いデプロイを戻す](#6-手順-b-悪いデプロイを戻す)
7. [手順 C: Firestore を復元する](#7-手順-c-firestore-を復元する)
8. [手順 D: Anthropic の API キーが漏れた](#8-手順-d-anthropic-の-api-キーが漏れた)
9. [手順 E: App Store のビルドの問題](#9-手順-e-app-store-のビルドの問題)
10. [振り返り(障害の記録)](#10-振り返り障害の記録)
11. [stg での障害訓練(#333 の完了条件)](#11-stg-での障害訓練333-の完了条件)
12. [知っておくべき落とし穴](#12-知っておくべき落とし穴)
13. [手順 F: 悪用(自動の動き)](#13-手順-f-悪用自動の動き)

## 1. 重さの区分(SEV)

| SEV | 目安 | 例 | 最初の反応 | 利用者への案内 |
| --- | --- | --- | --- | --- |
| **SEV1** | 多くの人が**アプリを使えない**、または**データ・秘密が危ない** | 起動直後に落ちる版が配信中 / appconfig が 5xx でアプリ一覧・記録が開けない / データの破損・消失 / API キーの漏えい / 料金の暴走 | 気づいたら**すぐ**(夜間も「止血」までは行う・§2) | 15 分以内にアプリ内のお知らせ(warning) |
| **SEV2** | 主な機能の一つが**止まっている**が、作ったアプリと記録は使える | Claude の障害・上限で生成できない / 一部の画面だけ失敗 / デプロイ後に特定の操作が壊れた | 営業時間内に 1 時間以内。夜間は翌朝 | 必要ならお知らせ(info か warning) |
| **SEV3** | 一部の人・一部の場面だけ。回避策がある | 特定の端末の表示崩れ / まれなエラー / 遅い | 次の作業日に Issue 化 | 不要(リリースノートで) |

迷ったら**重い方**にして、落ち着いてから下げる。データ・秘密が関わるものは常に SEV1。

## 2. 誰がいつ対応するか

2 人のチームで、**1 週間ごとの当番**を決める(月曜に交代。当番は Issue のカレンダーかチームの連絡手段で共有する)。

| 役 | 誰 | すること |
| --- | --- | --- |
| **指揮(当番)** | その週の当番 | SEV を決める・止血を行う・お知らせを出す・記録を取る・終わりを宣言する |
| **補助** | もう 1 人 | 原因の調査・修正の PR・指揮の手順の読み上げと確認(2 人で見る)。SEV1 で連絡がついたら加わる |

| 時間帯 | SEV1 | SEV2 | SEV3 |
| --- | --- | --- | --- |
| 平日の日中・夜(8:00〜24:00 JST) | 当番がすぐ。つかまらなければ気づいた方がそのまま指揮 | 当番が 1 時間以内 | 次の作業日 |
| 夜間(0:00〜8:00) | **気づいた方が止血だけ**(お知らせ・トラフィックの戻し・生成の停止)。原因の調査と修正は朝。起こし合う義務は無い | 朝 | 次の作業日 |
| **試験期間・帰省などの縮退期間** | 止血だけを当日中に。修正は期間明け | 期間明け(お知らせで「対応は○日以降」と出す) | 期間明け |

**縮退期間の前にやること**(期間の 1 週間前に当番が確認):

- [ ] 期間中は `main` にマージしない(=prod にデプロイしない)。直前の大きな変更は出さない。
- [ ] [観測の通知](../infra/observability.md)(#295)が 2 人に届くことを確かめる。
- [ ] 必要なら AI の 1 日の全体上限 `RATE_LIMIT_GLOBAL_DAILY` を控えめにする(費用の暴走対策・[§5](#5-手順-a-claude-api-の障害上限))。
- [ ] 2 人とも参加できない日を共有する。両方いない日は「障害は翌日以降の対応」と割り切る。

**止血のための権限**: 指揮が §4〜§6 のコマンドを打てるよう、2 人とも `jphacks-kz2601` で Cloud Run の env とトラフィックを変えられる権限(`roles/run.admin` 相当)を持っておく(付与は人が判断)。

## 3. 最初の 15 分

当番(または気づいた人)が上から順に。**直す前に止血**。

- [ ] **1. 記録を始める**(0 分): 時刻と気づいたきっかけ(アラート / レビュー / 問い合わせ)をメモする。あとで [振り返り](postmortem-template.md) の時系列になる。
- [ ] **2. 何が壊れているかを 1 行で**: 「生成ができない」「アプリが開けない」など、利用者から見た症状で書く。
- [ ] **3. SEV を決める**(§1)。補助に連絡する(SEV1 は必ず、SEV2 は日中なら)。
- [ ] **4. 直前の変更を確かめる**: 最近のデプロイ(`gh run list -R jphacks/kz_2601 --workflow=deploy-cloudrun.yml -L 5`)、iOS の新しい版の配信、env の変更、Anthropic の状態(<https://status.anthropic.com/>)。
- [ ] **5. 見る**: [observability.md](../infra/observability.md)(#295)のダッシュボード・Error Reporting。ログの例:

  ```sh
  gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="ai-prod" AND severity>=WARNING' \
    --project=jphacks-kz2601 --freshness=30m --limit=50 --format='value(timestamp,severity,textPayload,jsonPayload.message)'
  ```

- [ ] **6. 止血を選ぶ**:

  | 症状 | 止血 | 手順 |
  | --- | --- | --- |
  | デプロイ直後から壊れた | 前のリビジョンに戻す | [§6](#6-手順-b-悪いデプロイを戻す) |
  | Claude が落ちている・上限 | 生成を止めて案内 | [§5](#5-手順-a-claude-api-の障害上限) |
  | データが消えた・壊れた | 書き込みの原因を止める(デプロイを戻す)→ 復元は落ち着いて | [§7](#7-手順-c-firestore-を復元する) |
  | キーが漏れた | **先にキーを無効化** | [§8](#8-手順-d-anthropic-の-api-キーが漏れた) |
  | iOS の版が壊れている | 段階的リリースの一時停止・お知らせ | [§9](#9-手順-e-app-store-のビルドの問題) |

- [ ] **7. 利用者に知らせる**(SEV1 は 15 分以内): [§4 のお知らせ](#4-道具-アプリ内のお知らせ)。文面は「何ができないか」「作ったアプリと記録はどうか」「いつ頃わかるか」の 3 点。
- [ ] **8. 15 分ごとに状況をメモ**し、補助と共有する。

**終わりの宣言**: 症状が消え、ログ・ダッシュボードが平常に戻ってから 30 分見る → お知らせを外す(または「復旧しました」に差し替えて翌日外す)→ SEV1・SEV2 は 2 日以内に振り返りを書く。

### OS ごとに絞って見る

特定の OS(ios・android・macos・windows・linux)だけで起きていそうな時の絞り込みです。ログの鍵は `backend/internal/httpx/client.go` の `ClientAttrs` が付ける `clientPlatform`(`clientVersion`・`clientBuild`・`clientDslVersion` も同じ所)。付く行・値の丸め方は [observability.md](../infra/observability.md#クライアントの属性)。

- `clientPlatform` が付くのは `http.request`(アクセスログ)と `appcheck` の行。**`ai.usage` には付かない**ので、AI の失敗を OS で見る時は先に `http.request` で絞り、同じ `requestId` を `ai.usage` で引く。
- ヘッダが無い・知らない値のリクエストは `other`。

例(`ai-prod` で windows の 5xx。サービス名・OS を替えて使う):

```sh
gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="ai-prod" AND jsonPayload.event="http.request" AND jsonPayload.clientPlatform="windows" AND jsonPayload.status>=500' \
  --project=jphacks-kz2601 --freshness=1h --limit=50 --format='value(timestamp,jsonPayload.path,jsonPayload.status,jsonPayload.clientVersion,jsonPayload.clientBuild,jsonPayload.requestId)'
```

```
jsonPayload.event="http.request" AND jsonPayload.clientPlatform="linux" AND jsonPayload.status>=400
jsonPayload.event="http.request" AND jsonPayload.clientPlatform="ios" AND jsonPayload.clientBuild="12"
jsonPayload.event="http.request" AND jsonPayload.clientPlatform="other"
jsonPayload.requestId="<上で見つけた requestId>"
```

どの OS・どの版に偏っているかは、`clientPlatform`・`clientBuild` ごとに件数を数えて比べる(上のコマンドの `--format` を `json` にして集計するか、Logs Explorer の「ログ分析」で `GROUP BY`)。

**デスクトップ版と Mac 版を分ける**: Mac アプリとデスクトップ版を macOS で動かした時は、どちらも `clientPlatform="macos"` になる([client-platforms.md](../backend/client-platforms.md))。分けるのは `User-Agent`。デスクトップ版は `TailorIt-Desktop/<版> (<OS> <OS の版>; <arch>)` を付ける(`desktop/app/.../net/ClientPlatform.kt` の `desktopUserAgent`)。アプリの `jsonPayload` には User-Agent を出さないので、Cloud Run のリクエストログの `httpRequest.userAgent` で見る:

```
resource.type="cloud_run_revision" AND resource.labels.service_name="ai-prod" AND httpRequest.userAgent:"TailorIt-Desktop/"
```

見つけた行の `trace`(Cloud Run のリクエストログとアプリのログは同じ trace でまとまる)から、同じリクエストの `http.request` を引いて `clientPlatform` を確かめる。

## 4. 道具: アプリ内のお知らせ

appconfig の `GET /v1/client-config` が、環境変数 `NOTICE_JSON` の 1 行の告知を `notice` として返し、iOS が画面上部の**閉じられるバナー**で出します(サインイン前にも出る・審査用の版でも出る)。形は [`openapi-appconfig.yaml`](../backend/openapi-appconfig.yaml) の `Notice`。

| フィールド | 必須 | 意味 |
| --- | --- | --- |
| `id` | ○ | `^[A-Za-z0-9._-]{1,64}$`。**iOS は閉じた id を覚え、同じ id は二度と出さない**。文面を変えて出し直す時は id も変える(例: `2026-11-02-ai` → `2026-11-02-ai-2`) |
| `severity` | | `info`(既定・青)/ `warning`(橙) |
| `message.ja` | ○ | 1 行(200 字まで。改行は空白になる) |
| `message.en` | | 英語の表示の時に出す。無ければ `ja` |
| `linkUrl` | | 「詳しく見る」で開く https のページ |
| `minIOSBuild` / `maxIOSBuild` | | この範囲の CFBundleVersion だけに出す(例: 壊れた build 15 だけに `"minIOSBuild":15,"maxIOSBuild":15`) |

**出す**(env の変更 = 新しいリビジョン。約 1 分。JSON にカンマが入るので区切り文字を `^@^` に変える):

```sh
SERVICE=appconfig-prod   # 訓練は appconfig-stg
NOTICE='{"id":"2026-11-02-ai","severity":"warning","message":{"ja":"AI の生成を一時的に止めています。作ったアプリと記録はそのまま使えます。","en":"AI generation is paused for now. Your apps and records still work."}}'
echo "$NOTICE" | jq -e '.id and .message.ja' >/dev/null && \
gcloud run services update "$SERVICE" --region=asia-northeast1 --project=jphacks-kz2601 \
  --update-env-vars="^@^NOTICE_JSON=$NOTICE"
```

**確かめる**(届かない時は不正な JSON。ログに `appconfig: ignoring invalid notice` が出ている):

```sh
URL=$(gcloud run services describe "$SERVICE" --region=asia-northeast1 --project=jphacks-kz2601 --format='value(status.url)')
curl -s "$URL/v1/client-config" | jq .notice
```

**外す**: `gcloud run services update "$SERVICE" --region=asia-northeast1 --project=jphacks-kz2601 --remove-env-vars=NOTICE_JSON`

注意:

- 応答は `Cache-Control: max-age=300`。端末に届くまで**最大 5 分**+ 次に前面に戻った時。
- デプロイ(`--update-env-vars`・#513)の後も**残る**。終わったら消し忘れない。変え方は [runtime-config.md](../infra/runtime-config.md#変え方)。
- appconfig を前のリビジョンに戻している間(§6)は、`services update` で作ったリビジョンにトラフィックが行かない。[§12](#12-知っておくべき落とし穴) の手順で出す。
- 文面の例:
  - Claude の障害: 「AI の生成を一時的に止めています。作ったアプリと記録はそのまま使えます。」/ "AI generation is paused for now. Your apps and records still work."
  - 復旧: 「AI の生成が使えるようになりました。ご不便をおかけしました。」(info・新しい id)
  - 壊れた版: 「この版で落ちる問題があります。修正版を準備中です。」(warning・`min/maxIOSBuild` で絞る)

## 5. 手順 A: Claude API の障害・上限

SEV2(全員が生成できない)。料金の暴走は SEV1。

1. **確かめる**: <https://status.anthropic.com/>、`ai-prod` のログ(§3 の 5)。Claude からの応答で見分ける。

   | 症状(ログ・Claude の応答) | 原因 | 対応 |
   | --- | --- | --- |
   | 529 `overloaded_error` / 5xx が多い | Anthropic 側の障害 | 2〜4 へ。復旧を待つ |
   | 429 `rate_limit_error` | 組織のレート上限 | Anthropic Console の Limits を確かめる。上限の引き上げは人(Console) |
   | 400 で credit / billing の文言 | 前払い残高・月の上限に達した | Console で残高・支出上限を確かめる(費用の見方は [ai-cost.md](../infra/ai-cost.md)) |
   | 401 `authentication_error` | キーが無効(ローテーション漏れ・漏えいで無効化) | §8 |

2. **生成を止める(丁寧に断る)**: ai に `AI_PAUSED=1` を入れる([ai-pause.md](../infra/ai-pause.md))。生成・対話・テキストだけを Claude を呼ばずに 503 `ai_paused` で断り、iOS は「AI を一時的に止めています。しばらくしてからもう一度お試しください」を出す。作ったアプリ・記録・一覧(appconfig)はそのまま使える。

   ```sh
   gcloud run services update ai-prod --region=asia-northeast1 --project=jphacks-kz2601 \
     --update-env-vars=AI_PAUSED=1
   ```

   **予備(AI_PAUSED が効かない版の時)**: **全体の 1 日上限**(コストの遮断器)を 1 にする。Claude を呼ぶ前に 429 `rate_limited`(scope `global`)で断り、iOS は「本日の受付を終了しました / 利用が集中したため、新しい生成を止めています。○時に再開します」のカードを出す。

   ```sh
   gcloud run services update ai-prod --region=asia-northeast1 --project=jphacks-kz2601 \
     --update-env-vars=RATE_LIMIT_GLOBAL_DAILY=1
   ```

   - (予備のみ)数え方は日ごと(JST 0 時でリセット)。今日すでに 1 単位以上使っていれば、以後の生成・対話・aiButton は**全部**断られる。
   - (予備のみ)断る文言が「利用が集中」なので、**必ずお知らせ(§4)で本当の理由を出す**。
   - 止めずに待つ判断もある(短い障害なら、iOS のエラー表示と再試行で足りる)。目安: 30 分を超えそうなら止める。

3. **お知らせを出す**(§4・warning)。
4. **戻す**: 復旧を 15 分確かめてから入れた env を外し(`AI_PAUSED` / 予備なら `RATE_LIMIT_GLOBAL_DAILY`。既定 2000 に戻る)、お知らせを「復旧しました」(新しい id・info)に差し替える。

   ```sh
   gcloud run services update ai-prod --region=asia-northeast1 --project=jphacks-kz2601 \
     --remove-env-vars=AI_PAUSED   # 予備を使った時は RATE_LIMIT_GLOBAL_DAILY
   ```

   ※ `AI_PAUSED`・`RATE_LIMIT_GLOBAL_DAILY` はデプロイの後も残る(#513。戻し忘れに注意)。

## 6. 手順 B: 悪いデプロイを戻す

`appconfig-*` と `ai-*` は同じイメージから別々にデプロイされる。壊れた方だけ戻してよい(両方戻すのが無難)。**数十秒で効き、コードは変わらない。**

1. **前のリビジョンを探す**(新しい順。2 行目が直前):

   ```sh
   SERVICE=ai-prod   # appconfig-prod / ai-stg / appconfig-stg
   gcloud run revisions list --service="$SERVICE" --region=asia-northeast1 --project=jphacks-kz2601 \
     --sort-by='~metadata.creationTimestamp' --limit=5 \
     --format='table(metadata.name,metadata.creationTimestamp,status.conditions[0].status:label=READY)'
   # 今どこにトラフィックが行っているか
   gcloud run services describe "$SERVICE" --region=asia-northeast1 --project=jphacks-kz2601 --format='yaml(status.traffic)'
   # そのリビジョンのコミット(deploy-cloudrun が GIT_SHA を入れている)
   gcloud run revisions describe <REVISION> --region=asia-northeast1 --project=jphacks-kz2601 --format=json \
     | jq -r '.spec.containers[0].env[] | select(.name=="GIT_SHA").value'
   ```

   - env だけを変えたリビジョン(§4・§5 の操作)も一覧に出る。**GIT_SHA が今と違う、最後の良いリビジョン**を選ぶ。

2. **戻す**:

   ```sh
   gcloud run services update-traffic "$SERVICE" --region=asia-northeast1 --project=jphacks-kz2601 \
     --to-revisions=<PREV>=100
   ```

3. **確かめる**: `curl -fsS "$URL/health"`、§3 のログ、iOS で壊れていた操作。
4. **コードを直す**: `main` を戻す(revert の PR)か、`hotfix/*` で直す([release/process.md §7](../release/process.md#7-チェックリスト-hotfix))。main に force push はしない。
5. **トラフィックを最新に戻す**: 直したデプロイが終わったら、`describe` で `status.traffic` を見て、最新に 100% 行っていなければ

   ```sh
   gcloud run services update-traffic "$SERVICE" --region=asia-northeast1 --project=jphacks-kz2601 --to-latest
   ```

   **戻したままにしない**(固定したままだと、以後のデプロイが反映されない・反映されたか分からなくなる)。

## 7. 手順 C: Firestore を復元する

SEV1。詳しい手順・費用・安全ルールは [firestore-backup-restore.md](../infra/firestore-backup-restore.md)(#294・PITR 7 日 + 日次バックアップ 14 日)。ここは障害対応での順番だけ。

1. **壊している原因を止める**: デプロイが原因なら §6 で戻す。ジョブが原因なら止める(例: 消去ジョブ `account-purge-daily` を `gcloud scheduler jobs pause`)。
2. **お知らせ**(§4・warning)。「一部のデータが表示されない可能性があります。復旧作業中です。」
3. **壊れる前の時刻 `T`(UTC・分単位)を決める**: ログ・デプロイの時刻・利用者の報告から。
4. **戻し方を選ぶ**:
   - 7 日以内 → [ケース 1(PITR から一部を戻す)](../infra/firestore-backup-restore.md#ケース-1-一部の文書を過去の時点から戻す)
   - 7 日より前 → [ケース 2(バックアップを新しい DB に復元)](../infra/firestore-backup-restore.md#ケース-2-バックアップを新しい-db-に復元して戻す)
5. **生きている `(default)` を消したり、まるごと上書きしたりしない**。新しい DB で中身を確かめ、`--collection-ids` で絞って戻す。本番への `import` は 2 人で確認してから。
6. **退会済みの人のデータがよみがえっていないか**: 消去ジョブを `-dry-run` で流す([account-purge.md](../infra/account-purge.md))。
7. 止めたジョブを戻す(`gcloud scheduler jobs resume`)。

注意: **今は stg と prod が同じ `(default)` を使っている**([release/process.md §1](../release/process.md#1-前提)・分離は [#288](../decisions/288-prod-staging-db-split.md))。stg 向けの操作でも本番のデータに効く。

## 8. 手順 D: Anthropic の API キーが漏れた

SEV1。**原因の調査より先にキーを無効化する。**

1. Anthropic Console でそのキーを**無効化(Disable)**する。この時点から本番の生成は 401 で止まる → §4 でお知らせ。
2. 新しいキーを作り、Secret Manager `ANTHROPIC_API_KEY` に新しい版を足し、`ai-*` に新しいリビジョンを作って読ませる。手順は [credentials-inventory.md の「2. Anthropic API キー」](../infra/credentials-inventory.md#2-anthropic-api-キー)(#301)。
3. 生成が戻ったことを iOS で確かめ、お知らせを外す。
4. Console の Usage で、漏れていた間の不審な利用・費用を確かめる。
5. 漏れた経路を塞ぐ: リポジトリなら gitleaks の手順([CONTRIBUTING.md の「秘密情報」](../../CONTRIBUTING.md#秘密情報)・credentials-inventory.md §8)。**キーの値は Issue・PR・チャット・振り返りに書かない。**

## 9. 手順 E: App Store のビルドの問題

起動直後に落ちる・データを壊すなどの版が配信された。SEV1(多くの人)/ SEV2(一部)。

1. **広がりを止める**: 段階的リリース中なら App Store Connect で**一時停止**(Pause Phased Release。最大 30 日)。「販売を停止」はアプリごと消えるので使わない。
2. **その版の人にだけ知らせる**: §4 のお知らせを `minIOSBuild` / `maxIOSBuild` でその build に絞る(例: 「この版で落ちる問題があります。修正版を準備中です。」)。
3. **backend 側で回避できないか**: 特定の部品・API が原因なら、backend を戻す(§6)・その版にだけ返さない、で止まることがある(古い版との互換は [release/process.md §2.2](../release/process.md#22-backend))。
4. **修正版を出す**: `hotfix/*` → [release/process.md §7](../release/process.md#7-チェックリスト-hotfix)(iOS は `bump-build.sh --version X.Y.(Z+1)`)。審査提出時に**迅速な審査(expedited review)**を申請する: <https://developer.apple.com/contact/app-store/?topic=expedite>(理由は「重大なバグの修正」・再現手順と影響を英語で短く。頻繁に使うと通りにくい)。
5. **修正版が配信されてから**、必要なら強制アップデート(#302): `appconfig-prod` に `MIN_IOS_BUILD=<修正版の build>`(閉じられない更新画面)か `LATEST_IOS_BUILD`(閉じられる案内)。

   ```sh
   gcloud run services update appconfig-prod --region=asia-northeast1 --project=jphacks-kz2601 \
     --update-env-vars=MIN_IOS_BUILD=<BUILD>
   ```

   - **修正版がストアで落とせる状態になる前に上げない**(全員が更新画面から出られなくなる)。国によって配信に時間差がある。
   - `MIN_IOS_BUILD` はデプロイの後も残る(#513・§12)。
6. 段階的リリースを再開(または修正版で新しく始める)。お知らせを外す。

## 10. 振り返り(障害の記録)

- SEV1・SEV2 は**収まってから 2 日以内**に [postmortem-template.md](postmortem-template.md) を写して書く。SEV3 は Issue のコメントで足りる。
- 置き場所: `docs/ops/postmortems/YYYY-MM-DD-<短い名前>.md`(PR で入れる。**個人情報・キー・利用者の内容は書かない**)。
- **人を責めない**。「なぜ仕組みがそれを許したか」を書き、再発防止は Issue にして担当と期日を付ける。
- 訓練(§11)も同じテンプレートで記録する(種類を「訓練」にする)。

## 11. stg での障害訓練(#333 の完了条件)

**人が行う**(GCP の変更を含む)。所要 60〜90 分、2 人で。指揮役が本書を読みながら操作し、補助役が時刻とつまずきを記録する。**本番(`*-prod`)には触れない。** **Firestore への書き込み・import は訓練に含めない**(stg と prod が同じ DB のため。DB の復元練習は [firestore-backup-restore.md の runbook](../infra/firestore-backup-restore.md#復元練習の-runbook294-の完了条件)(#294)で別に行う)。

### 準備

- [ ] develop の最新が `ai-stg` / `appconfig-stg` にデプロイ済み(このお知らせの機能を含む版)。
- [ ] Staging 構成の iOS アプリ(`project.yml` の既定 = stg)を入れた端末かシミュレータ。
- [ ] 記録用に [postmortem-template.md](postmortem-template.md) を写す(種類 = 訓練)。
- [ ] 今のリビジョンを控える: §6 の 1 を `ai-stg` と `appconfig-stg` で。

### シナリオ: 「develop のデプロイ後、AI が応答しなくなった」

| # | すること | 本書 | 合格の基準 |
| --- | --- | --- | --- |
| 1 | 補助役が「生成できない」と知らせる。指揮役が §3 のチェックリストを始め、SEV を決める | §3 | 5 分以内に SEV2 と判断し、記録を始めた |
| 2 | `appconfig-stg` にお知らせ(warning)を出す | §4 | `curl …/v1/client-config \| jq .notice` に出る。アプリを前面に戻すとバナーが出る(最大 5 分)。ダークモードと英語表示でも読める。VoiceOver で読み上げ・閉じられる |
| 3 | `ai-stg` の生成を止める | §5 の 2 | iOS で生成すると「本日の受付を終了しました」のカードが出る。作ったアプリ・記録は開ける |
| 4 | 「直前のデプロイが原因」と判断し、`ai-stg` と `appconfig-stg` を前のリビジョンに戻す | §6 の 1〜3 | `status.traffic` が前のリビジョン 100%。`/health` が 200 |
| 5 | お知らせを出し直す(appconfig を戻した状態で) | §12 の 1 | 戻した状態でもお知らせが出る |
| 6 | 復旧: トラフィックを最新に戻し、生成の停止を外す | §6 の 5・§5 の 4 | `status.traffic` が最新 100%。iOS で生成できる |
| 7 | お知らせを「復旧しました」(新しい id・info)に差し替え → 外す | §4 | 閉じた後も新しい id は出る。外すとバナーが消える |
| 8 | 後片付けの確認 | — | `gcloud run services describe ai-stg/appconfig-stg --format=yaml(spec.template.spec.containers[0].env,status.traffic)` に `NOTICE_JSON`・`RATE_LIMIT_GLOBAL_DAILY` が無く、トラフィックは最新 100% |

### 終わったら

- [ ] 記録(各段の時刻・つまずき・本書の直す所)を `docs/ops/postmortems/YYYY-MM-DD-drill.md` に入れる PR を出す。本書の間違いは同じ PR で直す。
- [ ] #333 に「stg で訓練し、手順書どおりに戻せた」と記録へのリンクをコメントする(完了条件)。
- [ ] 次の訓練の予定(目安: 半年ごと・大きなインフラ変更の後)。

## 12. 知っておくべき落とし穴

1. **トラフィックを前のリビジョンに固定している間の env 変更**: `gcloud run services update --update-env-vars` は「最新のリビジョン」(=壊れたイメージ)を元に新しいリビジョンを作り、しかもトラフィックは固定のままなので**効かない**。戻している間にお知らせを出す時は、良いリビジョンのイメージを指定して作り、そこに流す:

   ```sh
   IMAGE=$(gcloud run revisions describe <PREV> --region=asia-northeast1 --project=jphacks-kz2601 --format='value(spec.containers[0].image)')
   gcloud run services update appconfig-prod --region=asia-northeast1 --project=jphacks-kz2601 \
     --image="$IMAGE" --update-env-vars="^@^NOTICE_JSON=$NOTICE"
   gcloud run services update-traffic appconfig-prod --region=asia-northeast1 --project=jphacks-kz2601 --to-latest
   ```

2. **デプロイと手で入れた env**: `deploy-cloudrun.yml` は(#513 から) `--update-env-vars` でデプロイが決める値だけを上書きし、手で足した `NOTICE_JSON`・`RATE_LIMIT_*`・`MIN_IOS_BUILD`・`LATEST_IOS_BUILD` は**残る**(消し忘れに注意)。どの値を運用で変えてよいか・変え方は [runtime-config.md](../infra/runtime-config.md)。障害中・強制アップデート中はデプロイ後に必ず確かめる。
3. **stg と prod は同じ Firestore**(#288 で分けるまで): stg の訓練・調査でもデータを書き換えない。
4. **キャッシュ**: `client-config` は 5 分キャッシュされる。お知らせ・強制アップデートは即時ではない。

## 13. 手順 F: 悪用(自動の動き)

自動の動き(サーバーのルール・#341)を使ったスパム・フィッシング・いやがらせの通知、または越境(他のアプリのデータが動いた・宛先でない人に届いた)の疑い。越境の疑いは SEV1、スパム・フィッシングは広がり方で SEV1〜2([決定 339](../decisions/339-abuse-threat-model.md#止めるスイッチ346))。

runner はまだデプロイしていない。デプロイの後に使う手順。止めるスイッチ(#346)は 6 つあり、どれもデプロイ無しに次のきっかけから効く(止めている間のきっかけは実行せずに終わり、解いても後からまとめて実行しない):

| スイッチ | 止める範囲 | 操作 |
|---|---|---|
| `RUNNER_PAUSED=1` | 環境の自動の動きを全部(`skipped_paused`) | runner のサービスの env |
| `RUNNER_NOTIFY_PAUSED=1` | 環境の**通知だけ**(書き込みは動く) | runner のサービスの env |
| 全体の停止 | `RUNNER_PAUSED` と同じ範囲(`skipped_stopped`・`stop_level=global`)。新しいリビジョンを待たずに効く | `moderate stop-all`(`-force` で下の書き込みの拒否も) |
| アプリの停止 | 1 つのアプリ(`skipped_stopped`・`reason=operator_stop`・`stop_level=app`)。定期のルールも止まる。作成者は解けない。`-for 24h` で期限つきの一時停止、`-force` でルールのきっかけになる書き込みも 403 `app_stopped` で断る(強制停止) | `moderate stop-app` |
| ルールの停止 | 1 つのルール(`stop_level=rule`) | `moderate stop-rule` |
| アプリの通知の一時停止 | 1 つのアプリの通知だけ(期限つき) | `moderate pause-notify`(と、下の自動) |

**自動の一時停止**: 1 つのアプリ(アプリそのもの・その中の投稿)を 24 時間で **3 人**(別々の人。同じ人の何回もの通報は 1 人)が通報すると、runner がそのアプリの通知を 72 時間止める(ログ `runner.stop`・`by=auto`)。止めるのは通知だけ。非公開化・削除・アカウントの無効化は人が決める。人数と長さは `RUNNER_AUTO_NOTIFY_PAUSE_REPORTERS` / `_FOR`([runtime-config.md](../infra/runtime-config.md))。

### 止める順(最初の 15 分)

1. **通知だけを止める**。1 つのアプリなら `pause-notify`、どのアプリか分からない・広がっているなら `RUNNER_NOTIFY_PAUSED=1`。越境の疑い(SEV1)なら最初から `RUNNER_PAUSED=1`。

   ```sh
   cd backend
   go run ./cmd/moderate pause-notify <appId> -env prod -reason "通知の内容を確認しています" -allow-prod-db        # 何をするか見る
   go run ./cmd/moderate pause-notify <appId> -env prod -reason "通知の内容を確認しています" -allow-prod-db -yes   # 実行
   ```

   ```sh
   gcloud run services update <runner のサービス> --region=asia-northeast1 --project=jphacks-kz2601 \
     --update-env-vars=RUNNER_NOTIFY_PAUSED=1
   ```

2. **範囲を確かめる**。Cloud Logging で `jsonPayload.event="runner.run"` を `app_id` で絞り、ルール・結果・宛先の数を見る(中身と uid はログに無い)。`moderate reports` でそのアプリへの通報を見る。
3. **アプリ(またはルール)を止める**。書き込みも止めたい・規約に当たる疑いが強い時。

   ```sh
   go run ./cmd/moderate stop-app <appId> -env prod -reason "規約に反する通知を送っていたため止めました" -allow-prod-db -yes
   go run ./cmd/moderate stop-app <appId> -env prod -reason "..." -force -allow-prod-db -yes     # 書き込みも断る
   go run ./cmd/moderate stop-rule <appId> <ruleId> -env prod -reason "..." -allow-prod-db -yes
   ```

4. 規約に当たれば非公開化(`hide-app`・[moderation.md](../infra/moderation.md))、悪質ならアカウントの無効化(人)。作成者への理由の表示は #347。

`-env` は runner の環境(`prod` / `stg` / `local`)。本番の Firestore には `-allow-prod-db` が無いと動かない。`-yes` が無いと書かずに何をするかだけ出す。止めた記録は `runner/{env}/stops/*` に残る(誰が・いつ・理由・解いた時刻)。`go run ./cmd/moderate stops -env prod -allow-prod-db` で一覧。止める・解くたびに `runner/{env}/stop_log` にも 1 件ずつ足す(操作した人 `-operator`(既定 `$USER`)・理由・期限・強制か。自動の一時停止も)。`go run ./cmd/moderate stop-log [appId] -env prod -allow-prod-db` で新しい順に見る。

### 戻し方

- 全体・アプリ・ルール・アプリの通知: `resume-all` / `resume-app <appId>` / `resume-rule <appId> <ruleId>` / `resume-notify <appId>`(それぞれ `-env prod -allow-prod-db -yes`)。自動の一時停止も `resume-notify` で解く。解いた後は、それまでの通報では自動で止め直さない(新しく 3 人の通報があれば止まる)。
- 環境の `RUNNER_NOTIFY_PAUSED` / `RUNNER_PAUSED`: `--remove-env-vars=RUNNER_NOTIFY_PAUSED`(または `=0`)。
- 止めていた間のきっかけは実行されない(後からまとめて送らない)。必要なら作成者に知らせる。
- 振り返り(§10)を書く。
