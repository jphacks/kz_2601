#!/bin/bash
# 設定の「アイコン」画面のプレビュー(AppIconPreview*.imageset・180px)を、各 AppIcon*.appiconset の 1024px から作り直す(#232)。
# アプリアイコンの絵を差し替えたら実行する。docs/ios/alternate-icons.md
# 引数で Assets.xcassets の場所を変えられる(テスト用・#2858)。省略時はアプリの Assets.xcassets
set -euo pipefail
A="$(cd "${1:-$(dirname "$0")/../Metamon/Resources/Assets.xcassets}" && pwd)"
for set in "$A"/AppIcon*.appiconset; do
  name="$(basename "$set" .appiconset)"     # AppIcon / AppIcon-Dark …
  suffix="${name#AppIcon}"                  # "" / -Dark …
  src="$set/$name-1024.png"
  [[ -f "$src" ]] || { echo "missing: $src" >&2; exit 1; }
  out="$A/AppIconPreview$suffix.imageset"
  mkdir -p "$out"
  sips -z 180 180 "$src" --out "$out/AppIconPreview$suffix.png" >/dev/null
  cat > "$out/Contents.json" <<EOF
{
  "images" : [
    {
      "filename" : "AppIconPreview$suffix.png",
      "idiom" : "universal"
    }
  ],
  "info" : {
    "author" : "xcode",
    "version" : 1
  }
}
EOF
  echo "updated: $(basename "$out")"
done
