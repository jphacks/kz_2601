# backend/Makefile の末尾から -include される。レシピは backend/ を作業ディレクトリとして動く(make -C backend)。

# フィードバック(#319)をエミュレータから読む(読み取りだけ)。FEEDBACK_ARGS="show -out /tmp/fb <id>" など。
FEEDBACK_ARGS ?= list
.PHONY: run-feedback
run-feedback:
	FIRESTORE_EMULATOR_HOST=localhost:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 go run ./cmd/feedback $(FEEDBACK_ARGS)
