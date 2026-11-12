#!/usr/bin/env bash
# scripts/firestore-emulator.sh の起動待ち(wait・test-all)を偽の curl・gcloud で確かめる(#2864)。
# エミュレータ・Java・GCP は不要。本物の gcloud は呼ばない。上限は短く(1〜2 秒)して回す。
#   偽の curl: $FAKE_STATE/up_after(N)があれば N 回目の問い合わせから応答する。無ければずっと応答しない
#   偽の gcloud: ログに数行書いてすぐ終わる(起動に失敗したエミュレータ)
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin" "$TMP/state" "$TMP/tmpdir"

cat >"$TMP/bin/curl" <<'EOF'
#!/usr/bin/env bash
n=$(( $(cat "$FAKE_STATE/calls" 2>/dev/null || echo 0) + 1 ))
echo "$n" >"$FAKE_STATE/calls"
[ -f "$FAKE_STATE/up_after" ] && [ "$n" -ge "$(cat "$FAKE_STATE/up_after")" ] && exit 0
exit 7
EOF
cat >"$TMP/bin/gcloud" <<'EOF'
#!/usr/bin/env bash
echo "fake gcloud $*"
echo "Exception in thread main: port in use"
exit 1
EOF
chmod +x "$TMP/bin/curl" "$TMP/bin/gcloud"

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }
run() { # 期待コード 秒の上限 args...(環境変数は呼ぶ側で付ける)
  local want="$1" max="$2" code start; shift 2
  rm -f "$TMP/state/calls"
  start=$SECONDS
  set +e
  OUT="$(PATH="$TMP/bin:$PATH" FAKE_STATE="$TMP/state" TMPDIR="$TMP/tmpdir" \
    FIRESTORE_EMULATOR_HOST=127.0.0.1:65530 "$HERE/firestore-emulator.sh" "$@" 2>&1)"
  code=$?
  set -e
  [ "$code" = "$want" ] || fail "$*: 終了コード $want のはずが $code"
  [ $((SECONDS - start)) -le "$max" ] || fail "$*: ${max} 秒より長くかかった($((SECONDS - start)) 秒)"
}

# 1. 起動済みならすぐ ready
echo 1 >"$TMP/state/up_after"
run 0 10 wait
has "emulator ready"

# 2. 3 回目の問い合わせで応答する(上限の内なら待って ready)
echo 3 >"$TMP/state/up_after"
FIRESTORE_EMULATOR_WAIT_SECONDS=10 run 0 15 wait
has "emulator ready"
[ "$(cat "$TMP/state/calls")" = 3 ] || fail "問い合わせの回数: $(cat "$TMP/state/calls")"

# 3. 上限を超えたら理由とログの最後の 20 行を出して 1
rm -f "$TMP/state/up_after"
seq 1 30 | sed 's/^/log line /' >"$TMP/emulator.log"
FIRESTORE_EMULATOR_WAIT_SECONDS=2 FIRESTORE_EMULATOR_LOG="$TMP/emulator.log" run 1 10 wait
has "::error::Firestore emulator at 127.0.0.1:65530: did not become ready in 2s (FIRESTORE_EMULATOR_WAIT_SECONDS)"
has "--- last 20 lines of $TMP/emulator.log ---"
has "log line 11"
has "log line 30"
hasnt "log line 10"
hasnt "emulator ready"

# 4. ログを渡さない時は案内だけ
FIRESTORE_EMULATOR_WAIT_SECONDS=1 run 1 8 wait
has "did not become ready in 1s"
has "FIRESTORE_EMULATOR_LOG=<file>"

# 5. 上限の値の誤り
for bad in 0 abc -1 1.5; do
  FIRESTORE_EMULATOR_WAIT_SECONDS="$bad" run 2 5 wait
  has "FIRESTORE_EMULATOR_WAIT_SECONDS は 1 以上の整数"
done

# 6. test-all: 起動したエミュレータ(偽の gcloud)が先に終わったら上限を待たずに止まり、その出力を出す。一時ログは残さない
FIRESTORE_EMULATOR_WAIT_SECONDS=30 run 1 15 test-all
has "the emulator process exited before becoming ready"
has "fake gcloud emulators firestore start --host-port=127.0.0.1:65530 --project=demo-kz2601"
has "Exception in thread main: port in use"
[ -z "$(ls -A "$TMP/tmpdir")" ] || fail "一時ログが残った: $(ls -A "$TMP/tmpdir")"

echo "ok"
