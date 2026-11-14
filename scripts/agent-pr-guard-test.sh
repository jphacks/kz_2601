#!/usr/bin/env bash
# scripts/agent-pr-guard.sh を使い捨ての git リポジトリで確かめる(ネットワーク不要)。
# 本物の一覧(agent-pr-guard.paths)を使い、一覧の各行が今のリポジトリに実在するかも確かめる。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
GUARD="$HERE/agent-pr-guard.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }
expect_code() { # 期待コード 説明 args...
  local want="$1" what="$2" code; shift 2
  set +e; OUT="$(cd "$TMP/repo" && "$GUARD" "$@" 2>&1)"; code=$?; set -e
  [ "$code" = "$want" ] || fail "$what: 終了コード $want のはずが $code"
}

g() { git -C "$TMP/repo" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false "$@"; }
put() { mkdir -p "$TMP/repo/$(dirname "$1")"; printf '%s\n' "${2:-x}" >"$TMP/repo/$1"; }
# 新しいブランチ <名前> を base から切り、渡したファイルを足してコミットする
branch() {
  local name="$1"; shift
  g checkout -q -b "$name" base
  for f in "$@"; do put "$f"; done
  g add -A && g commit -q -m "$name"
}

git init -q "$TMP/repo"
put README.md
put backend/internal/ai/prompt.go
put backend/internal/store/store.go
put docs/legal/terms.md
g add -A && g commit -q -m base && g branch base

# 1. 触れていない
branch clean backend/internal/store/store_test.go docs/backend/features/x.md ios/Metamon/Features/Home/Home.swift
expect_code 0 "触れていない" base clean
has "変更 3 件"
has "OK:"

# 2. 種類ごとに触れる(1 ファイル 1 件)
branch touch \
  .github/workflows/x.yml \
  backend/internal/auth/verifier.go \
  backend/internal/coins/coins.go \
  backend/cmd/runner/coins.go \
  backend/internal/store/firestore_account_purge.go \
  docs/00_decisions.md \
  docs/legal/new.md \
  infra/main.tf \
  backend/internal/loop/triage/prompt.go \
  backend/internal/ai/catalog.go \
  backend/.env \
  ios/GoogleService-Info.plist \
  scripts/agent-pr-guard.paths \
  backend/internal/store/store_test.go
expect_code 1 "触れている" base touch
has "変更 14 件"
has "(13 件)"
has "github	.github/workflows/x.yml"
has "auth	backend/internal/auth/verifier.go"
has "billing	backend/internal/coins/coins.go"
has "billing	backend/cmd/runner/coins.go"
has "deletion	backend/internal/store/firestore_account_purge.go"
has "docs	docs/00_decisions.md"
has "docs	docs/legal/new.md"
has "infra	infra/main.tf"
has "loop	backend/internal/loop/triage/prompt.go"
has "prompt	backend/internal/ai/catalog.go"
has "secret	backend/.env"
has "secret	ios/GoogleService-Info.plist"
has "guard	scripts/agent-pr-guard.paths"
hasnt "store_test.go"

# 3. 既存のファイルの変更・削除・名前の変更(前の名前)も見る
g checkout -q -b edit base
put backend/internal/ai/prompt.go changed
g mv docs/legal/terms.md docs/terms.md
mkdir -p "$TMP/repo/backend/internal/coins"
g mv backend/internal/store/store.go backend/internal/coins/moved.go
g commit -q -a -m edit
expect_code 1 "変更・名前の変更" base edit
has "変更 5 件"
has "prompt	backend/internal/ai/prompt.go"
has "docs	docs/legal/terms.md"
has "billing	backend/internal/coins/moved.go"
hasnt "store/store.go"
hasnt "	docs/terms.md"

# 3b. 削除も見る
g checkout -q -b remove base
g rm -q docs/legal/terms.md
g commit -q -m remove
expect_code 1 "削除" base remove
has "docs	docs/legal/terms.md"

# 4. base の後に base に足したものは数えない(base...head)
g checkout -q base
put infra/later.tf && g add -A && g commit -q -m later
expect_code 0 "base 側の変更は数えない" base clean
hasnt "infra"
g checkout -q -B base HEAD~1

# 5. 似た名前は触れない
branch similar backend/internal/authz/x.go docs/legal.md infra.md backend/internal/ai/prompt_test.go envelope.go
expect_code 0 "似た名前" base similar
has "変更 5 件"

# 6. 引数の誤り
expect_code 2 "無いコミット" base no-such-ref
expect_code 2 "不明な引数" --bogus
expect_code 2 "引数が多い" base clean extra
printf 'auth\n' >"$TMP/bad.paths"
set +e; OUT="$(cd "$TMP/repo" && AGENT_PR_GUARD_PATHS="$TMP/bad.paths" "$GUARD" base clean 2>&1)"; code=$?; set -e
[ "$code" = 2 ] || fail "形の違う一覧: 終了コード 2 のはずが $code"
has "一覧の行の形が違う"

# 7. 一覧の各行がこのリポジトリに実在する(秘密の形は実在しないのが正しいので除く)
ROOT="$(cd "$HERE/.." && pwd)"
tracked="$(git -C "$ROOT" ls-files)"
missing=""
while read -r kind pat _; do
  case "$kind" in '' | '#'* | secret) continue ;; esac
  case "$pat" in */) pat="$pat*" ;; esac
  found=""
  while IFS= read -r f; do
    # shellcheck disable=SC2053
    if [[ "$f" == $pat ]]; then found=1; break; fi
  done <<<"$tracked"
  [ -n "$found" ] || missing="$missing $pat"
done <"$HERE/agent-pr-guard.paths"
[ -z "$missing" ] || { OUT=""; fail "一覧にあるが repo に無い:$missing"; }

echo "ok"
