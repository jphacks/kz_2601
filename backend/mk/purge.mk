# backend/Makefile の末尾から -include される。レシピは backend/ を作業ディレクトリとして動く(make -C backend)。

# 退会から 30 日を過ぎたアカウントの消去(#458)をエミュレータに対して試す。既定は -dry-run。
# 本番は Cloud Run Job(docs/infra/account-purge.md)。ローカルから stg/prod に向けない。
PURGE_FLAGS ?= -dry-run
.PHONY: run-purge
run-purge:
	FIRESTORE_EMULATOR_HOST=localhost:8089 FIREBASE_AUTH_EMULATOR_HOST=localhost:9099 \
	GOOGLE_CLOUD_PROJECT=demo-kz2601 go run ./cmd/purge $(PURGE_FLAGS)
