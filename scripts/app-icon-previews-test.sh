#!/usr/bin/env bash
# ios/scripts/app-icon-previews.sh の流れを偽の sips で確かめる(#2858・Linux でも回る)。
# 偽の sips は引数を $SIPS_LOG に 1 行ずつ記録し、--out の先に空のファイルを作る。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$ROOT/ios/scripts/app-icon-previews.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"

cat >"$TMP/bin/sips" <<'EOF'
#!/usr/bin/env bash
echo "$*" >>"$SIPS_LOG"
out=""
while [ $# -gt 0 ]; do
  case "$1" in --out) out="$2"; shift 2 ;; *) shift ;; esac
done
[ -n "$out" ] && : >"$out"
EOF
chmod +x "$TMP/bin/sips"
export SIPS_LOG="$TMP/sips.log"

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $1" >&2; exit 1; }
run() { # assets のフォルダ
  : >"$SIPS_LOG"
  set +e
  OUT="$(PATH="$TMP/bin:$PATH" "$SCRIPT" "$1" 2>&1)"
  CODE=$?
  set -e
}
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
appicon() { # assets 名前(AppIcon / AppIcon-Dark) 1024px を置くか(yes/no)
  mkdir -p "$1/$2.appiconset"
  if [ "$3" = yes ]; then printf 'png' >"$1/$2.appiconset/$2-1024.png"; fi
}

# 1. AppIcon と AppIcon-Dark: 2 つのプレビューと Contents.json を作る
A="$TMP/ok/Assets.xcassets"
appicon "$A" AppIcon yes
appicon "$A" AppIcon-Dark yes
run "$A"
[ "$CODE" = 0 ] || fail "2 つ: 終了コード 0 のはずが $CODE"
has "updated: AppIconPreview.imageset"
has "updated: AppIconPreview-Dark.imageset"
want="-z 180 180 $A/AppIcon-Dark.appiconset/AppIcon-Dark-1024.png --out $A/AppIconPreview-Dark.imageset/AppIconPreview-Dark.png
-z 180 180 $A/AppIcon.appiconset/AppIcon-1024.png --out $A/AppIconPreview.imageset/AppIconPreview.png"
got="$(sort "$SIPS_LOG")"
[ "$got" = "$want" ] || { printf 'want:\n%s\ngot:\n%s\n' "$want" "$got" >&2; fail "sips の引数が違う"; }
for suffix in "" -Dark; do
  dir="$A/AppIconPreview$suffix.imageset"
  [ -f "$dir/AppIconPreview$suffix.png" ] || fail "$dir の png が無い"
  grep -qF "\"filename\" : \"AppIconPreview$suffix.png\"" "$dir/Contents.json" || fail "$dir/Contents.json の filename が違う"
  grep -qF '"idiom" : "universal"' "$dir/Contents.json" || fail "$dir/Contents.json の idiom が違う"
done
if command -v python3 >/dev/null; then
  python3 -c 'import json,sys; [json.load(open(p)) for p in sys.argv[1:]]' \
    "$A/AppIconPreview.imageset/Contents.json" "$A/AppIconPreview-Dark.imageset/Contents.json" ||
    fail "Contents.json が JSON として読めない"
fi

# 2. 1024px の画像が無い: 1 で止まる
A="$TMP/missing/Assets.xcassets"
appicon "$A" AppIcon yes
appicon "$A" AppIcon-Tinted no
run "$A"
[ "$CODE" = 1 ] || fail "1024px 無し: 終了コード 1 のはずが $CODE"
has "missing: $A/AppIcon-Tinted.appiconset/AppIcon-Tinted-1024.png"
[ ! -e "$A/AppIconPreview-Tinted.imageset/Contents.json" ] || fail "1024px 無し: プレビューを作っている"

# 3. 引数を省くとスクリプトから見た ../Metamon/Resources/Assets.xcassets を使う(既定は変えていない)。
#    本物の Assets を書き換えないよう、スクリプトを一時フォルダの ios/scripts/ に写して回す
T="$TMP/tree/ios"
mkdir -p "$T/scripts"
cp "$SCRIPT" "$T/scripts/"
A="$T/Metamon/Resources/Assets.xcassets"
appicon "$A" AppIcon yes
: >"$SIPS_LOG"
set +e
OUT="$(PATH="$TMP/bin:$PATH" "$T/scripts/app-icon-previews.sh" 2>&1)"
CODE=$?
set -e
[ "$CODE" = 0 ] || fail "引数なし: 終了コード 0 のはずが $CODE"
[ -f "$A/AppIconPreview.imageset/Contents.json" ] || fail "引数なし: 既定の Assets.xcassets に作っていない"

echo "OK: app-icon-previews-test"
