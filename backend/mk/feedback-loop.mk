# backend/Makefile の末尾から -include される。レシピは backend/ を作業ディレクトリとして動く(make -C backend)。

# 自己改善ループ(#364)をエミュレータに対して動かす。LOOP_ARGS="drafts list" など。
# 手順と環境変数: docs/backend/features/feedback-loop.md
LOOP_ARGS ?= ingest
.PHONY: run-feedback-loop
run-feedback-loop:
	FIRESTORE_EMULATOR_HOST=localhost:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 go run ./cmd/feedback-loop $(LOOP_ARGS)
