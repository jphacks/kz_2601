#!/bin/sh
# 構成に合う Firebase の plist をアプリのバンドルへ GoogleService-Info.plist として入れる(#292)。
# Metamon ターゲットのビルドフェーズ(project.yml の postBuildScripts「Select Firebase plist」)。署名の前に走る。
#   METAMON_FIREBASE_ENV=prod(Release / Review) : ios/Firebase/GoogleService-Info.plist
#   METAMON_FIREBASE_ENV=stg (Debug / Staging)  : ios/Firebase/GoogleService-Info-Staging.plist。
#                                                無ければ prod の plist に戻して警告(stg の Firebase プロジェクトができるまでの暫定)
# どちらも無ければ何も入れない(認証はモック = 今まで通り)。plist は scripts/sync-firebase-plist.sh が ios/Firebase/ に置く。
set -eu
SRC_DIR="${SRCROOT}/Firebase"
PROD="$SRC_DIR/GoogleService-Info.plist"
STG="$SRC_DIR/GoogleService-Info-Staging.plist"
DST="${TARGET_BUILD_DIR}/${UNLOCALIZED_RESOURCES_FOLDER_PATH}/GoogleService-Info.plist"
env="${METAMON_FIREBASE_ENV:-prod}"

src=""
case "$env" in
  stg)
    if [ -f "$STG" ]; then
      src="$STG"
    elif [ -f "$PROD" ]; then
      src="$PROD"
      echo "warning: ios/Firebase/GoogleService-Info-Staging.plist が無いため、${CONFIGURATION} は prod の Firebase で認証します(docs/ios/architecture.md「ビルド構成と接続先」)"
    fi ;;
  prod)
    if [ -f "$PROD" ]; then src="$PROD"; fi ;;
  *)
    echo "error: METAMON_FIREBASE_ENV は stg か prod です(今: $env)"
    exit 1 ;;
esac

if [ -z "$src" ]; then
  rm -f "$DST"
  echo "select-firebase-plist: ${CONFIGURATION}: plist なし(認証はモック)。ios/scripts/sync-firebase-plist.sh で入ります"
  exit 0
fi
cp "$src" "$DST"
echo "select-firebase-plist: ${CONFIGURATION} ($env) -> $(basename "$src")"
