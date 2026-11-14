#!/usr/bin/env bash
# scripts/xcstrings-format-check.py を作り物の .xcstrings で確かめる(Xcode 不要)。合う・数が違う・種類が違うを分ける。
# 書式指定(%1$@ など)をそのまま書くので、単一引用符の中の $ の警告は出さない
# shellcheck disable=SC2016
set -euo pipefail
cd "$(dirname "$0")"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# 合う物だけ
cat >"$TMP/ok.xcstrings" <<'EOF'
{"sourceLanguage":"ja","version":"1.0","strings":{
  "%@ とつないでください": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"Connect %@"}}}},
  "%@: %lld": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"%1$@: %2$lld"}}}},
  "%1$@ が %2$lld 件": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"%2$lld items by %1$@"}}}},
  "%.1f 点": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"%f points"}}}},
  "100%% 完了": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"100%% done"}}}},
  "引数なし": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"No args"}}}},
  "英語なし %@": {},
  "訳さない %@": {"shouldTranslate":false,"localizations":{"en":{"stringUnit":{"state":"translated","value":"x"}}}},
  "%@ ・ %lld 人が利用": {"localizations":{"en":{
    "stringUnit":{"state":"translated","value":"%#@count@"},
    "substitutions":{"count":{"argNum":2,"formatSpecifier":"lld","variations":{"plural":{
      "one":{"stringUnit":{"state":"translated","value":"%1$@ · %arg user"}},
      "other":{"stringUnit":{"state":"translated","value":"%1$@ · %arg users"}}}}}}}}},
  "%lld 日": {"localizations":{"en":{"variations":{"plural":{
    "one":{"stringUnit":{"state":"translated","value":"%lld day"}},
    "other":{"stringUnit":{"state":"translated","value":"%lld days"}}}}}}}
}}
EOF

# 崩れている物
cat >"$TMP/bad.xcstrings" <<'EOF'
{"sourceLanguage":"ja","version":"1.0","strings":{
  "%@ と %@": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"Only %@"}}}},
  "%lld 件": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"%@ items"}}}},
  "%1$@ の %2$lld": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"%2$@ of %1$lld"}}}},
  "%@ を足す": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"Add"}}}},
  "%lld 個": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"%d pieces"}}}},
  "%@ ・ %lld 人": {"localizations":{"en":{
    "stringUnit":{"state":"translated","value":"%#@count@"},
    "substitutions":{"count":{"argNum":2,"formatSpecifier":"@","variations":{"plural":{
      "other":{"stringUnit":{"state":"translated","value":"%1$@ · %arg users"}}}}}}}}},
  "合う": {"localizations":{"en":{"stringUnit":{"state":"translated","value":"Fine"}}}}
}}
EOF

fail() { printf '%s\n' "$OUT" >&2; echo "FAIL: $1" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }

set +e
OUT="$(python3 ./xcstrings-format-check.py "$TMP/ok.xcstrings" 2>&1)"
code=$?
set -e
[ "$code" = 0 ] || fail "合う物だけなら 0 のはずが $code"
has "ok: 1 ファイルの書式指定が合っている"

set +e
OUT="$(python3 ./xcstrings-format-check.py "$TMP/ok.xcstrings" "$TMP/bad.xcstrings" 2>&1)"
code=$?
set -e
[ "$code" = 1 ] || fail "崩れていれば 1 のはずが $code"
has 'bad.xcstrings: 数が違う: "%@ と %@" キー=1:@ 2:@ en=1:@'
has 'bad.xcstrings: 種類が違う: "%lld 件" キー=1:lld en=1:@'
has 'bad.xcstrings: 種類が違う: "%1$@ の %2$lld" キー=1:@ 2:lld en=1:lld 2:@'
has 'bad.xcstrings: 数が違う: "%@ を足す" キー=1:@ en=(なし)'
has 'bad.xcstrings: 種類が違う: "%lld 個" キー=1:lld en=1:d'
has 'bad.xcstrings: 種類が違う: "%@ ・ %lld 人" キー=1:@ 2:lld en=1:@ 2:@'
has "書式指定の合わないキー: 6 件"
grep -q "ok.xcstrings" <<<"$OUT" && fail "合う方のファイルは出さない"
grep -qF '"合う"' <<<"$OUT" && fail "合うキーは出さない"

# 今のカタログで通る
OUT="$(python3 ./xcstrings-format-check.py 2>&1)" || fail "今のカタログで落ちた"

echo "ok: xcstrings-format-check"
