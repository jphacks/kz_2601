# backend/Makefile の末尾から -include される。レシピは backend/ を作業ディレクトリとして動く(make -C backend)。

# App Store Server API で Pro の状態を取り直す照合(#315)をエミュレータに対して試す。既定は -dry-run。
# APPSTORE_API_* は自分で用意した sandbox 用のキーを環境変数で渡す(ファイルに書かない)。
# 本番は Cloud Run Job(docs/infra/app-store-notifications.md)。ローカルから stg/prod に向けない。
APPSTORE_RECONCILE_FLAGS ?= -dry-run
.PHONY: run-appstore-reconcile
run-appstore-reconcile:
	FIRESTORE_EMULATOR_HOST=localhost:8089 GOOGLE_CLOUD_PROJECT=demo-kz2601 \
	go run ./cmd/appstore-reconcile $(APPSTORE_RECONCILE_FLAGS)
