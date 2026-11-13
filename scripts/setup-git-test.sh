#!/usr/bin/env bash
# scripts/setup-git.sh を使い捨ての git リポジトリで 2 回回し、設定が同じ(冪等)ことを確かめる(ネットワーク不要)。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SETUP="$HERE/setup-git.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
# 手元の git の設定を持ち込まない
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null

fail() { echo "FAIL: $*" >&2; exit 1; }
cfg() { git -C "$TMP/repo" config --local --list; }
setup() { (cd "$TMP/repo" && "$SETUP" >/dev/null); }

git init -q "$TMP/repo"
before="$(cfg)"

# 1. 1 回目で、入れる設定が入る
setup
first="$(cfg)"
[ "$(git -C "$TMP/repo" config --get merge.xcstrings.driver)" = "python3 ios/scripts/merge-xcstrings.py %O %A %B" ] || fail "merge.xcstrings.driver が違う"
[ "$(git -C "$TMP/repo" config --get merge.xcstrings.name)" = "xcstrings key-level 3-way merge" ] || fail "merge.xcstrings.name が違う"
[ "$(git -C "$TMP/repo" config --get rerere.enabled)" = "false" ] || fail "rerere.enabled が false でない"
[ "$first" != "$before" ] || fail "1 回目で設定が増えていない"

# 2. 2 回目で、設定の一覧が 1 文字も変わらない(重ならない)
setup
second="$(cfg)"
[ "$first" = "$second" ] || fail "2 回目で設定が変わった:
$(diff <(printf '%s\n' "$first") <(printf '%s\n' "$second") || true)"
for k in merge.xcstrings.name merge.xcstrings.driver rerere.enabled; do
  n="$(git -C "$TMP/repo" config --get-all "$k" | wc -l | tr -d ' ')"
  [ "$n" = 1 ] || fail "$k が $n 個ある"
done

# 3. rerere が有効になっていても、回すと無効に戻る
git -C "$TMP/repo" config rerere.enabled true
setup
[ "$(git -C "$TMP/repo" config --get rerere.enabled)" = "false" ] || fail "rerere を無効に戻していない"
setup
[ "$(cfg)" = "$first" ] || fail "rerere を戻した後の 2 回目で設定が変わった"

# 4. 入れる設定以外には触れない
git -C "$TMP/repo" config user.name keep-me
setup
[ "$(git -C "$TMP/repo" config --get user.name)" = "keep-me" ] || fail "無関係な設定が消えた"

# 5. サブディレクトリから回しても、リポジトリの根に入る
mkdir -p "$TMP/repo/sub/dir"
(cd "$TMP/repo/sub/dir" && "$SETUP" >/dev/null)
[ "$(git -C "$TMP/repo" config --get rerere.enabled)" = "false" ] || fail "サブディレクトリから回して壊れた"

echo "setup-git-test: OK"
