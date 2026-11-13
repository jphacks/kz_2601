#!/usr/bin/env bash
# scripts/desktop-strings-dup-report.py を作り物の XML で確かめる(ネットワーク不要)。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/desktop-strings-dup-report.py"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }

mkdir -p "$TMP/android/renderer/res/values" "$TMP/android/renderer/res/values-en"
cat >"$TMP/android/renderer/res/values/strings_a.xml" <<'XML'
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="common_close">閉じる</string>
    <string name="common_add">追加</string>
    <string name="common_quote">それは\'だめ\'です</string>
</resources>
XML
cat >"$TMP/android/renderer/res/values-en/strings_a.xml" <<'XML'
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="en_only">英語だけの値</string>
</resources>
XML
cat >"$TMP/desktop.xml" <<'XML'
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="desktop_close">閉じる</string>
    <string name="desktop_quote">それは'だめ'です</string>
    <string name="desktop_own">デスクトップだけ</string>
    <string name="desktop_en_only">英語だけの値</string>
    <string name="plain_close">追加</string>
</resources>
XML

set +e; OUT="$(python3 -I "$SCRIPT" --desktop "$TMP/desktop.xml" --android "$TMP/android" 2>&1)"; code=$?; set -e
[ "$code" = 0 ] || fail "終了コード 0 のはずが $code"
has "desktop_close	common_close	閉じる"
has "desktop_quote	common_quote"
hasnt "desktop_own"
hasnt "desktop_en_only"
hasnt "plain_close"
has "同じ文言の desktop_ キー: 2 件"

set +e; OUT="$(python3 -I "$SCRIPT" --desktop "$TMP/none.xml" --android "$TMP/android" 2>&1)"; code=$?; set -e
[ "$code" = 2 ] || fail "無い XML で終了コード 2 のはずが $code"
echo "ok"
