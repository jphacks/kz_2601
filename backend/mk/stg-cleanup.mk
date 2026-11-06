# backend/Makefile の末尾から -include される。レシピは backend/ を作業ディレクトリとして動く(make -C backend)。

# 試験データ(smoke-* アプリ・stg の回数制限・試験ユーザー)の掃除(#293)をエミュレータに対して試す。既定はドライラン。
# stg / prod に向ける手順は docs/infra/stg-cleanup.md(人が実行する)。この target はエミュレータにしか向けない。
STG_CLEANUP_FLAGS ?= -smoke -stg-ratelimits
.PHONY: run-stg-cleanup
run-stg-cleanup:
	FIRESTORE_EMULATOR_HOST=localhost:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 go run ./cmd/stg-cleanup $(STG_CLEANUP_FLAGS)
