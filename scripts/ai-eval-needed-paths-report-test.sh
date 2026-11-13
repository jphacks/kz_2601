#!/usr/bin/env bash
# scripts/ai-eval-needed-paths-report.sh を使い捨てのリポジトリで確かめる(ネットワーク不要)。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/ai-eval-needed-paths-report.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qxF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }
put() { mkdir -p "$TMP/repo/$(dirname "$1")"; printf 'x\n' >"$TMP/repo/$1"; }
run() { OUT="$(cd "$TMP/repo" && AI_EVAL_NEEDED_PATHS="$1" "$SCRIPT" 2>&1)" || fail "exit 0 のはず"; }

git init -q "$TMP/repo"
cat >"$TMP/paths" <<'PATHS'
ai backend/internal/ai/
dsl backend/pkg/dsl/
schema docs/dsl/dsl.schema.json
skip *_test.go
PATHS

# 当たる物
put backend/internal/ai/prompt.go
put backend/pkg/dsl/validate.go
put docs/dsl/dsl.schema.json
# 当たらない物(名前が生成に関わりそう)
put backend/internal/appconfig/dsl_rules.go
put backend/internal/other/system_prompt.go
put docs/dsl/extra.schema.json
# 候補ではない物・テスト
put backend/internal/store/store.go
put backend/internal/appconfig/dsl_rules_test.go
git -C "$TMP/repo" add -A

run "$TMP/paths"
has "backend/internal/appconfig/dsl_rules.go"
has "backend/internal/other/system_prompt.go"
has "docs/dsl/extra.schema.json"
hasnt "backend/internal/ai/prompt.go"
hasnt "backend/pkg/dsl/validate.go"
hasnt "store.go"
hasnt "_test.go"
has "ai-eval-needed-paths-report: 候補 6 件のうち一覧に当たらない 3 件(足すかどうかは人が決める)"

# 全部当たる時は 0 件・それでも exit 0
cat >>"$TMP/paths" <<'PATHS'
ai backend/internal/appconfig/
ai backend/internal/other/
schema docs/dsl/extra.schema.json
PATHS
run "$TMP/paths"
has "ai-eval-needed-paths-report: 候補 6 件のうち一覧に当たらない 0 件(足すかどうかは人が決める)"

# 一覧が無くても落とさない
run "$TMP/none"
has "ai-eval-needed-paths-report: 一覧が無い: $TMP/none"

echo "ok"
