#!/usr/bin/env bash
# 止まらないアニメ(repeatForever・TimelineView(.animation))を使うのに、同じファイルで
# accessibilityReduceMotion を見ていない Swift ファイルを一覧に出す(#3084)。
# 読むだけ。落とさない(終了コードは 0。引数の誤りだけ 2)。
# 使い方: scripts/ios-reduce-motion-report.sh [<探す場所>...]   既定は ios/
# 出力: 該当ファイルのパスを 1 行ずつ + 件数。Tests・Previews は数えない。
set -euo pipefail
cd "$(dirname "$0")/.."
[ "$#" -gt 0 ] || set -- ios
for d in "$@"; do [ -e "$d" ] || { echo "ない場所: $d" >&2; exit 2; }; done

n=0
while IFS= read -r f; do
  case "$f" in */Tests/* | *Tests.swift | */.build/* | */DerivedData/*) continue ;; esac
  grep -qE 'repeatForever|TimelineView\(\.animation' "$f" || continue
  grep -q 'accessibilityReduceMotion' "$f" && continue
  echo "$f"
  n=$((n + 1))
done < <(find "$@" -name '*.swift' -type f | sort)
echo "動きを減らすを見ていないファイル: $n 件"
