#!/bin/sh
# クラッシュ報告(#296): dSYM を Firebase Crashlytics に送る(Metamon・MetamonMac(#930)ターゲットの postBuildScripts)。docs/ios/crash-reporting.md
# 送るのは実機向けの Release / Review だけ。Debug・シミュレータ・plist 無し(CI)では何もしない(ローカルのビルドを遅くしない・
# 手元の確認ビルドで本番の Firebase に書き込まない)。
# --dry-run(#2859): 送らずに、送る dSYM・使う plist(送り先の PROJECT_ID)・run の場所を出すだけ。Xcode からは引数なしで呼ぶ。
#   CONFIGURATION=Release METAMON_FIREBASE_ENV=prod PLATFORM_NAME=iphoneos TARGET_BUILD_DIR=… ios/scripts/crashlytics-dsym.sh --dry-run
set -eu
DRY_RUN=0
case "${1:-}" in
  "") ;;
  --dry-run) DRY_RUN=1 ;;
  *) echo "crashlytics-dsym: 使い方: crashlytics-dsym.sh [--dry-run](今: $1)" >&2; exit 2 ;;
esac
case "${CONFIGURATION:-}" in
  Release|Review) ;;
  *) echo "crashlytics-dsym: skip (${CONFIGURATION:-?})"; exit 0 ;;
esac
if [ "${PLATFORM_NAME:-}" = "iphonesimulator" ]; then
  echo "crashlytics-dsym: skip (simulator)"; exit 0
fi
PLIST="${TARGET_BUILD_DIR}/${UNLOCALIZED_RESOURCES_FOLDER_PATH}/GoogleService-Info.plist"
if [ ! -f "$PLIST" ]; then
  echo "warning: crashlytics-dsym: GoogleService-Info.plist が無いので dSYM を送りません(ios/scripts/sync-firebase-plist.sh)"
  exit 0
fi
# SPM のチェックアウト先(DerivedData/<name>/SourcePackages または -derivedDataPath 直下)
RUN="${BUILD_DIR%/Build/*}/SourcePackages/checkouts/firebase-ios-sdk/Crashlytics/run"
if [ ! -x "$RUN" ]; then
  echo "warning: crashlytics-dsym: $RUN が見つかりません"; exit 0
fi
if [ "$DRY_RUN" = 1 ]; then
  # 送り先は バンドルに入った plist(select-firebase-plist.sh が METAMON_FIREBASE_ENV で選んだもの)の PROJECT_ID
  project="$(sed -n '/<key>PROJECT_ID<\/key>/{n;s/.*<string>\(.*\)<\/string>.*/\1/p;}' "$PLIST")"
  echo "crashlytics-dsym: dry-run (${CONFIGURATION}・${METAMON_FIREBASE_ENV:-?})"
  echo "  plist: $PLIST"
  echo "  project: ${project:-?}"
  echo "  dsym: ${DWARF_DSYM_FOLDER_PATH:-?}/${DWARF_DSYM_FILE_NAME:-?}"
  echo "  run: $RUN"
  exit 0
fi
exec "$RUN"
