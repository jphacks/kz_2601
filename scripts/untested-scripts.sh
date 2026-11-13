#!/usr/bin/env bash
# 追跡中のシェル・Python のうち、対応するテストの無い物の一覧を出す(#2861)。一覧を出すだけで、終了コードは 0。
# 使い方: scripts/untested-scripts.sh [<repo の根>]   既定はこのスクリプトのある repo
# 対応するテスト(repo のどこにあってもよい。ファイル名で見る):
#   foo.sh → foo-test.sh
#   foo.py → foo_test.py / test_foo.py(- は _ に読み替えても可)/ foo-test.sh
# テスト自身(*-test.sh・*_test.py・test_*.py)は数えない。
set -euo pipefail

case "${1:-}" in -h | --help) sed -n '2,6p' "$0"; exit 0 ;; -*) echo "error: 不明な引数: $1" >&2; exit 2 ;; esac
[ $# -le 1 ] || { echo "usage: $0 [<repo の根>]" >&2; exit 2; }
root="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
files="$(git -C "$root" -c core.quotePath=false ls-files -- '*.sh' '*.py')" || { echo "error: git ls-files に失敗: $root" >&2; exit 2; }
names="$(printf '%s\n' "$files" | sed 's#.*/##')"

has_name() { grep -qxF -- "$1" <<<"$names"; }

total=0 missing=()
while IFS= read -r f; do
  [ -n "$f" ] || continue
  base="${f##*/}"
  case "$base" in *-test.sh | *_test.py | test_*.py) continue ;; esac
  total=$((total + 1))
  stem="${base%.*}"
  case "$base" in
    *.sh) has_name "$stem-test.sh" && continue ;;
    *.py)
      u="${stem//-/_}"
      for t in "${stem}_test.py" "test_$stem.py" "${u}_test.py" "test_$u.py" "$stem-test.sh"; do
        has_name "$t" && continue 2
      done ;;
  esac
  missing+=("$f")
done <<<"$files"

echo "untested-scripts: テストの無いスクリプト ${#missing[@]} 件 / 全 $total 件"
[ ${#missing[@]} -eq 0 ] || printf '  %s\n' "${missing[@]}"
exit 0
