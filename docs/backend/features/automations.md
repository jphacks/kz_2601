# 自動の動きの見える化(#347)

> [バックエンド設計](../architecture.md) から分けた機能の節。

作成者が「自分のアプリの自動の動き(`serverRules`)が動いたか・止まったか」を iOS の設定から見られるようにする。

```
runner/{env}/rule_status/{RuleStatusID(appId, uid, ruleId)}
  # {appId, ruleId, at, outcome, reason, notifyPaused, lastOkAt, quotaDay, quotaUsed, quotaLimit, expireAt}
  # uid は "" (shared / public) か private のデータの持ち主。文書には uid を書かない(id はハッシュ)
```

| 項目 | 決定 |
|---|---|
| 書く | runner が 1 回の実行(ルールごと)の終わりに上書きする(`runner.run` のログと同じ所)。`lastOkAt` と今日の数は、持っていない実行では前の値を残す。失敗しても実行の結果は変えない(ログだけ) |
| 中身 | ルールの ID・結果のコード・時刻・アプリの今日の実行の数だけ。レコードの値・通知の本文・uid は持たない(決定 337「実行の履歴」) |
| 消える | `expireAt`(最後の実行から 30 日)。TTL ポリシーは人の作業: `gcloud firestore fields ttls update expireAt --collection-group=rule_status --enable-ttl --project=<PROJECT_ID>` |
| 読む | `GET /apps/{appId}/automations`(openapi `Automations`)。作成者(`app_owners`)だけ。参加者・他人は 403。outbox は読まない |
| ルールの一覧 | 作成者の DSL の最新の版の `serverRules`。説明は `DescribeServerRuleIn`(Accept-Language が en なら英語・それ以外は日本語) |
| どの結果を見せるか | 共有・公開のデータで動いた結果(uid "")と、作成者本人の private のデータで動いた結果のうち新しい方。他の人の private のデータの結果は読まない |
| 結果の語 | `internal/runner/status` が runner の outcome / reason を利用者向けの決まった語に変える(openapi `AutomationResult`)。上限の種類・停止の段・理由のコードは返さない |
| 運営の停止 | アプリ・ルール・全体(`stop-all`)の停止と通知の停止が**効いているか**と、自動で解ける時刻だけ(`store.StopState`)。全体の停止は `stopped` に出る。理由・誰が止めたか・通報の数は返さない。appconfig は停止を書けない・解けない(#337)。止め方は [runner.md の止め方](../../infra/runner.md#止め方)(`go run ./cmd/moderate`) |
| 停止が効くまで | runner は実行の直前に停止を読む。止める前に outbox に積まれていたきっかけも、次の tick(毎分)で `skipped_stopped` になり、解いた後も動かない(#346・`TestStopCatchesQueuedTriggers`)。`-force` の時は、ルールのきっかけになる書き込みがその場で 403 `app_stopped` |
| 今日の数 | runner の上限の数え方(アプリの実行 `rx_app_{appId}`)で、最後の実行が見た数と上限。日本時間の日が変わっていれば 0。返金(あきらめた実行)は反映しない |

## 上限が近い・上限に達した時のプッシュ(#345)

アプリの今日の実行(`rx_app_{appId}`)が上限の 80% に達した時と、上限で止まった時に、作成者へプッシュを 1 回ずつ送る(決定 338「80% / 100% で 1 日 1 回」)。

| 項目 | 決定 |
|---|---|
| いつ | 実行の数を数えた直後(`internal/runner/quota_warn.go`)。80% は実行できた回で数が上限の 80% 以上、100% はアプリの実行の上限で `skipped_quota` になった回。作成者・全体の上限はこの知らせを出さない。再試行の回は数を読まないので出さない |
| 1 日 1 回 | 上限 1 の数 `rq_warn_app_{appId}_{80|100}`(日本時間の日ごと・上限と同じ ratelimit の store)を 1 足せた 1 台だけが送る。複数の runner でも重ならない。送れなかった時も同じ日は送り直さない |
| 宛先 | 作成者の端末(ルールの通知と同じ `users/{uid}/devices`)。同意・宛先の上限には数えない(知らせ自体は枠に数えない) |
| 文面 | 日本語だけ(作成者の言語は持っていない)。「『{アプリ名}』の自動の動きが、今日の上限の 80% に達しました」/「『{アプリ名}』の自動の動きは、今日の上限に達したため、今日は止まっています。明日また動きます」。数は割合だけ |
| 止める | `RUNNER_PAUSED`・運営の停止で実行しない時は出さない。通知の停止(`RUNNER_NOTIFY_PAUSED`・アプリの通知停止)の間は数えずに出さない(解けた後の同じ日の実行で送る) |
| 失敗 | 送れなくても実行の結果は変えない。ログ `runner.quota_warn`(`app_id`・`day`・`level`・`result`)を 1 日 1 回ぶん書く |

## 環境変数

| 変数 | 意味 |
|---|---|
| `APP_ENV` | 読む `runner/{env}`(outbox と同じ決め方。未設定は `local`・`staging` は `stg`) |
