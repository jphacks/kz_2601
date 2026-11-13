#!/usr/bin/env bash
# scripts/agent-pr-guard-coverage.sh を使い捨ての git リポジトリで確かめる(ネットワーク不要)。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
COV="$HERE/agent-pr-guard-coverage.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }
run() { # 期待コード 説明 args...
  local want="$1" what="$2" code; shift 2
  set +e; OUT="$("$COV" "$@" 2>&1)"; code=$?; set -e
  [ "$code" = "$want" ] || fail "$what: 終了コード $want のはずが $code"
}
put() { mkdir -p "$TMP/repo/$(dirname "$1")"; printf 'x\n' >"$TMP/repo/$1"; }

git init -q "$TMP/repo"
# 一覧に当たる物
put backend/internal/auth/auth.go
put ios/Metamon/Features/Coins/CoinView.swift
# 一覧に当たらない物(名前は該当)
put desktop/app/auth/Login.kt
put desktop/app/auth/Token.kt
put ios/MetamonMac/App/Purchase/Paywall.swift
put backend/internal/ai/coins_test.go
put android/FirebaseAuthService.kt
# 名前が該当しない物・語が違う物(author は auth でない)
put backend/internal/author/author.go
put backend/internal/store/store.go
put README.md
# 見ない場所
put docs/auth/design.md
put notes/purchase.md
git -C "$TMP/repo" add -A

cat >"$TMP/paths" <<'EOF'
# テスト用の一覧
auth backend/internal/auth/
billing ios/Metamon/Features/Coins/
EOF
export AGENT_PR_GUARD_PATHS="$TMP/paths"

# 1. 漏れが出る(表示だけ・終了コード 0)
run 0 "漏れが有っても 0" "$TMP/repo"
has "追跡 12 件"
has "desktop/app/auth/"
has "2 件"
has "ios/MetamonMac/App/Purchase/"
has "backend/internal/ai/coins_test.go"
has "android/FirebaseAuthService.kt"
# 一覧に当たる物・語が違う物・見ない場所は出さない
hasnt "backend/internal/auth/"
hasnt "Features/Coins"
hasnt "author"
hasnt "store.go"
hasnt "docs/auth"
hasnt "notes/purchase.md"

# 2. 一覧に足すと漏れが消える
cat >>"$TMP/paths" <<'EOF'
auth desktop/app/auth/
auth android/FirebaseAuthService.kt
billing ios/MetamonMac/App/Purchase/
billing backend/internal/ai/coins_test.go
EOF
run 0 "全部当たる" "$TMP/repo"
has "OK:"

# 3. 探す名前・見ない場所を差し替えられる
AGENT_PR_GUARD_WORDS="store" run 0 "名前の差し替え" "$TMP/repo"
has "backend/internal/store/"
AGENT_PR_GUARD_COVERAGE_EXCLUDE="^nothing-matches$" AGENT_PR_GUARD_WORDS="purchase" run 0 "除外の差し替え" "$TMP/repo"
has "notes/purchase.md"

# 4. 誤り
run 2 "git のリポジトリでない" "$TMP"
run 2 "不明な引数" --bogus
AGENT_PR_GUARD_PATHS="$TMP/none" run 2 "一覧が無い" "$TMP/repo"
printf 'auth a b\n' >"$TMP/badpaths"
AGENT_PR_GUARD_PATHS="$TMP/badpaths" run 2 "一覧の行の形" "$TMP/repo"

# 5. 本物の一覧でも回る(中身は固定しない。形だけ)
unset AGENT_PR_GUARD_PATHS
run 0 "本物の一覧" "$HERE/.."
has "agent-pr-guard-coverage:"

echo "agent-pr-guard-coverage-test: OK"
