#!/usr/bin/env bash
# ios/scripts/crashlytics-dsym.sh の送る・送らないと送り先の決め方を、Xcode の環境変数を偽って確かめる(#2859)。
# Xcode と同じく select-firebase-plist.sh で plist をバンドルに入れてから --dry-run で回す。
# Crashlytics の run は偽物(呼ばれたら記録するだけ)で、Firebase には何も送らない。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPTS="$ROOT/ios/scripts"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# 偽の SRCROOT(ios/)の Firebase の plist。PROJECT_ID で prod / stg を見分ける
mkdir -p "$TMP/ios/Firebase"
plist() { # ファイル PROJECT_ID
  cat >"$1" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>GOOGLE_APP_ID</key>
	<string>1:000:ios:test</string>
	<key>PROJECT_ID</key>
	<string>$2</string>
</dict>
</plist>
EOF
}
plist "$TMP/ios/Firebase/GoogleService-Info.plist" metamon-prod-test
plist "$TMP/ios/Firebase/GoogleService-Info-Staging.plist" metamon-stg-test

# 偽の DerivedData。BUILD_DIR から SourcePackages の run を探す
DD="$TMP/DerivedData/Metamon-abc"
RUN="$DD/SourcePackages/checkouts/firebase-ios-sdk/Crashlytics/run"
mkdir -p "$(dirname "$RUN")"
cat >"$RUN" <<'EOF'
#!/bin/sh
echo "run called: ${CONFIGURATION} $*" >>"$RUN_LOG"
EOF
chmod +x "$RUN"
export RUN_LOG="$TMP/run.log"

# 1 回分のビルド(構成 METAMON_FIREBASE_ENV PLATFORM_NAME 引数...)。plist を選んでから dSYM のスクリプトを回す
build() {
  local config="$1" env="$2" platform="$3"; shift 3
  local products="$DD/Build/Products/$config-$platform"
  rm -rf "$products"
  mkdir -p "$products/Metamon.app"
  : >"$RUN_LOG"
  set +e
  OUT="$(
    export CONFIGURATION="$config" METAMON_FIREBASE_ENV="$env" PLATFORM_NAME="$platform" \
      SRCROOT="$TMP/ios" BUILD_DIR="$DD/Build/Products" TARGET_BUILD_DIR="$products" \
      UNLOCALIZED_RESOURCES_FOLDER_PATH=Metamon.app \
      DWARF_DSYM_FOLDER_PATH="$products" DWARF_DSYM_FILE_NAME=Metamon.app.dSYM
    if [ -z "${NO_SELECT:-}" ]; then sh "$SCRIPTS/select-firebase-plist.sh" >/dev/null || exit 9; fi
    sh "$SCRIPTS/crashlytics-dsym.sh" "$@" 2>&1
  )"
  CODE=$?
  set -e
  PRODUCTS="$products"
}
fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $1" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
code() { [ "$CODE" = "$1" ] || fail "$2: 終了コード $1 のはずが $CODE"; }
not_run() { [ ! -s "$RUN_LOG" ] || fail "$1: run が呼ばれた"; }

# 1. prod(Release・実機): prod の plist で送る予定を出す。run は呼ばない
build Release prod iphoneos --dry-run
code 0 "Release"
has "crashlytics-dsym: dry-run (Release・prod)"
has "  plist: $PRODUCTS/Metamon.app/GoogleService-Info.plist"
has "  project: metamon-prod-test"
has "  dsym: $PRODUCTS/Metamon.app.dSYM"
has "  run: $RUN"
not_run "Release の dry-run"

# 2. 審査用(Review・実機): Release と同じく prod
build Review prod iphoneos --dry-run
code 0 "Review"
has "crashlytics-dsym: dry-run (Review・prod)"
has "  project: metamon-prod-test"
not_run "Review の dry-run"

# 3. stg(Staging / Debug): 送らない
for config in Staging Debug; do
  build "$config" stg iphoneos --dry-run
  code 0 "$config"
  has "crashlytics-dsym: skip ($config)"
  not_run "$config"
done

# 4. 送り先はバンドルの plist で決まる(Release でも stg の plist が入っていれば stg に送る)
build Release stg iphoneos --dry-run
code 0 "Release + stg の plist"
has "  project: metamon-stg-test"

# 5. シミュレータ・plist 無し・run 無し は送らない
build Release prod iphonesimulator --dry-run
code 0 "simulator"
has "crashlytics-dsym: skip (simulator)"
NO_SELECT=1 build Release prod iphoneos --dry-run
code 0 "plist 無し"
has "GoogleService-Info.plist が無いので dSYM を送りません"
mv "$RUN" "$RUN.off"
build Release prod iphoneos --dry-run
code 0 "run 無し"
has "warning: crashlytics-dsym: $RUN が見つかりません"
mv "$RUN.off" "$RUN"

# 6. 引数なし(Xcode のビルドフェーズ)は今まで通り run を呼ぶ。Debug は呼ばない
build Release prod iphoneos
code 0 "引数なし Release"
[ "$(cat "$RUN_LOG")" = "run called: Release " ] || fail "引数なし Release: run が呼ばれていない($(cat "$RUN_LOG"))"
build Debug stg iphoneos
code 0 "引数なし Debug"
not_run "引数なし Debug"

# 7. 知らない引数は 2 で止まる(送らない)
build Release prod iphoneos --dryrun
code 2 "知らない引数"
has "使い方: crashlytics-dsym.sh [--dry-run]"
not_run "知らない引数"

echo "OK: crashlytics-dsym-test"
