#!/usr/bin/env bash
# scripts/realtime-smoke.sh の引数の検査・--dry-run・通しの流れを偽の curl で確かめる(ネットワーク不要)。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SMOKE="$HERE/realtime-smoke.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }
# 期待コード 説明 args...(環境変数は呼ぶ側で渡す)
run() {
  local want="$1" what="$2" code; shift 2
  set +e; OUT="$("$SMOKE" "$@" 2>&1)"; code=$?; set -e
  [ "$code" = "$want" ] || fail "$what: 終了コード $want のはずが $code"
}

# 偽の curl: 引数を CURL_LOG に残す。/events には SSE を返し、それ以外は何もしない。
mkdir -p "$TMP/bin"
cat >"$TMP/bin/curl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$CURL_LOG"
case "$*" in *"@-"*) cat >/dev/null ;; esac
case "$*" in
  *"/events"*) [ "${FAKE_NO_RECORDS:-}" = 1 ] && printf 'event: hello\n' || printf 'event: hello\nevent: records\n' ;;
esac
exit 0
EOF
chmod +x "$TMP/bin/curl"
export CURL_LOG="$TMP/curl.log"
: >"$CURL_LOG"
export PATH="$TMP/bin:$PATH"
unset U1_TOKEN U2_TOKEN

# 1. 引数の誤り
run 2 "不明なオプション" --bogus
has "不明な引数"
run 2 "URL が 2 つ" http://localhost:1 http://localhost:2
run 2 "http で始まらない" ftp://example.invalid
has "http:// か https://"
run 2 "help" --help

# 2. ローカル以外でトークンが無い
run 2 "stg で両方無い" https://stg.example.invalid
has "U1_TOKEN"
U1_TOKEN=a run 2 "stg で U2 が無い" https://stg.example.invalid
has "U2_TOKEN"
hasnt "U1_TOKEN"
U2_TOKEN=b run 2 "stg で U1 が無い" https://stg.example.invalid
has "U1_TOKEN"
[ ! -s "$CURL_LOG" ] || fail "検査で止まる時は curl を呼ばない"

# 3. --dry-run: 通信せず、呼ぶ順だけ。トークンは出さない
U1_TOKEN=secret-one U2_TOKEN=secret-two run 0 "dry-run(stg)" --dry-run https://stg.example.invalid/
has "dry-run: base=https://stg.example.invalid "
has "u1  PUT  https://stg.example.invalid/v1/apps/"
has "event: records"
hasnt "secret-one"; hasnt "secret-two"
run 0 "dry-run(ローカルはトークン不要)" --dry-run
has "base=http://localhost:8080"
run 0 "dry-run(127.0.0.1)" http://127.0.0.1:9 --dry-run
run 0 "dry-run(IPv6 ローカル)" --dry-run 'http://[::1]:8080'
# dry-run でもトークン検査は効く
run 2 "dry-run でも stg はトークン要" --dry-run https://stg.example.invalid
[ ! -s "$CURL_LOG" ] || fail "dry-run は curl を呼ばない"

# 4. 通し(偽の curl): 成功
run 0 "通し" http://localhost:8080
has "OK: app=RT-SMOKE-"
grep -q 'PUT' "$CURL_LOG" || fail "PUT が呼ばれていない"
grep -q 'entities/expense/records' "$CURL_LOG" || fail "投稿が呼ばれていない"
# ローカルの既定トークン
grep -q 'Bearer rt-user-1' "$CURL_LOG" || fail "ローカルの既定の u1 トークンが使われていない"

# 5. 通し: records が来ない
FAKE_NO_RECORDS=1 run 1 "records が来ない" http://localhost:8080
has "records イベントが 10 秒以内に来ない"

echo "realtime-smoke-test: OK"
