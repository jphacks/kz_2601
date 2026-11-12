#!/usr/bin/env bash
# Firestore エミュレータ(gcloud SDK の cloud-firestore-emulator)の起動と試験(#546)。
# CI(.github/workflows/backend-emulator.yml)と make -C backend emulator / test-emulator が共用する。
#   start      前面で起動(Ctrl-C で止まる)
#   wait       起動済みのエミュレータが応答するまで待つ(上限 FIRESTORE_EMULATOR_WAIT_SECONDS 秒・既定 60)。
#              超えたら FIRESTORE_EMULATOR_LOG のログの最後の 20 行を出して終わりのコード 1
#   test       起動済みのエミュレータに対してエミュレータ試験を回す
#   test-all   起動していなければ一時的に起動 → test → 自分で起動した分だけ止める
# 接続先は FIRESTORE_EMULATOR_HOST(既定 127.0.0.1:8089)。プロジェクトは demo-kz2601 固定
# (demo- プロジェクトは実 GCP に繋がらない。テスト側もハードコード)。必要なもの: gcloud
# (cloud-firestore-emulator コンポーネント)・Java 11+・Go・jq・curl。
set -euo pipefail

export FIRESTORE_EMULATOR_HOST="${FIRESTORE_EMULATOR_HOST:-127.0.0.1:8089}"
export GOOGLE_CLOUD_PROJECT=demo-kz2601
BACKEND="$(cd "$(dirname "$0")/../backend" && pwd)"

# 待ちの上限(秒)。超えたらログの最後を出して終わりのコード 1(#2864)
WAIT_SECONDS="${FIRESTORE_EMULATOR_WAIT_SECONDS:-60}"

# 応答の無いポートで止まらないよう 1 回の問い合わせにも上限を付ける
up() { curl -sf --connect-timeout 2 --max-time 3 "http://$FIRESTORE_EMULATOR_HOST/" >/dev/null; }

cmd_start() {
  echo "Firestore emulator: export FIRESTORE_EMULATOR_HOST=$FIRESTORE_EMULATOR_HOST GOOGLE_CLOUD_PROJECT=$GOOGLE_CLOUD_PROJECT"
  # gcloud はエミュレータ(java)を自分の pid のプロセスグループで動かし、止められても java を残す。
  # なのでグループごと止める(Ctrl-C・TERM・終了のどれでも)。
  gcloud emulators firestore start --host-port="$FIRESTORE_EMULATOR_HOST" --project="$GOOGLE_CLOUD_PROJECT" &
  local pid=$!
  trap 'kill -TERM -- -'"$pid"' 2>/dev/null || kill -TERM '"$pid"' 2>/dev/null || true' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  wait "$pid"
}

# cmd_wait [<ログ>] [<起動した pid>]: 応答するまで待つ。ログは FIRESTORE_EMULATOR_LOG でも渡せる。
# pid を渡すと、そのプロセスが先に終わった時は上限を待たずに止まる。
cmd_wait() {
  case "$WAIT_SECONDS" in '' | *[!0-9]* | 0) echo "FIRESTORE_EMULATOR_WAIT_SECONDS は 1 以上の整数: $WAIT_SECONDS" >&2; return 2 ;; esac
  local log="${1:-${FIRESTORE_EMULATOR_LOG:-}}" pid="${2:-}" reason deadline=$((SECONDS + WAIT_SECONDS))
  while :; do
    if up; then echo "emulator ready"; return 0; fi
    if [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; then
      reason="the emulator process exited before becoming ready"; break
    fi
    if [ "$SECONDS" -ge "$deadline" ]; then
      reason="did not become ready in ${WAIT_SECONDS}s (FIRESTORE_EMULATOR_WAIT_SECONDS)"; break
    fi
    sleep 1
  done
  echo "::error::Firestore emulator at $FIRESTORE_EMULATOR_HOST: $reason" >&2
  if [ -n "$log" ] && [ -f "$log" ]; then
    echo "--- last 20 lines of $log ---" >&2
    tail -n 20 "$log" >&2
  else
    echo "(ログは無し。FIRESTORE_EMULATOR_LOG=<file> で最後の行を出す)" >&2
  fi
  return 1
}

cmd_test() {
  up || { echo "::error::no emulator at $FIRESTORE_EMULATOR_HOST" >&2; return 1; }
  local out status=0
  out="${RUNNER_TEMP:-$(mktemp -d)}/emulator-test.json"
  cd "$BACKEND"
  # t.Skip("FIRESTORE_EMULATOR_HOST not set") を持つパッケージ = エミュレータ試験のあるパッケージ
  local pkgs
  pkgs=$(grep -rl --include='*_test.go' 'FIRESTORE_EMULATOR_HOST not set' . | xargs -n1 dirname | sort -u)
  if [ -z "$pkgs" ]; then echo "::error::no emulator test packages found"; return 1; fi
  printf "packages:\n%s\n" "$pkgs"
  # shellcheck disable=SC2086
  go test -count=1 -json $pkgs > "$out" || status=$?
  jq -rj 'select(.Action=="output") | .Output' "$out"

  # 黙ってスキップさせない: スキップ理由にエミュレータが出たら、または
  # Firestore 系テスト(TestFirestore* / */firestore)が 1 件でもスキップされたら失敗
  if grep -q 'FIRESTORE_EMULATOR_HOST not set' "$out"; then
    echo "::error::emulator tests were skipped"; return 1
  fi
  local fs='select(.Test != null and (.Test | test("^TestFirestore|/firestore$")))'
  local skipped passed
  skipped=$(jq -r "$fs | select(.Action==\"skip\") | .Test" "$out")
  if [ -n "$skipped" ]; then echo "::error::skipped Firestore tests:"; echo "$skipped"; return 1; fi
  passed=$(jq -r "$fs | select(.Action==\"pass\") | .Test" "$out" | wc -l | tr -d " ")
  echo "Firestore emulator tests passed: $passed"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then echo "Firestore emulator tests passed: $passed" >> "$GITHUB_STEP_SUMMARY"; fi
  if [ "$status" -ne 0 ]; then return "$status"; fi
  if [ "$passed" -eq 0 ]; then echo "::error::no Firestore emulator test ran"; return 1; fi
}

cmd_test_all() {
  if up; then cmd_test; return; fi
  local log pid
  log="$(mktemp -t firestore-emulator.XXXXXX)"
  "$0" start >"$log" 2>&1 &
  pid=$!
  trap 'kill -TERM '"$pid"' 2>/dev/null || true; wait '"$pid"' 2>/dev/null || true; rm -f -- '"'$log'" EXIT
  cmd_wait "$log" "$pid" || return 1
  cmd_test
}

case "${1:-}" in
  start) cmd_start ;;
  wait) cmd_wait ;;
  test) cmd_test ;;
  test-all) cmd_test_all ;;
  *) echo "usage: $0 start|wait|test|test-all" >&2; exit 2 ;;
esac
