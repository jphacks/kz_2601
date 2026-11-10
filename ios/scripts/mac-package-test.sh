#!/bin/sh
# ローカル Swift Package のテストを macOS で回す(#928・mac-ci / ios-ci の ios-dsl から呼ぶ。手元でも同じ)。
#   ios/scripts/mac-package-test.sh <MetamonDSL|MetamonRenderer|MetamonServices> [xcodebuild の追加引数…]
# `swift test` ではなく `xcodebuild test` を使う: `swift test` は文字列カタログ(.xcstrings)を訳さず、
# 英語の文言のテスト(EnglishCopyTests)が落ちるため。
# 置き場所は環境変数で変えられる(既定は ios/ の下。CI のキャッシュと同じ):
#   MAC_DERIVED_DATA(ios/DerivedData-mac)・MAC_SOURCE_PACKAGES(ios/SourcePackages)・MAC_TEST_RESULTS(ios/TestResults-mac)
set -eu
pkg="${1:?パッケージ名を渡す(MetamonDSL / MetamonRenderer / MetamonServices)}"
shift
ios_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ios_dir/Packages/$pkg"

dd="${MAC_DERIVED_DATA:-$ios_dir/DerivedData-mac}"
spm="${MAC_SOURCE_PACKAGES:-$ios_dir/SourcePackages}"
results="${MAC_TEST_RESULTS:-$ios_dir/TestResults-mac}"

# テストの実行役(xctest)は ~/Desktop・~/Documents・~/Downloads の下を読めない(macOS の保護)。
# そこに repo や DerivedData があると、バンドルや docs/dsl の読み込みで落ちる。CI では起きない。
for p in "$ios_dir" "$dd"; do
  case "$p" in
    "$HOME/Desktop"*|"$HOME/Documents"*|"$HOME/Downloads"*)
      echo "warning: $p は macOS の保護フォルダの下。テストが読み込みで落ちたら、repo と MAC_DERIVED_DATA を保護の外(例: ~/src・/tmp)に置く" >&2 ;;
  esac
done

mkdir -p "$results"
rm -rf "$results/$pkg.xcresult"
exec xcodebuild test \
  -scheme "$pkg" \
  -destination 'platform=macOS' \
  -derivedDataPath "$dd" \
  -clonedSourcePackagesDirPath "$spm" \
  -resultBundlePath "$results/$pkg.xcresult" \
  -skipPackagePluginValidation \
  -skipMacroValidation \
  -quiet \
  CODE_SIGNING_ALLOWED=NO \
  "$@"
