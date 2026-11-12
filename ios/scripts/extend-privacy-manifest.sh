#!/bin/sh
# 通常の版(Debug / Staging / Release)だけ、バンドルのプライバシーマニフェストに「通常の版だけで集める種類」を足す。
# 元の Metamon/PrivacyInfo.xcprivacy は App Store に出す審査用の版(APP_STORE_REVIEW)の申告のまま変えない。
# 足す種類は docs/store/privacy-label.md の 4 章と 1 対 1(片方を変えたらもう片方も変える)。
# Metamon・MetamonMac ターゲットの postBuildScripts。署名の前に走る。Review 構成では何もしない。
set -eu
[ "${METAMON_APP_STORE_REVIEW:-NO}" = "YES" ] && exit 0

MANIFEST="${TARGET_BUILD_DIR}/${UNLOCALIZED_RESOURCES_FOLDER_PATH}/PrivacyInfo.xcprivacy"
[ -f "$MANIFEST" ] || { echo "warning: extend-privacy-manifest: ${MANIFEST} が無い"; exit 0; }

# 購入履歴(App 内課金の取引をサーバーで確かめて保存・v1/me/entitlements/apple)
# (製品の操作は、サーバーの利用の集計が全部の版にあるので元のファイルで宣言済み。aiButton の集計もそこに入る)
# カスタマーサポート(アプリ内のフィードバック・v1/feedback)
# デバイス ID(プッシュの端末の登録・v1/me/devices)
# 足す種類はターゲットの METAMON_PRIVACY_EXTRA_TYPES で変えられる(Mac も #982 からプッシュを登録するので既定のまま)
for type in ${METAMON_PRIVACY_EXTRA_TYPES:-PurchaseHistory CustomerSupport DeviceID}; do
  key="NSPrivacyCollectedDataType${type}"
  # 差分ビルドでリソースがコピーし直されない時に二重に足さない
  if plutil -extract NSPrivacyCollectedDataTypes xml1 -o - "$MANIFEST" | grep -q "<string>${key}</string>"; then
    continue
  fi
  plutil -insert NSPrivacyCollectedDataTypes -json \
    "{\"NSPrivacyCollectedDataType\":\"${key}\",\"NSPrivacyCollectedDataTypeLinked\":true,\"NSPrivacyCollectedDataTypeTracking\":false,\"NSPrivacyCollectedDataTypePurposes\":[\"NSPrivacyCollectedDataTypePurposeAppFunctionality\"]}" \
    -append "$MANIFEST"
done
plutil -lint "$MANIFEST" >/dev/null
echo "extend-privacy-manifest: added normal-build data types to ${MANIFEST}"
