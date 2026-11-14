#!/usr/bin/env bash
# scripts/agent-pr-body.sh を確かめる(ネットワーク・git 不要)。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
BODY="$HERE/agent-pr-body.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }
run() { # 期待コード 説明 args...
  local want="$1" what="$2" code; shift 2
  set +e; OUT="$("$BODY" "$@" 2>&1)"; code=$?; set -e
  [ "$code" = "$want" ] || fail "$what: 終了コード $want のはずが $code"
}

printf '%s\n' '- 検査の文言を直した' >"$TMP/summary"
# shellcheck disable=SC2016
printf '%s\n' '- `scripts/foo-test.sh` -> OK' >"$TMP/tests"
printf '%s\n' 'agent-pr-guard: origin/develop...HEAD 変更 2 件' 'OK: 触れていない' >"$TMP/guard"
printf '%s\n' ' scripts/foo.sh | 3 ++-' ' 1 file changed' >"$TMP/stat"
: >"$TMP/empty"

# 1. 全部有る
run 0 "全部有る" --issue 12 --summary "$TMP/summary" --tests "$TMP/tests" --guard "$TMP/guard" --stat "$TMP/stat"
first="$(head -n1 <<<"$OUT")"
[ "$first" = "Closes #12" ] || fail "先頭が Closes #12 でない: $first"
has "## 変更の要点"; has "検査の文言を直した"
has "## 試したこと"; has "foo-test.sh"
has "## 差分の統計"; has "1 file changed"
has "## 対象外の領域に触れていないこと"; has "OK: 触れていない"

# 2. --stat 省略は欄ごと出さない
run 0 "stat 省略" --issue 12 --summary "$TMP/summary" --tests "$TMP/tests" --guard "$TMP/guard"
hasnt "## 差分の統計"

# 3. 引数の誤り
run 2 "issue 無し" --summary "$TMP/summary" --tests "$TMP/tests" --guard "$TMP/guard"
run 2 "issue が数字でない" --issue abc --summary "$TMP/summary" --tests "$TMP/tests" --guard "$TMP/guard"
run 2 "issue が 0" --issue 0 --summary "$TMP/summary" --tests "$TMP/tests" --guard "$TMP/guard"
run 2 "tests 無し" --issue 1 --summary "$TMP/summary" --guard "$TMP/guard"
run 2 "ファイルが無い" --issue 1 --summary "$TMP/nope" --tests "$TMP/tests" --guard "$TMP/guard"
run 2 "空のファイル" --issue 1 --summary "$TMP/empty" --tests "$TMP/tests" --guard "$TMP/guard"
run 2 "不明な引数" --issue 1 --bogus x
run 2 "値が無い" --issue

# 4. 入れてはいけない物
printf '%s\n' '詳しくは https://example.invalid を見る' >"$TMP/bad"
run 1 "URL" --issue 1 --summary "$TMP/bad" --tests "$TMP/tests" --guard "$TMP/guard"
has "URL・メンション"
printf '%s\n' 'cc @someone' >"$TMP/bad"
run 1 "メンション" --issue 1 --summary "$TMP/summary" --tests "$TMP/bad" --guard "$TMP/guard"
printf '%s\n' '![x](a.png)' >"$TMP/bad"
run 1 "画像" --issue 1 --summary "$TMP/summary" --tests "$TMP/tests" --guard "$TMP/guard" --stat "$TMP/bad"
# shellcheck disable=SC2016
printf '%s\n' 'mail a@b と `@types/node`' >"$TMP/ok"
run 0 "メールの途中の @・コード内の @ は通す" --issue 1 --summary "$TMP/ok" --tests "$TMP/tests" --guard "$TMP/guard"

echo "agent-pr-body-test: OK"
