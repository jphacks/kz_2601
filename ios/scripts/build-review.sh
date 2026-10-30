#!/bin/sh
# App Store 審査用の版(Review 構成・APP_STORE_REVIEW・#245)をビルド / アーカイブする。docs/ios/review-build.md
#   ios/scripts/build-review.sh build   [xcodebuild の追加引数…]  # 署名なしでシミュレータ向けにビルド
#   ios/scripts/build-review.sh archive [xcodebuild の追加引数…]  # 実機向けアーカイブ(要 DEVELOPMENT_TEAM)
# ローカル Swift Package(MetamonDSL / Renderer / Services)は project の build setting を継承しないので、
# コンパイル条件はコマンドラインで全ターゲットに渡す(渡し忘れるとアプリ側の ReviewBuildPackageMarker でコンパイルが落ちる)。
set -eu
cd "$(dirname "$0")/.."
action="${1:-build}"
[ $# -gt 0 ] && shift
xcodegen generate --quiet
case "$action" in
  build)
    set -- -destination 'generic/platform=iOS Simulator' CODE_SIGNING_ALLOWED=NO "$@" build ;;
  archive)
    set -- -destination 'generic/platform=iOS' -archivePath "${ARCHIVE_PATH:-build/TailorIt-Review.xcarchive}" "$@" archive ;;
  *) echo "usage: $0 build|archive [xcodebuild args...]" >&2; exit 2 ;;
esac
set -- -project Metamon.xcodeproj -scheme Metamon-Review -configuration Review \
  'SWIFT_ACTIVE_COMPILATION_CONDITIONS=$(inherited) APP_STORE_REVIEW' "$@"
# 並行ビルドで取り合わないよう、ロックのディレクトリがある機械では lockf で直列にする(#452 G9)。無ければそのまま。
lock="${XCODEBUILD_LOCK:-/private/tmp/claude-501/-Users-kota/44acee01-4956-468b-91b0-b11b2b64aee3/scratchpad/xcodebuild.lock}"
if [ -d "$(dirname "$lock")" ] && [ -x /usr/bin/lockf ]; then
  exec nice -n 15 /usr/bin/lockf -k "$lock" xcodebuild "$@"
fi
exec xcodebuild "$@"
