#!/bin/sh
# 審査用ビルド(Review 構成・APP_STORE_REVIEW・#245)で、外した機能の権限の説明などを Info.plist から消す。
# Metamon ターゲットの最後のビルドフェーズ(project.yml の postBuildScripts)。署名の前に走る。通常の構成では何もしない。
set -eu
[ "${METAMON_APP_STORE_REVIEW:-NO}" = "YES" ] || exit 0

PLIST="${TARGET_BUILD_DIR}/${INFOPLIST_PATH}"
PB=/usr/libexec/PlistBuddy

# HealthKit / 位置の常時・背面 / 近くの端末(ローカルネットワーク・Bonjour)/ モーション(healthStat の歩数計・sensorValue の気圧計)/ 共有リンクの URL スキーム
for key in NSHealthShareUsageDescription NSHealthUpdateUsageDescription NSHealthClinicalHealthRecordsShareUsageDescription \
           NSLocationAlwaysUsageDescription NSLocationAlwaysAndWhenInUseUsageDescription \
           NSLocalNetworkUsageDescription NSBonjourServices NSMotionUsageDescription CFBundleURLTypes; do
  "$PB" -c "Delete :$key" "$PLIST" 2>/dev/null || true
done

# UIBackgroundModes から location だけ外す(fetch = 裏の定期同期は残す)
i=0
while v=$("$PB" -c "Print :UIBackgroundModes:$i" "$PLIST" 2>/dev/null); do
  if [ "$v" = "location" ]; then "$PB" -c "Delete :UIBackgroundModes:$i" "$PLIST"; else i=$((i + 1)); fi
done

# 残す権限の説明は、外した機能に触れない文に差し替える
"$PB" -c "Set :NSCameraUsageDescription 生成したアプリで QR コードを読み取るために、カメラを使用します。" "$PLIST"
"$PB" -c "Set :NSMicrophoneUsageDescription 話しかけた内容からアプリを生成するために、マイクを使用します。" "$PLIST"
"$PB" -c "Set :NSLocationWhenInUseUsageDescription 生成したアプリで、ボタンを押した時の現在地を記録・地図に表示したり、案内を開始した時に目的地まで道案内したりするために使います。" "$PLIST"
"$PB" -c "Set :NSPhotoLibraryAddUsageDescription 生成したアプリの共有ボタンから画像を写真に保存するために、写真ライブラリへの追加を使用します。" "$PLIST"

# 外した部品を使う同梱の例(docs/dsl/examples)もバンドルから外す(DSLExamples.reviewExcluded と一致させる)
for name in attendance_nearby walk_route health dashcam sensors; do
  rm -f "${TARGET_BUILD_DIR}/${UNLOCALIZED_RESOURCES_FOLDER_PATH}/${name}.json"
done
echo "strip-review-plist: removed review-excluded keys from ${PLIST}"
