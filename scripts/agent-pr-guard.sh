#!/usr/bin/env bash
# エージェントの PR の差分が「人の確認が必須」の場所に触れていないか確かめる(#370・#366)。
# 使い方: scripts/agent-pr-guard.sh [<base>] [<head>]   既定は origin/develop と HEAD
#   差分は base と head の分岐点から head まで(PR の差分と同じ。base...head)。名前の変更は前後の両方を見る。
# 終了コード: 0 触れていない / 1 触れている(一覧を出す) / 2 引数や git の誤り
# 場所の一覧は scripts/agent-pr-guard.paths(AGENT_PR_GUARD_PATHS で差し替え可。テスト用)。
set -euo pipefail

usage() { echo "usage: $0 [<base>] [<head>]   (default: origin/develop HEAD)" >&2; exit 2; }
case "${1:-}" in -h | --help) usage ;; -*) echo "error: 不明な引数: $1" >&2; usage ;; esac
[ $# -le 2 ] || usage
base="${1:-origin/develop}"
head="${2:-HEAD}"
rules="${AGENT_PR_GUARD_PATHS:-$(cd "$(dirname "$0")" && pwd)/agent-pr-guard.paths}"
[ -f "$rules" ] || { echo "error: 一覧が無い: $rules" >&2; exit 2; }

for ref in "$base" "$head"; do
  git rev-parse --verify -q "$ref^{commit}" >/dev/null || { echo "error: コミットが見つからない: $ref" >&2; exit 2; }
done

kinds=() pats=()
while read -r kind pat rest; do
  case "$kind" in '' | '#'*) continue ;; esac
  [ -n "$pat" ] && [ -z "${rest:-}" ] || { echo "error: 一覧の行の形が違う: $kind $pat ${rest:-}" >&2; exit 2; }
  case "$pat" in */) pat="$pat*" ;; esac
  kinds+=("$kind") pats+=("$pat")
done <"$rules"

files=()
while IFS= read -r -d '' f; do files+=("$f"); done < <(git diff --name-only --no-renames -z "$base...$head" --)

hits=()
for f in ${files[@]+"${files[@]}"}; do
  for i in "${!pats[@]}"; do
    # shellcheck disable=SC2053 # パターンとして一致させる
    if [[ "$f" == ${pats[$i]} ]]; then
      hits+=("${kinds[$i]}	$f	(${pats[$i]})")
      break
    fi
  done
done

echo "agent-pr-guard: $base...$head 変更 ${#files[@]} 件"
if [ ${#hits[@]} -eq 0 ]; then
  echo "OK: 人の確認が必須の場所に触れていない"
  exit 0
fi
echo "NG: 人の確認が必須の場所に触れている(${#hits[@]} 件)。エージェントの PR にせず、人が引き取る:"
printf '  %s\n' "${hits[@]}"
echo "一覧: ${rules#"$(git rev-parse --show-toplevel)"/}  手順: docs/backend/features/feedback-loop-agent-pr.md"
exit 1
