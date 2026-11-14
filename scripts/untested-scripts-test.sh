#!/usr/bin/env bash
# scripts/untested-scripts.sh を使い捨ての git リポジトリで確かめる(#2861・ネットワーク不要)。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qxF -- "$1" <<<"$OUT" || fail "出力に「$1」の行が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }
run() { # 期待コード args...
  local want="$1" code; shift
  set +e; OUT="$("$HERE/untested-scripts.sh" "$@" 2>&1)"; code=$?; set -e
  [ "$code" = "$want" ] || fail "終了コード $want のはずが $code"
}

git init -q "$TMP/repo"
for f in a/tested.sh a/untested.sh b/tested-test.sh \
  py/mod.py py/mod_test.py py/other.py tests/test_other.py py/hy-phen.py py/test_hy_phen.py \
  py/by-shell.py b/by-shell-test.sh py/lonely.py py/test_alone.py README.md "c/with space.sh"; do
  mkdir -p "$TMP/repo/$(dirname "$f")" && : >"$TMP/repo/$f"
done
: >"$TMP/repo/a/not-tracked.sh"
git -C "$TMP/repo" add -A
git -C "$TMP/repo" rm -q --cached a/not-tracked.sh

# 1. 一覧(テスト自身・追跡外・.sh/.py 以外は数えない)
run 0 "$TMP/repo"
has "untested-scripts: テストの無いスクリプト 3 件 / 全 8 件"
has "  a/untested.sh"
has "  py/lonely.py"
has "  c/with space.sh"
hasnt "a/tested.sh"
hasnt "py/mod.py"
hasnt "py/other.py"
hasnt "hy-phen"
hasnt "by-shell"
hasnt "not-tracked"
hasnt "README"

# 2. 全部にテストがあれば 0 件
git -C "$TMP/repo" rm -q --cached a/untested.sh py/lonely.py "c/with space.sh"
run 0 "$TMP/repo"
has "untested-scripts: テストの無いスクリプト 0 件 / 全 5 件"

# 3. この repo でも落ちずに一覧が出る
run 0
grep -q '^untested-scripts: テストの無いスクリプト [0-9]* 件 / 全 [0-9]* 件$' <<<"$OUT" || fail "この repo の見出しが無い"

# 4. 引数の誤り
run 2 --bogus
run 2 "$TMP/repo" extra
run 2 "$TMP/no-such-dir"

echo "ok"
