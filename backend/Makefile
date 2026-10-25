SHELL := /bin/bash
PROJECT ?= $(shell gcloud config get-value project 2>/dev/null)
REGION  ?= asia-northeast1

.PHONY: build vet test lint schema-sync dev emulator run-appconfig run-ai smoke-local smoke-stg smoke-prod demo-check

build:
	go build ./...

vet:
	go vet ./...

test: schema-check
	go test ./...

lint: vet
	@command -v staticcheck >/dev/null && staticcheck ./... || echo "staticcheck not installed (go install honnef.co/go/tools/cmd/staticcheck@latest)"

# docs/dsl/dsl.schema.json is canonical; the Go package embeds a copy.
schema-sync:
	cp ../docs/dsl/dsl.schema.json pkg/dsl/schema.json

schema-check:
	@cmp -s ../docs/dsl/dsl.schema.json pkg/dsl/schema.json || (echo "schema out of sync: run make schema-sync" && exit 1)

# ---- local dev -------------------------------------------------------------
# Terminal 1: make emulator   Terminal 2: make dev
emulator:
	firebase emulators:start --only firestore,auth --project demo-kz2601

dev:
	@echo "appconfig :8080 / ai :8081 (Ctrl-C stops both)"
	@trap 'kill 0' INT; \
	  $(MAKE) run-appconfig & \
	  $(MAKE) run-ai & \
	  wait

run-appconfig:
	FIRESTORE_EMULATOR_HOST=localhost:8089 FIREBASE_AUTH_EMULATOR_HOST=localhost:9099 \
	GOOGLE_CLOUD_PROJECT=demo-kz2601 AUTH_MODE=dev PORT=8080 go run ./cmd/appconfig

run-ai:
	FIREBASE_AUTH_EMULATOR_HOST=localhost:9099 GOOGLE_CLOUD_PROJECT=demo-kz2601 AUTH_MODE=dev \
	APPCONFIG_BASE_URL=http://localhost:8080 PORT=8081 go run ./cmd/ai

# ---- smoke (docs/03_demo_script.md の 3 本 + undo。Claude を 3 回呼ぶ) -------------
AI_STG_URL        ?= https://ai-stg-378953239426.asia-northeast1.run.app
APPCONFIG_STG_URL ?= https://appconfig-stg-378953239426.asia-northeast1.run.app
SMOKE_ONLY        ?=
SMOKE_FLAGS        = $(if $(SMOKE_ONLY),-only $(SMOKE_ONLY),)

# ローカル 2 プロセス(appconfig: memory store + AUTH_MODE=dev / ai: ../.env の ANTHROPIC_API_KEY)を
# :18080/:18081 で起動 → smoke → 終了。SMOKE_ONLY=kakeibo で 1 本だけ。
smoke-local:
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
smoke-stg:
	go run ./cmd/smoke -ai $(AI_STG_URL) -appconfig $(APPCONFIG_STG_URL) -firebase-api-key "$(FIREBASE_WEB_API_KEY)" $(SMOKE_FLAGS)

# production。Claude 費用を抑えるため家計簿 1 本(create → edit → undo・Claude 2 回)だけ、
# 終了時に作った smoke-* アプリを archive(-cleanup・DELETE /v1/apps/{id})する。
AI_PROD_URL        ?= https://ai-prod-zgciuf3iha-an.a.run.app
APPCONFIG_PROD_URL ?= https://appconfig-prod-zgciuf3iha-an.a.run.app
SMOKE_PROD_ONLY    ?= kakeibo

smoke-prod:
	go run ./cmd/smoke -ai $(AI_PROD_URL) -appconfig $(APPCONFIG_PROD_URL) -firebase-api-key "$(FIREBASE_WEB_API_KEY)" -only $(SMOKE_PROD_ONLY) -cleanup

# デモ前チェック兼ウォームアップ(#171)。curl だけで動く(Go 不要)。ENV=stg|prod、審査の 30 分前に ENV=prod で 1 回。
ENV ?= stg
demo-check:
	../scripts/demo-check.sh $(ENV)
