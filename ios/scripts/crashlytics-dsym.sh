#!/bin/sh
# クラッシュ報告(#296): dSYM を Firebase Crashlytics に送る(Metamon ターゲットの postBuildScripts)。docs/ios/crash-reporting.md
# 送るのは実機向けの Release / Review だけ。Debug・シミュレータ・plist 無し(CI)では何もしない(ローカルのビルドを遅くしない・
# 手元の確認ビルドで本番の Firebase に書き込まない)。
set -eu
case "${CONFIGURATION:-}" in
  Release|Review) ;;
  *) echo "crashlytics-dsym: skip (${CONFIGURATION:-?})"; exit 0 ;;
esac
if [ "${PLATFORM_NAME:-}" = "iphonesimulator" ]; then
  echo "crashlytics-dsym: skip (simulator)"; exit 0
fi
if [ ! -f "${TARGET_BUILD_DIR}/${UNLOCALIZED_RESOURCES_FOLDER_PATH}/GoogleService-Info.plist" ]; then
  echo "warning: crashlytics-dsym: GoogleService-Info.plist が無いので dSYM を送りません(ios/scripts/sync-firebase-plist.sh)"
  exit 0
fi
# SPM のチェックアウト先(DerivedData/<name>/SourcePackages または -derivedDataPath 直下)
RUN="${BUILD_DIR%/Build/*}/SourcePackages/checkouts/firebase-ios-sdk/Crashlytics/run"
if [ ! -x "$RUN" ]; then
  echo "warning: crashlytics-dsym: $RUN が見つかりません"; exit 0
fi
exec "$RUN"
