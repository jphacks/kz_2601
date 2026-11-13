#!/usr/bin/env bash
# PR の差分が生成(AI の指示文・出力のスキーマ・部品の規則)に関わるかを判定する(#371)。
# ai-eval を CI で回すかどうかと、回すケースの数の上限(費用の上限)を出す。読むだけ・API は呼ばない。
# 使い方: scripts/ai-eval-needed.sh [--max-cases N] [<base>] [<head>]   既定は origin/develop と HEAD
#   差分は base...head(PR の差分と同じ)。名前の変更は前後の両方を見る。
# 出力(標準出力・GITHUB_OUTPUT にそのまま足せる形):
#   needed=true|false
#   max_cases=<N>      needed=false の時は 0
#   matched=<件数>
#   その後に一致したファイルを「<種類>\t<パス>」で 1 行ずつ(標準エラー)
# 終了コード: 0 判定できた / 2 引数や git の誤り
# 場所の一覧は scripts/ai-eval-needed.paths(AI_EVAL_NEEDED_PATHS で差し替え可。テスト用)。
set -euo pipefail

usage() { echo "usage: $0 [--max-cases N] [<base>] [<head>]   (default: --max-cases 20 origin/develop HEAD)" >&2; exit 2; }
max_cases="${AI_EVAL_MAX_CASES:-20}"
args=()
while [ $# -gt 0 ]; do
  case "$1" in
    -h | --help) usage ;;
    --max-cases) [ $# -ge 2 ] || usage; max_cases="$2"; shift 2 ;;
    --max-cases=*) max_cases="${1#*=}"; shift ;;
    -*) echo "error: 不明な引数: $1" >&2; usage ;;
    *) args+=("$1"); shift ;;
  esac
done
[ ${#args[@]} -le 2 ] || usage
case "$max_cases" in '' | *[!0-9]*) echo "error: --max-cases は 1 以上の整数: $max_cases" >&2; exit 2 ;; esac
[ "$max_cases" -ge 1 ] || { echo "error: --max-cases は 1 以上の整数: $max_cases" >&2; exit 2; }
base="${args[0]:-origin/develop}"
head="${args[1]:-HEAD}"
rules="${AI_EVAL_NEEDED_PATHS:-$(cd "$(dirname "$0")" && pwd)/ai-eval-needed.paths}"
[ -f "$rules" ] || { echo "error: 一覧が無い: $rules" >&2; exit 2; }

for ref in "$base" "$head"; do
  git rev-parse --verify -q "$ref^{commit}" >/dev/null || { echo "error: コミットが見つからない: $ref" >&2; exit 2; }
done

kinds=() pats=() skips=()
while read -r kind pat rest; do
  case "$kind" in '' | '#'*) continue ;; esac
  [ -n "$pat" ] && [ -z "${rest:-}" ] || { echo "error: 一覧の行の形が違う: $kind $pat ${rest:-}" >&2; exit 2; }
  case "$pat" in */) pat="$pat*" ;; esac
  if [ "$kind" = skip ]; then
    skips+=("$pat")
  else
    kinds+=("$kind") pats+=("$pat")
  fi
done <"$rules"

files=()
while IFS= read -r -d '' f; do files+=("$f"); done < <(git diff --name-only --no-renames -z "$base...$head" --)

hits=()
for f in ${files[@]+"${files[@]}"}; do
  for s in ${skips[@]+"${skips[@]}"}; do
    # shellcheck disable=SC2053 # パターンとして一致させる
    [[ "$f" == $s ]] && continue 2
  done
  for i in ${pats[@]+"${!pats[@]}"}; do
    # shellcheck disable=SC2053
    if [[ "$f" == ${pats[$i]} ]]; then
      hits+=("${kinds[$i]}	$f")
      break
    fi
  done
done

if [ ${#hits[@]} -eq 0 ]; then
  printf 'needed=false\nmax_cases=0\nmatched=0\n'
  echo "ai-eval-needed: $base...$head 変更 ${#files[@]} 件。生成に関わる差分なし" >&2
  exit 0
fi
printf 'needed=true\nmax_cases=%s\nmatched=%s\n' "$max_cases" "${#hits[@]}"
echo "ai-eval-needed: $base...$head 変更 ${#files[@]} 件のうち生成に関わる ${#hits[@]} 件:" >&2
printf '  %s\n' "${hits[@]}" >&2
