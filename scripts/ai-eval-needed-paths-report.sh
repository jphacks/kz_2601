#!/usr/bin/env bash
# 生成に関わりそうなのに scripts/ai-eval-needed.paths のどの行にも当たらないファイルを一覧に出す(#3265)。
# 足すかどうかは人が決める。出すだけで常に exit 0(落とさない)。読むだけ・API は呼ばない。
# 使い方: scripts/ai-eval-needed-paths-report.sh      (リポジトリのどこからでも。管理下のファイルを見る)
# 候補: backend/ と docs/dsl/ の下で、名前に prompt・schema・rules を含むテスト以外のファイル、
#       backend/internal/ai/・backend/pkg/dsl/・docs/dsl/dsl.schema.json の下すべて。
# 出力: 当たらない物を 1 行 1 ファイル(標準出力)・件数の要約(標準エラー)。
# 一覧は scripts/ai-eval-needed.paths(AI_EVAL_NEEDED_PATHS で差し替え可。テスト用)。
# 一致の規則は ai-eval-needed.sh と同じ(skip を先に見る・末尾 / は下すべて・* は / をまたぐ)。
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
rules="${AI_EVAL_NEEDED_PATHS:-$here/ai-eval-needed.paths}"
if [ ! -f "$rules" ]; then
  echo "ai-eval-needed-paths-report: 一覧が無い: $rules" >&2
  exit 0
fi

pats=() skips=()
while read -r kind pat _rest; do
  case "$kind" in '' | '#'*) continue ;; esac
  [ -n "${pat:-}" ] || continue
  case "$pat" in */) pat="$pat*" ;; esac
  if [ "$kind" = skip ]; then skips+=("$pat"); else pats+=("$pat"); fi
done <"$rules"

covered() { # 当たれば 0
  local f="$1" p
  for p in ${skips[@]+"${skips[@]}"}; do
    # shellcheck disable=SC2053 # パターンとして一致させる
    [[ "$f" == $p ]] && return 1
  done
  for p in ${pats[@]+"${pats[@]}"}; do
    # shellcheck disable=SC2053
    [[ "$f" == $p ]] && return 0
  done
  return 1
}

candidate() {
  local f="$1"
  case "$f" in
    *_test.go) return 1 ;;
    backend/internal/ai/* | backend/pkg/dsl/* | docs/dsl/dsl.schema.json) return 0 ;;
    backend/* | docs/dsl/*)
      case "$(basename "$f")" in *prompt* | *schema* | *rules*) return 0 ;; esac
      ;;
  esac
  return 1
}

total=0 missed=0
while IFS= read -r -d '' f; do
  candidate "$f" || continue
  total=$((total + 1))
  if ! covered "$f"; then
    missed=$((missed + 1))
    printf '%s\n' "$f"
  fi
done < <(git ls-files -z -- backend docs/dsl)

echo "ai-eval-needed-paths-report: 候補 $total 件のうち一覧に当たらない $missed 件(足すかどうかは人が決める)" >&2
exit 0
