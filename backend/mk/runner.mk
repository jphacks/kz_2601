# backend/Makefile の末尾から -include される。レシピは backend/ を作業ディレクトリとして動く(make -C backend)。

# 自動の動き(serverRules)を実行する runner(#341)をエミュレータに対して動かす。:8082 で待ち、10 秒ごとに outbox を処理する。
# きっかけを積むには appconfig を RUNNER_OUTBOX=1 で動かす(RUNNER_OUTBOX=1 make run-appconfig)。
# 本番は未デプロイ(サービスの追加は人の承認の後)。ローカルから stg/prod に向けない。
RUNNER_TICK_INTERVAL ?= 10s
.PHONY: run-runner
run-runner:
	FIRESTORE_EMULATOR_HOST=localhost:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 \
	PORT=8082 RUNNER_TICK_INTERVAL=$(RUNNER_TICK_INTERVAL) go run ./cmd/runner
