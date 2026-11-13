# 段階的なリリース: Cloud Run のトラフィックの分割と自動の巻き戻し(#372)

新しいリビジョンに少しずつトラフィックを移し(既定 5% → 25% → 100%)、段階ごとに新旧のリビジョンの指標を比べ、悪ければ前のリビジョンに 100% 戻すスクリプト `scripts/rollout.sh` の使い方です。

> **deploy の workflow(`.github/workflows/deploy-cloudrun.yml`)には stg だけ組み込んであります。** develop への push は今まで通り 100% で出し、rollout.sh は dry-run(コマンドを出すだけ)。実際に段階を踏むのは、人が workflow_dispatch で `environment=staging`・`staged_rollout=execute` を選んだ時だけです。prod は変えていません。stg / prod で実際に動かしたことはまだありません(下の「人の作業」)。

## 目次

1. [流れ](#流れ)
2. [使い方](#使い方)
3. [比べる指標](#比べる指標)
4. [閾値の選び方](#閾値の選び方)
5. [人が途中で止める](#人が途中で止める)
6. [deploy の workflow での動き](#deploy-の-workflow-での動き)
7. [どの版で生成したかの記録](#どの版で生成したかの記録)
8. [人の作業 1: 権限と prod への組み込み](#人の作業-1-権限と-prod-への組み込み)
9. [人の作業 2: stg でわざと悪い版を出して戻ることを確かめる](#人の作業-2-stg-でわざと悪い版を出して戻ることを確かめる)
10. [わかっている限界](#わかっている限界)

## 流れ

```mermaid
flowchart LR
  D["deploy --no-traffic"] --> S1["新 5%"]
  S1 -->|"窓を待つ・比べる"| J1{"判定"}
  J1 -->|promote| S2["新 25%"]
  S2 --> J2{"判定"}
  J2 -->|promote| S3["新 100%"]
  S3 --> J3{"判定"}
  J3 -->|promote| OK["終わり(0)"]
  J1 -->|rollback| RB["旧 100%(3)"]
  J2 -->|rollback| RB
  J3 -->|rollback| RB
```

1. deploy の workflow が `gcloud run deploy --no-traffic` で新しいリビジョンを作る(トラフィックは旧のまま)
2. `rollout.sh` が `gcloud run services update-traffic --to-revisions=NEW=5,OLD=95` で 5% を移す
3. `--wait` 秒(既定 300)待ち、その窓のログから新旧の指標を出して判定する
   - **promote**: 次の段階へ
   - **rollback**: `--to-revisions=OLD=100` で戻し、`$GITHUB_STEP_SUMMARY`(あれば)と標準エラーに理由を出して終了コード 3
   - **insufficient**(新のリクエストが少なすぎる): 窓を延ばす(既定 2 回まで)。それでも足りなければ既定は巻き戻し(終了コード 4)
4. 100% の段階も 1 窓見る(旧は受けていないので、新の絶対値だけで判定)

## 使い方

```sh
# dry-run(既定): 出すコマンドを見るだけ。GCP には触らない・認証も要らない
scripts/rollout.sh --env stg --service appconfig

# stg で実行(新 = 最後に作ったリビジョン・旧 = 今いちばん多く受けているリビジョン)
scripts/rollout.sh --env stg --service ai --execute

# prod は --env prod と --execute の両方を書いた時だけ実行する
scripts/rollout.sh --env prod --service appconfig --execute --steps 5,25,50,100 --wait 600

# 模擬: 段階ごとの指標を DIR/<割合>.json から読む(dry-run のまま判定と巻き戻しの流れを見る)
scripts/rollout.sh --env stg --service ai --to-revision new --from-revision old --wait 0 \
  --metrics-dir scripts/testdata/rollout/sim-bad

# 判定だけ / ログの JSON から指標だけ(オフライン)
scripts/rollout.sh --decide metrics.json
scripts/rollout.sh --summarize logs.json --new ai-stg-00002-abc --old ai-stg-00001-xyz
```

| オプション | 既定 | 中身 |
| --- | --- | --- |
| `--env` | (必須) | `stg` か `prod`。サービス名の接尾辞と合わないと止まる(`--env stg --service appconfig-prod` は不可) |
| `--service` | (必須) | `appconfig` / `ai`(`-<env>` を足す)か、`appconfig-stg` のような完全な名前 |
| `--execute` / `--dry-run` | dry-run | 実行するか。`--execute` が無ければ prod でも stg でも何も変えない |
| `--to-revision` / `--from-revision` | 自動 | 新旧のリビジョン。無ければ `gcloud run services describe` から読む |
| `--steps` | `5,25,100` | 新に回す割合。1..100 の増える順・最後は 100 |
| `--wait` | `300` | 1 つの窓の秒数 |
| `--max-extends` | `2` | 指標が足りない時に窓を延ばす回数 |
| `--on-insufficient` | `rollback` | 延ばしても足りない時。`promote` で進める(トラフィックが少ない stg 向け) |
| `--metrics-dir` | なし | 模擬。dry-run だけで使える |

環境変数: `PROJECT_ID`(既定 `jphacks-kz2601`)・`REGION`(既定 `asia-northeast1`)・`LIMIT`(1 窓で読むログの行数・既定 20000)と、下の `ROLLOUT_*`。

要る権限(`--execute`): `roles/run.developer`(update-traffic)と `roles/logging.viewer`。deploy の SA はもう `run deploy` ができるので、組み込みの時に `logging.viewer` があるかを IAM で見ます。

## 比べる指標

ログ(`gcloud logging read`)から、窓の中の新旧のリビジョン(`resource.labels.revision_name`)ごとに出します。形は [observability.md](observability.md) の「ログの形」と [ai-cost.md](ai-cost.md)。

| 指標 | もと | 使うもの |
| --- | --- | --- |
| リクエスト数・5xx 率 | `jsonPayload.event="http.request"`(`/health` は除く) | `status >= 500` の割合 |
| p95 の待ち時間 | 同じ | `duration`(ns → ms)の 95 パーセンタイル |
| Claude の失敗率(ai) | `jsonPayload.event="ai.usage"` | `outcome="error"` の割合 |
| 1 回あたりの費用(ai) | 同じ | `est_cost_usd`(#298)がある行の平均。単価の無いモデルの行は数えない |

**なぜ Cloud Monitoring の `request_count` / `request_latencies` でなくログか**: 監視のアラート([observability.md](observability.md)・`infra/monitoring.tf`)は Cloud Run の組み込みの指標を使っていますが、`gcloud monitoring` には時系列を読むコマンドが無く、Monitoring API を `curl` で叩くことになります。ログなら `scripts/ai-cost.sh` と同じ `gcloud logging read` + `jq` で済み、判定の入力をそのままテストの固定データにできます。ログベースの指標 `ai_claude_calls` / `backend_panics` もログが元なので、見ているものは同じです。違いは、Cloud Run 自身が返した 503(コンテナに届かなかったもの)がログの `http.request` に入らないこと。これが要るなら、`decide` の入力(JSON)はそのままで、`fetch_metrics` を Monitoring API の `timeSeries.list`(`run.googleapis.com/request_count` を `resource.labels.revision_name` で分ける)に替えられます。

費用は新旧の 1 回あたりを比べます。プロンプトやモデルの変更で 1 回の費用が跳ねた版を、5xx や待ち時間が変わらなくても戻すためです。

## 閾値の選び方

判定は `decide`(純粋な関数・`scripts/rollout-test.sh` で確かめる)。**どれか 1 つでも超えたら rollback**、超えていなくて新のリクエストが少なければ insufficient、それ以外は promote。「ちょうど」は超えていない扱いです。

| 環境変数 | 既定 | 意味 | 選び方 |
| --- | --- | --- | --- |
| `ROLLOUT_MAX_ERROR_RATE` | `0.05` | 新の 5xx 率の上限 | 監視のアラート `error_rate_threshold`(5%)と揃える。リリースで止める線がアラートより緩いと、戻さずに鳴る |
| `ROLLOUT_MAX_ERROR_RATE_DELTA` | `0.01` | 新 − 旧の 5xx 率の差の上限 | 旧も同じ窓で受けているので、外部の障害(Firestore・Claude の落ち)は両方に出て差は小さい。新だけが悪いことを見る線。普段の 5xx 率の揺れ(Logs Explorer で数日分)の 2〜3 倍 |
| `ROLLOUT_MAX_P95_RATIO` | `1.5` | 新の p95 / 旧の p95 の上限 | ai は Claude の待ちで p95 が数十秒あり揺れが大きいので、ai だけ 2 にしてもよい |
| `ROLLOUT_P95_SLACK_MS` | `100` | 差がこれ以下なら比が大きくても通す | appconfig の p95 は数十 ms なので、比だけだと 20ms → 40ms で落ちる。ユーザーが気づかない差 |
| `ROLLOUT_MAX_CLAUDE_FAILURE_RATE` | `0.2` | 新の Claude の失敗率の上限 | 監視の `claude_failure_rate_threshold`(20%)と揃える |
| `ROLLOUT_CLAUDE_FAILURE_MIN_COUNT` | `3` | 失敗がこの件数未満なら率は見ない | 監視の `claude_failure_min_count` と揃える。5% の段階では呼び出しが数件なので、1 件の失敗で戻さないため |
| `ROLLOUT_MAX_COST_RATIO`(`--max-cost-ratio`) | `2` | 新の 1 回あたりの費用 / 旧の上限 | 緩めにしている。キャッシュの読みの割合や依頼の中身で 1 回の費用は揺れるので、普段の揺れ(`scripts/ai-cost.sh` で数日分)より大きく取る。費用を上げる変更を意図して出す時は引数で上げる |
| `ROLLOUT_COST_MIN_CALLS` | `5` | 新旧どちらかの費用つきの呼び出しがこれ未満なら費用は見ない | 5% の段階では呼び出しが数件なので、1 件の重い依頼で戻さないため。旧が 0 件(100% の段階)なら比べない |
| `ROLLOUT_MIN_REQUESTS` | `20` | 新がこれ未満なら判定しない | 5% の 1 窓に 20 件来るには、全体で 5 分に 400 件。**stg は足りないことが多い**ので、stg は `--on-insufficient promote` か、ジョブの中で負荷(スモーク)を流す |

段階と窓: 5% の窓で 20 件を超える程度のトラフィックがあること、が段階と `--wait` を決める目安です。prod の今のトラフィックなら、5% の段階は `--wait 600` 以上を勧めます。段階を細かくするほど、悪い版を受けるユーザーは減るがリリースは遅くなります。

## 人が途中で止める

- 手元: Ctrl-C。ジョブ: GitHub Actions の Cancel(SIGINT → SIGTERM)
- どちらも、旧のリビジョンに 100% 戻してから終了コード 3 で終わります(待っている間でもすぐ。`sleep` を裏で動かして `wait` しているため)
- 戻さずに今の割合のまま止めたい時は、ジョブを止めずに(または SIGKILL で)止め、あとは手で `gcloud run services update-traffic` を打つ

## deploy の workflow での動き

`deploy-cloudrun.yml` の `deploy-changes` ジョブが `rollout`(`off` / `dry-run` / `execute`)を決め、`deploy` ジョブは `ROLLOUT_MODE` で分かれます。

| いつ | `ROLLOUT_MODE` | 何が起きるか |
| --- | --- | --- |
| main への push・`environment=production` | `off` | 今まで通り 100% で出す。`staged_rollout=execute` を選んでも無視して warning を出す |
| develop への push・`environment=staging`(既定) | `dry-run` | 今まで通り 100% で出し、最後に `rollout.sh`(dry-run)が打つはずのコマンドを log に出す。GCP には触らない・失敗してもデプロイは成功扱い |
| workflow_dispatch で `environment=staging`・`staged_rollout=execute` | `execute` | appconfig を `--no-traffic` で出す → `rollout.sh --env stg --service appconfig --execute --wait 120 --on-insufficient promote` → ai も同じ順で。その後にヘルスチェックとスモーク |

- 巻き戻した時(終了コード 3 / 4)はステップが失敗し、ジョブも失敗する。理由は job summary(`### rollout <svc>: rolled back`)と `::error` 注釈(run の画面と Checks に出る)に出る。ジョブの失敗はいつもの GitHub の通知で届く
- ai は appconfig の段階が終わってから進める(同時に進めると、どちらの版が悪いか分からない)
- ジョブの `timeout-minutes` は execute の時だけ 70(各サービスの段階に 25 分まで)
- `scripts-ci.yml` で `scripts/rollout-test.sh` を回す(判定・巻き戻し・通知を偽の gcloud で確かめる)
- 巻き戻した後は次の deploy まで旧のまま。新のリビジョンは 0% で残る(消さない)

## どの版で生成したかの記録

生成した版(`users/{uid}/apps/{appId}/versions/{n}` の `source`)に、モデル(`model`・前からある)とプロンプトの版(`promptVersion`)を残します。`ai.generate` のログにも `model` と `prompt_version` を出します。

- `promptVersion` は `p-` + プロンプト本文(`systemPrompt` と `editRules`)の sha256 の先頭 12 桁(`backend/internal/ai/prompt_version.go`)。プロンプトやレシピの文を変えると自動で変わる。手で番号を上げる必要は無い
- その呼び出しで送ったレシピは、今まで通り `Meta.Recipes`
- 段階の途中で新旧を分けて見たい時は、Logs Explorer で `jsonPayload.prompt_version` ごとに `ai.generate` の `outcome` を数える

## 人の作業 1: 権限と prod への組み込み

| # | やること | 気をつけること |
| --- | --- | --- |
| 1 | deploy の SA に `roles/logging.viewer` を足す(`infra/` の IAM) | 無いと最初の判定でログが読めず、毎回旧に戻る(下の限界)。stg で execute を使う前に要る |
| 2 | prod にも入れるか決める | 入れるなら `deploy-changes` の `rollout` を prod でも `execute` にし、`--env prod --wait 600`(`--on-insufficient` は既定の rollback)。承認を得てから |
| 3 | (任意)`--tag` | `gcloud run deploy --no-traffic --tag=sha-xxxxxxx` にすると、新だけを `https://sha-xxxxxxx---appconfig-stg-....run.app` で叩けて、スモークを 0% のうちに流せる。タグは溜まるので消す手順とあわせて |

## 人の作業 2: stg でわざと悪い版を出して戻ることを確かめる

Issue の完了の条件「stg でわざと悪い版を出すと、自動で前の版に戻る」の確かめ方です。手元から stg に対して行えます(要る権限は上)。workflow で確かめるなら、悪い版のブランチから workflow_dispatch(`environment=staging`・`staged_rollout=execute`)で出し、その間 stg に負荷を流します。

1. 今の stg のリビジョンを控える: `gcloud run services describe appconfig-stg --project=jphacks-kz2601 --region=asia-northeast1 --format='value(status.traffic)'`
2. わざと悪い版を、トラフィック 0 で出す(例: 全リクエストで 500 を返す一行の変更を入れたイメージ・または `/debug/error` を叩く負荷)。`gcloud run deploy appconfig-stg --image=<悪いイメージ> --no-traffic --tag=bad ...`
3. 段階を短くして実行する: `scripts/rollout.sh --env stg --service appconfig --execute --steps 50,100 --wait 120`
4. その間、別の端末で stg の URL に 1 秒 1 回ほどリクエストを流す(5xx の判定に 20 件以上要る)
5. `step 50%: rollback — 5xx rate …` が出て終了コード 3、`describe` で旧が 100% に戻っていることを見る
6. 良い版(今の develop)でも同じ手順を行い、100% まで進むことを見る

## わかっている限界

- **判定のログが読めない時**(権限・API の失敗)は、悪い版かどうか分からないので **旧に 100% 戻します**(終了コード 3)。権限の付け忘れでも毎回戻るので、組み込んだ最初の deploy で気づけます
- `update-traffic` 自体が失敗した時は、その場で止まり(`set -e`)、トラフィックはその時の割合のままです。ジョブの失敗で気づき、手で戻すか、もう一度実行してください
- ログに出ない 5xx(Cloud Run 自身の 503 など)は数えません(上の「比べる指標」)
- 費用は 1 回あたりの平均で比べます。依頼の中身(長い会話・大きなアプリ)が窓によって偏ると、版が同じでも平均が動きます。倍率を緩め(既定 2)にしているのはこのため。プロンプト・モデルの版は記録する(上)が、判定には使っていません(新旧はリビジョンで分けている)
- 1 窓で読むログは `LIMIT` 行まで。超えると古い行が落ちて指標が偏ります(prod のトラフィックが増えたら Monitoring API に替える)
- ai と appconfig は同じイメージなので、片方だけ戻すと版がずれます(ai は appconfig を HTTP で呼ぶだけなので、API の互換が保たれていれば動く)
