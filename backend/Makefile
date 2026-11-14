SHELL := /bin/bash
PROJECT ?= $(shell gcloud config get-value project 2>/dev/null)
REGION  ?= asia-northeast1

.PHONY: build
build: ## Go のコードをビルドする
	go build ./...

# ターゲットの一覧(#2798)。各ターゲットの行の後ろの `## 説明` を集めて名前順に出す。
# 説明の無いターゲットは最後に「説明なし」としてまとめる。
.PHONY: help
help: ## ターゲットと 1 行の説明を出す
	@awk '/^[a-zA-Z0-9_-]+:/ { \
	    name = $$0; sub(/:.*/, "", name); \
	    if (name in seen) next; \
	    seen[name] = 1; \
	    if (match($$0, /## /)) printf "D\t%s\t%s\n", name, substr($$0, RSTART + 3); \
	    else printf "U\t%s\n", name; \
	  }' $(MAKEFILE_LIST) \
	  | sort \
	  | awk -F'\t' '$$1 == "D" { printf "  %-24s %s\n", $$2, $$3; next } \
	                { if (!noted++) print "説明なし:"; printf "  %s\n", $$2 }'

.PHONY: vet
vet: ## go vet ./... で気になる書き方を見つける
	go vet ./...

.PHONY: test
test: schema-check ## スキーマの同期を確かめてから go test ./... を回す
	go test ./...

.PHONY: lint
lint: vet ## vet と staticcheck(入っていれば)を回す
	@command -v staticcheck >/dev/null && staticcheck ./... || echo "staticcheck not installed (go install honnef.co/go/tools/cmd/staticcheck@latest)"

# docs/dsl/dsl.schema.json is canonical; the Go package embeds a copy.
.PHONY: schema-sync
schema-sync: ## docs/dsl/dsl.schema.json を pkg/dsl/schema.json に写す
	cp ../docs/dsl/dsl.schema.json pkg/dsl/schema.json

.PHONY: schema-check
schema-check: ## pkg/dsl/schema.json が docs の正本と同じか確かめる
	@cmp -s ../docs/dsl/dsl.schema.json pkg/dsl/schema.json || (echo "schema out of sync: run make schema-sync" && exit 1)

# Android の DSL の型(android/dsl/.../generated)をスキーマから作り直す(#351)。
# 古いままだと cmd/dslgen の TestGeneratedKotlinIsUpToDate が落ちる。
.PHONY: dsl-gen
dsl-gen: schema-check ## スキーマから Android の DSL の型を作り直す
	go run ./cmd/dslgen

# ---- local dev -------------------------------------------------------------
# Terminal 1: make emulator   Terminal 2: make dev
# Firestore エミュレータは CI と同じ gcloud の cloud-firestore-emulator(firebase-tools・firebase.json は不要)。
# Auth エミュレータは起動しない(FIREBASE_AUTH_EMULATOR_HOST は AUTH_MODE=dev で X-Debug-Uid を許す目印)。
# 8089 が塞がっている時は FIRESTORE_EMULATOR_HOST=127.0.0.1:18089 make emulator のように変える。
.PHONY: emulator
emulator: ## Firestore エミュレータを起動する(別の端末で動かす)
	../scripts/firestore-emulator.sh start

# エミュレータ試験を CI(backend-emulator)と同じ判定で回す。エミュレータが無ければ一時的に起動して止める。
.PHONY: test-emulator
test-emulator: ## エミュレータ試験を CI と同じ判定で回す
	../scripts/firestore-emulator.sh test-all

.PHONY: dev
dev: ## appconfig(:8080)と ai(:8081)をまとめてローカルで動かす
	@echo "appconfig :8080 / ai :8081 (Ctrl-C stops both)"
	@trap 'kill 0' INT; \
	  $(MAKE) run-appconfig & \
	  $(MAKE) run-ai & \
	  wait

.PHONY: run-appconfig
run-appconfig: ## appconfig をエミュレータ向けに :8080 で動かす
	FIRESTORE_EMULATOR_HOST=localhost:8089 FIREBASE_AUTH_EMULATOR_HOST=localhost:9099 \
	GOOGLE_CLOUD_PROJECT=demo-kz2601 AUTH_MODE=dev PORT=8080 go run ./cmd/appconfig

