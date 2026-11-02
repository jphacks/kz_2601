# backend/Makefile の末尾から -include される。レシピは backend/ を作業ディレクトリとして動く(make -C backend)。

# 運営の通報確認・非公開化(#321)をエミュレータに対して試す: make run-moderate ARGS="reports"
# 本番の手順は docs/infra/moderation.md。書き込みは -yes を付けた時だけ。
.PHONY: run-moderate
run-moderate:
	FIRESTORE_EMULATOR_HOST=localhost:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 go run ./cmd/moderate $(ARGS)
