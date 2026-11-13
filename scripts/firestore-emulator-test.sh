#!/usr/bin/env bash
# scripts/firestore-emulator.sh の summary(パッケージごとの回った・Skip の数)を作り物の go test -json で確かめる(#3267)。
# エミュレータ・ネットワーク不要。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/firestore-emulator.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }

M=github.com/jphacks/kz_2601/backend/internal
ev() { printf '{"Action":"%s","Package":"%s/%s","Test":"%s"}\n' "$1" "$M" "$2" "$3"; }
{
  ev pass store TestFirestoreA
  ev pass store TestFirestoreB
  ev pass calls TestFirestoreCalls
  ev skip coins TestFirestoreCoins
  ev pass coins TestOther
  ev skip loop TestFirestoreLoop
  ev skip loop TestFirestoreLoop2
  # パッケージ単位の行(Test なし)は数えない
  printf '{"Action":"pass","Package":"%s/store"}\n' "$M"
  printf '{"Action":"output","Package":"%s/store","Output":"ok\\n"}\n' "$M"
} >"$TMP/fake.json"

OUT="$("$SCRIPT" summary "$TMP/fake.json" 2>&1)" || fail "summary は exit 0 のはず(Skip があっても落とさない)"
# 列: パッケージ pass skip 状態
grep -Eq '^internal/store +2 +0 +ran$' <<<"$OUT" || fail "store が ran・pass 2 でない"
grep -Eq '^internal/calls +1 +0 +ran$' <<<"$OUT" || fail "calls が ran でない"
grep -Eq '^internal/coins +1 +1 +SKIP$' <<<"$OUT" || fail "coins が SKIP・pass 1・skip 1 でない"
grep -Eq '^internal/loop +0 +2 +SKIP$' <<<"$OUT" || fail "loop が SKIP・skip 2 でない"
has "packages: 4 / ran: 2 / skipped: 2"

# 引数なし・存在しないファイルは使い方を出して 2
set +e; OUT="$("$SCRIPT" summary "$TMP/none.json" 2>&1)"; code=$?; set -e
[ "$code" = 2 ] || fail "ファイルが無い時は exit 2 のはずが $code"
has "usage:"

echo "ok"
