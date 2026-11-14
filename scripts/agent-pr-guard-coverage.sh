#!/usr/bin/env bash
# agent-pr-guard.paths の範囲に漏れが無いかを一覧に出す(#2541・#370)。表示だけ。落とさない・一覧は変えない(足すかは人が決める)。
# 認可・課金などの名前(auth・purchase・coins・billing・entitlement など)の付いたフォルダ・ファイルを
# git で追跡している物の中から探し、一覧のどのパターンにも当たらない物を出す。
# 使い方: scripts/agent-pr-guard-coverage.sh [<リポジトリのパス>]   既定は今のリポジトリ
#   AGENT_PR_GUARD_PATHS     一覧の差し替え(テスト用。既定は scripts/agent-pr-guard.paths)
#   AGENT_PR_GUARD_WORDS     探す名前(空白区切り・小文字)。既定は下の WORDS
#   AGENT_PR_GUARD_COVERAGE_EXCLUDE  見ない場所の正規表現(既定は docs/ と *.md)
# 名前は、パスの各部分を英数字の切れ目と camelCase の切れ目で分けた語と、完全に一致した時に当たる(author は auth に当たらない)。
# 該当の最初の部分までを候補にして(ios/.../Auth/ のように)、候補ごとに一覧に当たらないファイルの数を出す。
# 終了コード: 0 出した(漏れが有っても 0) / 2 引数・git・一覧の誤り
set -euo pipefail

WORDS_DEFAULT="appstore auth authentication authorization billing budget coin coins entitlement entitlements login paid paywall payment payments purchase purchases ratelimit storekit subscription subscriptions"

usage() { echo "usage: $0 [<repo-dir>]" >&2; exit 2; }
case "${1:-}" in -h | --help) usage ;; -*) echo "error: 不明な引数: $1" >&2; usage ;; esac
[ $# -le 1 ] || usage
repo="${1:-.}"
rules="${AGENT_PR_GUARD_PATHS:-$(cd "$(dirname "$0")" && pwd)/agent-pr-guard.paths}"
words="${AGENT_PR_GUARD_WORDS:-$WORDS_DEFAULT}"
exclude="${AGENT_PR_GUARD_COVERAGE_EXCLUDE:-^docs/|\\.md\$}"
[ -f "$rules" ] || { echo "error: 一覧が無い: $rules" >&2; exit 2; }
git -C "$repo" rev-parse --show-toplevel >/dev/null 2>&1 || { echo "error: git のリポジトリでない: $repo" >&2; exit 2; }

# 一覧のパターン(agent-pr-guard.sh と同じ読み方: 末尾 / は下すべて・* は / をまたぐ)を正規表現にして awk に渡す
patfile="$(mktemp)"
trap 'rm -f "$patfile"' EXIT
while read -r kind pat rest; do
  case "$kind" in '' | '#'*) continue ;; esac
  [ -n "$pat" ] && [ -z "${rest:-}" ] || { echo "error: 一覧の行の形が違う: $kind $pat ${rest:-}" >&2; exit 2; }
  case "$pat" in */) pat="$pat*" ;; esac
  printf '%s\n' "$pat" >>"$patfile"
done <"$rules"

git -C "$repo" -c core.quotePath=false ls-files | awk -v words="$words" -v patfile="$patfile" -v exclude="$exclude" '
function toregex(g,   i, c, r) {
  r = "^"
  for (i = 1; i <= length(g); i++) {
    c = substr(g, i, 1)
    if (c == "*") r = r ".*"
    else if (index("\\.[](){}+?^$|", c)) r = r "\\" c
    else r = r c
  }
  return r "$"
}
# 部分を語に分ける(英数字以外・camelCase の切れ目)。語は小文字にして返す
function tokens(s, out,   i, c, p, cur, n) {
  n = 0; cur = ""; p = ""
  for (i = 1; i <= length(s); i++) {
    c = substr(s, i, 1)
    if (c !~ /[A-Za-z0-9]/) { if (cur != "") out[++n] = tolower(cur); cur = ""; p = ""; continue }
    if (c ~ /[A-Z]/ && p ~ /[a-z0-9]/ && cur != "") { out[++n] = tolower(cur); cur = "" }
    cur = cur c; p = c
  }
  if (cur != "") out[++n] = tolower(cur)
  return n
}
BEGIN {
  while ((getline line < patfile) > 0) if (line != "") rx[++nr] = toregex(line)
  close(patfile)
  nw = split(words, wl, " ")
  for (i = 1; i <= nw; i++) isword[wl[i]] = 1
}
{
  f = $0
  if (f == "") next
  total++
  if (exclude != "" && f ~ exclude) next
  nparts = split(f, parts, "/")
  cand = ""
  for (k = 1; k <= nparts; k++) {
    name = parts[k]
    if (k == nparts) sub(/\.[^.]*$/, "", name)
    nt = tokens(name, t)
    hit = 0
    for (j = 1; j <= nt; j++) if (t[j] in isword) hit = 1
    if (hit) {
      cand = parts[1]
      for (m = 2; m <= k; m++) cand = cand "/" parts[m]
      if (k < nparts) cand = cand "/"
      break
    }
  }
  if (cand == "") next
  matched++
  covered = 0
  for (i = 1; i <= nr; i++) if (f ~ rx[i]) { covered = 1; break }
  if (covered) next
  if (!(cand in cnt)) order[++no] = cand
  cnt[cand]++
  if (!(cand in ex)) ex[cand] = f
  gaps++
}
END {
  printf "agent-pr-guard-coverage: 追跡 %d 件 / 名前が該当 %d 件 / 一覧に当たらない %d 件(候補 %d 個)\n", total, matched, gaps, no
  if (no == 0) { print "OK: 名前の該当はすべて一覧に当たる"; exit }
  print "一覧に当たらない候補(表示だけ。足すかは人が決める。一覧: scripts/agent-pr-guard.paths):"
  # 名前順に出す
  for (i = 1; i <= no; i++) for (j = i + 1; j <= no; j++) if (order[j] < order[i]) { tmp = order[i]; order[i] = order[j]; order[j] = tmp }
  for (i = 1; i <= no; i++) {
    if (substr(order[i], length(order[i])) == "/") printf "  %s\t%d 件\t例: %s\n", order[i], cnt[order[i]], ex[order[i]]
    else printf "  %s\n", order[i]
  }
}'
