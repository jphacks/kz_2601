SHELL := /bin/bash
PROJECT ?= $(shell gcloud config get-value project 2>/dev/null)
REGION  ?= asia-northeast1

.PHONY: build vet test lint schema-sync dev emulator run-appconfig run-ai

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
