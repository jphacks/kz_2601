#!/usr/bin/env bash
# scripts/demo-check.sh の版・env 照合を偽の curl で確かめる(ネットワーク・GCP・API キー不要)。
# 偽の curl は -K の url のホスト名(appconfig.test / ai.test)で $FAKE_HEALTH_DIR/<ホスト>.json を返す。
set -euo pipefail
cd "$(dirname "$0")"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin" "$TMP/health"

cat >"$TMP/bin/curl" <<'EOF'
#!/usr/bin/env bash
# demo-check.sh の req() が渡す形(-K <rc> -o <out> -w ...)だけ扱う
rc="" out=""
while [ $# -gt 0 ]; do
  case "$1" in -K) rc="$2"; shift 2 ;; -o) out="$2"; shift 2 ;; *) shift ;; esac
done
url="$(sed -n 's/^url = "\(.*\)"$/\1/p' "$rc")"
host="${url#*://}"; host="${host%%/*}"; host="${host%.test}"
f="$FAKE_HEALTH_DIR/$host.json"
if [ -f "$f" ]; then cp "$f" "$out"; printf '200 0.012'; else printf '{}' >"$out"; printf '404 0.001'; fi
EOF
chmod +x "$TMP/bin/curl"

health() { printf '{"status":"ok","env":"%s","firestoreDatabase":"%s","version":"%s"}\n' "$2" "$3" "$4" >"$TMP/health/$1.json"; }
run() {
  PATH="$TMP/bin:$PATH" FAKE_HEALTH_DIR="$TMP/health" \
    APPCONFIG_URL=http://appconfig.test AI_URL=http://ai.test FIREBASE_WEB_API_KEY='' \
    ./demo-check.sh "$@"
}
expect_code() { # 期待コード 説明 args...
  local want="$1" what="$2" code; shift 2
  set +e; OUT="$(run "$@" 2>&1)"; code=$?; set -e
  [ "$code" = "$want" ] || { printf '%s\n' "$OUT" >&2; echo "FAIL: $what: 終了コード $want のはずが $code" >&2; exit 1; }
}
has() { grep -qF -- "$1" <<<"$OUT" || { printf '%s\n' "$OUT" >&2; echo "FAIL: 出力に「$1」が無い" >&2; exit 1; }; }

# 1. 版も env も一致
health appconfig stg '(default)' 0123456
health ai stg stg 0123456
expect_code 0 "一致" stg --version-only --expect 0123456
has "OK appconfig 版(/health version) (- ms) version=0123456 期待=0123456"
has "OK ai 版(/health version) (- ms) version=0123456 期待=0123456"
has "OK appconfig env(/health) (- ms) env=stg 期待=stg firestoreDatabase=(default)"
has "OK ai env(/health) (- ms) env=stg 期待=stg firestoreDatabase=stg"
has "OK: すべて成功"

# 2. 40 文字・大文字の sha も先頭 7 文字で比べる
expect_code 0 "長い sha" stg --version-only --expect 0123456789ABCDEF0123456789abcdef01234567

# 3. ai だけ古い版
health ai stg stg fedcba9
expect_code 1 "版違い" stg --version-only --expect 0123456
has "NG ai 版(/health version) (- ms) version=fedcba9 期待=0123456"
has "OK appconfig 版(/health version)"
has "NG: 1 件失敗"

# 4. --expect skip なら版は見ない(env は見る)
expect_code 0 "skip" stg --version-only --expect skip
has "ai 版: --expect skip のため照合しない(version=fedcba9)"

# 5. prod を指定したのに stg が返る(URL の取り違え)
health appconfig stg '(default)' 0123456
health ai prod '(default)' 0123456
expect_code 1 "env 違い" prod --version-only --expect=0123456
has "NG appconfig env(/health) (- ms) env=stg 期待=prod firestoreDatabase=(default)"
has "OK ai env(/health)"

# 6. version の無い /health・届かない /health は NG
printf '{"status":"ok","env":"stg"}' >"$TMP/health/appconfig.json"
rm -f "$TMP/health/ai.json"
expect_code 1 "version 無し" stg --version-only --expect 0123456
has "NG appconfig 版(/health version) (- ms) version=- 期待=0123456"
has "NG ai GET /health ×3"

# 7. 引数の誤り
expect_code 2 "sha でない" stg --version-only --expect main
expect_code 2 "短い sha" stg --version-only --expect abc12
expect_code 2 "--expect の値なし" stg --expect
expect_code 2 "env なし" --version-only --expect skip
expect_code 2 "不明な引数" stg --bogus

# 8. 既定の期待値は origin/develop(手元に有る時だけ。CI の浅い checkout には無い)
if dev="$(git rev-parse --verify -q 'origin/develop^{commit}')"; then
  health appconfig stg '(default)' "${dev:0:7}"
  health ai stg stg "${dev:0:7}"
  expect_code 0 "既定 origin/develop" stg --version-only
  has "expect:    ${dev:0:7} (origin/develop)"
else
  echo "skip: origin/develop が無いので既定の期待値は試さない"
fi
echo "ok"