.PHONY: run-ai
run-ai: ## ai をエミュレータ向けに :8081 で動かす
	FIREBASE_AUTH_EMULATOR_HOST=localhost:9099 GOOGLE_CLOUD_PROJECT=demo-kz2601 AUTH_MODE=dev \
	APPCONFIG_BASE_URL=http://localhost:8080 PORT=8081 go run ./cmd/ai

# ---- smoke (docs/03_demo_script.md の 3 本 + undo。Claude を 3 回呼ぶ) -------------
AI_STG_URL        ?= https://ai-stg-378953239426.asia-northeast1.run.app
APPCONFIG_STG_URL ?= https://appconfig-stg-378953239426.asia-northeast1.run.app
SMOKE_ONLY        ?=
SMOKE_FLAGS        = $(if $(SMOKE_ONLY),-only $(SMOKE_ONLY),)

# ローカル 2 プロセス(appconfig: memory store + AUTH_MODE=dev / ai: ../.env の ANTHROPIC_API_KEY)を
# :18080/:18081 で起動 → smoke → 終了。SMOKE_ONLY=kakeibo で 1 本だけ。
.PHONY: smoke-local
smoke-local: ## ローカルの 2 プロセスを起動して smoke を流す
	@set -e; set -a; . ../.env; set +a; \
	  STORE=memory AUTH_MODE=dev PORT=18080 go run ./cmd/appconfig >/tmp/smoke-appconfig.log 2>&1 & A=$$!; \
	  AUTH_MODE=dev APPCONFIG_BASE_URL=http://localhost:18080 PORT=18081 go run ./cmd/ai >/tmp/smoke-ai.log 2>&1 & B=$$!; \
	  trap 'pkill -P $$A $$B 2>/dev/null; kill $$A $$B 2>/dev/null; true' EXIT; \
	  for i in $$(seq 1 45); do \
	    curl -fsS localhost:18080/health >/dev/null 2>&1 && curl -fsS localhost:18081/health >/dev/null 2>&1 && break; sleep 2; \
	  done; \
	  go run ./cmd/smoke -ai http://localhost:18081 -appconfig http://localhost:18080 $(SMOKE_FLAGS)

# staging(appconfig は AUTH_MODE=firebase)。FIREBASE_WEB_API_KEY(Firebase Web API key・公開情報)で
# 匿名ユーザーの ID トークンを取る。無ければ health と 401 確認だけにフォールバック。
.PHONY: smoke-stg
smoke-stg: ## staging に向けて smoke を流す
	go run ./cmd/smoke -ai $(AI_STG_URL) -appconfig $(APPCONFIG_STG_URL) -firebase-api-key "$(FIREBASE_WEB_API_KEY)" $(SMOKE_FLAGS)

# production。Claude 費用を抑えるため家計簿 1 本(create → edit → undo・Claude 2 回)だけ、
# 終了時に作った smoke-* アプリを archive(-cleanup・DELETE /v1/apps/{id})する。
AI_PROD_URL        ?= https://ai-prod-zgciuf3iha-an.a.run.app
APPCONFIG_PROD_URL ?= https://appconfig-prod-zgciuf3iha-an.a.run.app
SMOKE_PROD_ONLY    ?= kakeibo

.PHONY: smoke-prod
smoke-prod: ## production に向けて smoke を 1 本流して後片付けする
	go run ./cmd/smoke -ai $(AI_PROD_URL) -appconfig $(APPCONFIG_PROD_URL) -firebase-api-key "$(FIREBASE_WEB_API_KEY)" -only $(SMOKE_PROD_ONLY) -cleanup

# デモ前チェック兼ウォームアップ(#171)。curl だけで動く(Go 不要)。ENV=stg|prod、審査の 30 分前に ENV=prod で 1 回。
ENV ?= stg
.PHONY: demo-check
demo-check: ## デモ前のチェックとウォームアップを curl だけで行う(ENV=stg|prod)
	../scripts/demo-check.sh $(ENV)

# 運用コマンド(機能ごと)は mk/<feature>.mk に置く。末尾で読むので既定ターゲットは build のまま。
-include $(wildcard mk/*.mk)
