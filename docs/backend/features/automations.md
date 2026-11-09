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
| ルールの一覧 | 作成者の DSL の最新の版の `serverRules`。説明は `DescribeServerRule`(日本語) |
| どの結果を見せるか | 共有・公開のデータで動いた結果(uid "")と、作成者本人の private のデータで動いた結果のうち新しい方。他の人の private のデータの結果は読まない |
| 結果の語 | `internal/runner/status` が runner の outcome / reason を利用者向けの決まった語に変える(openapi `AutomationResult`)。上限の種類・停止の段・理由のコードは返さない |
| 運営の停止 | アプリ・ルール・全体(`stop-all`)の停止と通知の停止が**効いているか**と、自動で解ける時刻だけ(`store.StopState`)。全体の停止は `stopped` に出る。理由・誰が止めたか・通報の数は返さない。appconfig は停止を書けない・解けない(#337)。止め方は [runner.md の止め方](../../infra/runner.md#止め方)(`go run ./cmd/moderate`) |
| 停止が効くまで | runner は実行の直前に停止を読む。止める前に outbox に積まれていたきっかけも、次の tick(毎分)で `skipped_stopped` になり、解いた後も動かない(#346・`TestStopCatchesQueuedTriggers`)。`-force` の時は、ルールのきっかけになる書き込みがその場で 403 `app_stopped` |
| 今日の数 | runner の上限の数え方(アプリの実行 `rx_app_{appId}`)で、最後の実行が見た数と上限。日本時間の日が変わっていれば 0。返金(あきらめた実行)は反映しない |

## 環境変数

| 変数 | 意味 |
|---|---|
| `APP_ENV` | 読む `runner/{env}`(outbox と同じ決め方。未設定は `local`・`staging` は `stg`) |
