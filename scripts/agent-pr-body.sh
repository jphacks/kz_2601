#!/usr/bin/env bash
# エージェントの PR の本文を、決まった形で標準出力に出す(#2289・#370)。GitHub には何も書かない。
# 使い方: scripts/agent-pr-body.sh --issue <N> --summary <file> --tests <file> --guard <file> [--stat <file>]
#   --issue    Issue の番号(Closes #N になる)
#   --summary  変更の要点(エージェントが書く)
#   --tests    試したこと(コマンドと結果)
#   --guard    scripts/agent-pr-guard.sh の出力(終了コードを含めてよい)
#   --stat     差分の統計(git diff --stat の出力。省くと欄ごと出さない)
# 形は docs/backend/features/feedback-loop-agent-pr.md の「PR の形」。
# 本文に URL・メンション・画像の埋め込みを入れない(#366 の検査器 3): 入力に有れば 1 で止まる。
# 終了コード: 0 出力した / 1 入力に入れてはいけない物が有る / 2 引数・ファイルの誤り
set -euo pipefail

usage() { echo "usage: $0 --issue <N> --summary <file> --tests <file> --guard <file> [--stat <file>]" >&2; exit 2; }
die() { echo "error: $*" >&2; exit 2; }

issue="" summary="" tests="" guard="" stat=""
while [ $# -gt 0 ]; do
  case "$1" in
    -h | --help) usage ;;
    --issue | --summary | --tests | --guard | --stat)
      [ $# -ge 2 ] || die "$1 に値が無い"
      case "$1" in
        --issue) issue="$2" ;; --summary) summary="$2" ;; --tests) tests="$2" ;;
        --guard) guard="$2" ;; --stat) stat="$2" ;;
      esac
      shift 2 ;;
    *) die "不明な引数: $1" ;;
  esac
done

[[ "$issue" =~ ^[1-9][0-9]*$ ]] || die "--issue は正の整数: '${issue}'"
[ -n "$summary" ] || die "--summary が必要"
[ -n "$tests" ] || die "--tests が必要"
[ -n "$guard" ] || die "--guard が必要"
for f in "$summary" "$tests" "$guard" "$stat"; do
  [ -z "$f" ] || [ -f "$f" ] || die "ファイルが無い: $f"
done
for f in "$summary" "$tests" "$guard"; do
  [ -s "$f" ] || die "ファイルが空: $f"
done

# 入れてはいけない物(URL・メンション・画像の埋め込み)
for f in "$summary" "$tests" "$guard" "$stat"; do
  [ -n "$f" ] || continue
  if grep -Eq 'https?://|!\[|(^|[^A-Za-z0-9_`])@[A-Za-z0-9]' "$f"; then
    echo "NG: $f に URL・メンション・画像の埋め込みが有る。外してから回し直す(#366)" >&2
    exit 1
  fi
done

echo "Closes #$issue"
echo
echo "## 変更の要点"
echo
cat "$summary"
echo
echo "## 試したこと"
echo
cat "$tests"
if [ -n "$stat" ]; then
  echo
  echo "## 差分の統計"
  echo
  echo '```'
  cat "$stat"
  echo '```'
fi
echo
echo "## 対象外の領域に触れていないこと"
echo
# shellcheck disable=SC2016 # バッククォートはそのまま出す
echo '`scripts/agent-pr-guard.sh` の結果:'
echo
echo '```'
cat "$guard"
echo '```'
