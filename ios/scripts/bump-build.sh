#!/bin/sh
# iOS の版とビルド番号を ios/project.yml(唯一の置き場所)で上げる。docs/release/process.md
#   ios/scripts/bump-build.sh                  # ビルド番号(CURRENT_PROJECT_VERSION)を +1
#   ios/scripts/bump-build.sh --build 42       # ビルド番号を 42 にする(今より大きい数だけ)
#   ios/scripts/bump-build.sh --version 1.0.0  # 版(MARKETING_VERSION)を変え、ビルド番号も +1
# ファイルを書き換えるだけ。コミット・タグ・push はしない。
set -eu
cd "$(dirname "$0")/.."
yml=project.yml

current_build=$(sed -n 's/^ *CURRENT_PROJECT_VERSION: "\([0-9]*\)".*/\1/p' "$yml")
current_version=$(sed -n 's/^ *MARKETING_VERSION: "\([^"]*\)".*/\1/p' "$yml")
[ -n "$current_build" ] && [ -n "$current_version" ] || { echo "error: $yml に版 / ビルド番号が見つからない" >&2; exit 1; }

new_build=$((current_build + 1))
new_version=$current_version
while [ $# -gt 0 ]; do
  case "$1" in
    --build) new_build="${2:?--build に数を渡す}"; shift 2 ;;
    --version) new_version="${2:?--version に X.Y.Z を渡す}"; shift 2 ;;
    -h|--help) sed -n 2,6p "$0"; exit 0 ;;
    *) echo "usage: $0 [--build N] [--version X.Y.Z]" >&2; exit 2 ;;
  esac
done

case "$new_build" in ''|*[!0-9]*) echo "error: ビルド番号は正の整数: $new_build" >&2; exit 2 ;; esac
[ "$new_build" -gt "$current_build" ] || { echo "error: ビルド番号は今($current_build)より大きくする" >&2; exit 2; }
echo "$new_version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "error: 版は X.Y.Z: $new_version" >&2; exit 2; }

tmp=$(mktemp)
sed -e "s/^\( *MARKETING_VERSION: \)\"[^\"]*\"/\1\"$new_version\"/" \
    -e "s/^\( *CURRENT_PROJECT_VERSION: \)\"[0-9]*\"/\1\"$new_build\"/" "$yml" > "$tmp"
cat "$tmp" > "$yml"
rm -f "$tmp"
echo "MARKETING_VERSION $current_version -> $new_version / CURRENT_PROJECT_VERSION $current_build -> $new_build"
