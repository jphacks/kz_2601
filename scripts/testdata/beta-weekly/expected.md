# β 品質ゲート prod(ai-prod・<TS>〜<TS> UTC・7 日)

| 基準 | 値 | 基準値 | 判定 | 件数・補足 |
|---|---|---|---|---|
| クラッシュなし率(ユーザー) | Crashlytics で見る | 99.5% 以上 | 手で記入 | Firebase Console → Crashlytics |
| 生成の成功率(検証を通る) | 80% | 98% 以上 | 満たさない | 4/5 件(bad_request=1 invalid=1 ok=5) |
| 初回の生成の p95 | 35.0 秒 | 30 秒以内 | 満たさない | 3 件・p50 20.0 秒(サーバー側の時間) |
| 7 日後に開いた率(D7) | scripts/product-metrics-weekly.sh で見る | 記録のみ | 手で記入 | #297 のイベント・docs/metrics/product-metrics.md |
| P1 のフィードバックの残数 | 2 | 0 | 満たさない | 件名「[β]」・priority:P1・open |

参考(基準ではない)

| 項目 | 値 |
|---|---|
| POST /v1/generate のリクエスト(Cloud Run) | 5 件・5xx 1 件・p95 60 秒 |
| Claude の試行(ai.usage) | 4 件・error 1 件 |
