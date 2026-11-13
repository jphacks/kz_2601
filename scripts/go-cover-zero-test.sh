#!/usr/bin/env bash
# scripts/go-cover-zero.sh を作り物の go tool cover -func の出力で確かめる(#3268)。ネットワーク・エミュレータ不要。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/go-cover-zero.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }

M=github.com/jphacks/kz_2601/backend
mkdir -p "$TMP/b/internal/x" "$TMP/b/internal/y"
echo 'package x' >"$TMP/b/internal/x/plain.go"
printf 'package y\n// FIRESTORE_EMULATOR_HOST を見る\n' >"$TMP/b/internal/y/uses_env.go"
echo 'package y' >"$TMP/b/internal/y/firestore_store.go"
{
  printf '%s:10:\tUncovered\t0.0%%\n' "$M/internal/x/plain.go"
  printf '%s:20:\tCovered\t100.0%%\n' "$M/internal/x/plain.go"
  printf '%s:30:\tPartial\t45.5%%\n' "$M/internal/x/plain.go"
  printf '%s:5:\tEnvFn\t0.0%%\n' "$M/internal/y/uses_env.go"
  printf '%s:7:\tFsFn\t0.0%%\n' "$M/internal/y/firestore_store.go"
  printf 'total:\t(statements)\t50.0%%\n'
} >"$TMP/func.txt"

OUT="$(BACKEND_DIR="$TMP/b" "$SCRIPT" --func "$TMP/func.txt" 2>&1)" || fail "exit 0 のはず"
has "[0% の関数] 1 件"
has $'internal/x/plain.go:10\tUncovered'
hasnt "Covered	"
hasnt "Partial"
hasnt "total"
has "エミュレータが要る物(firestore のファイル・FIRESTORE_EMULATOR_HOST を使うファイル)] 2 件"
has $'internal/y/uses_env.go:5\tEnvFn'
has $'internal/y/firestore_store.go:7\tFsFn'
# 段の分け: plain の行はエミュレータの段より前、エミュレータの行は後
plain_line=$(grep -n 'Uncovered' <<<"$OUT" | cut -d: -f1)
emu_head=$(grep -n 'エミュレータが要る物' <<<"$OUT" | cut -d: -f1)
emu_line=$(grep -n 'EnvFn' <<<"$OUT" | cut -d: -f1)
[ "$plain_line" -lt "$emu_head" ] && [ "$emu_head" -lt "$emu_line" ] || fail "段の順が違う"

# ファイルが無くても落とさない
set +e; OUT="$("$SCRIPT" --func "$TMP/none" 2>&1)"; code=$?; set -e
[ "$code" = 0 ] || fail "ファイルが無くても exit 0 のはずが $code"
has "ファイルが無い"

echo "ok"
