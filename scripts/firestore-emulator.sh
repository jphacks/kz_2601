#!/usr/bin/env bash
# Firestore エミュレータ(gcloud SDK の cloud-firestore-emulator)の起動と試験(#546)。
# CI(.github/workflows/backend-emulator.yml)と make -C backend emulator / test-emulator が共用する。
#   start      前面で起動(Ctrl-C で止まる)
#   wait       起動済みのエミュレータが応答するまで待つ(最大 90 秒)
#   test       起動済みのエミュレータに対してエミュレータ試験を回す
#   test-all   起動していなければ一時的に起動 → test → 自分で起動した分だけ止める
#   summary F  go test -json の出力 F から、パッケージごとの回った数・Skip の数を表で出す(落とさない・#3267)
# 接続先は FIRESTORE_EMULATOR_HOST(既定 127.0.0.1:8089)。プロジェクトは demo-kz2601 固定
# (demo- プロジェクトは実 GCP に繋がらない。テスト側もハードコード)。必要なもの: gcloud
# (cloud-firestore-emulator コンポーネント)・Java 11+・Go・jq・curl。
set -euo pipefail

export FIRESTORE_EMULATOR_HOST="${FIRESTORE_EMULATOR_HOST:-127.0.0.1:8089}"
export GOOGLE_CLOUD_PROJECT=demo-kz2601
BACKEND="$(cd "$(dirname "$0")/../backend" && pwd)"

up() { curl -sf "http://$FIRESTORE_EMULATOR_HOST/" >/dev/null; }

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

cmd_wait() {
  for _ in $(seq 1 90); do
    if up; then echo "emulator ready"; return 0; fi
    sleep 1
  done
  echo "::error::Firestore emulator did not become ready in 90s" >&2
  return 1
}

# go test -json の出力から、エミュレータ試験を持つパッケージごとの pass / skip の数と回ったかを表で出す。
# 読むだけ。Skip があっても失敗にはしない(失敗の判定は cmd_test)。回った = Skip 0 かつ pass 1 以上。
cmd_summary() {
  local f="${1:-}"
  [ -f "$f" ] || { echo "usage: $0 summary <go-test-json-file>" >&2; return 2; }
  local rows
  rows=$(jq -rs '
    [ .[] | select(.Test != null and .Package != null and (.Action=="pass" or .Action=="skip")) ]
    | group_by(.Package)
    | map({pkg: .[0].Package,
           pass: (map(select(.Action=="pass")) | length),
           skip: (map(select(.Action=="skip")) | length)})
    | sort_by(.pkg)[]
    | [.pkg, .pass, .skip, (if .skip > 0 then "SKIP" elif .pass > 0 then "ran" else "none" end)] | @tsv' "$f")
  local ran=0 skipped=0 total=0 pkg pass skip st
  echo "== emulator test packages =="
  printf '%-60s %5s %5s  %s\n' PACKAGE PASS SKIP STATUS
  while IFS=$'\t' read -r pkg pass skip st; do
    [ -n "$pkg" ] || continue
    printf '%-60s %5s %5s  %s\n' "${pkg#*/backend/}" "$pass" "$skip" "$st"
    total=$((total + 1))
    case "$st" in ran) ran=$((ran + 1)) ;; SKIP) skipped=$((skipped + 1)) ;; esac
  done <<<"$rows"
  echo "packages: $total / ran: $ran / skipped: $skipped"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    echo "Emulator test packages: $total, ran: $ran, skipped: $skipped" >> "$GITHUB_STEP_SUMMARY"
  fi
  return 0
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
  cmd_summary "$out" || true

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
  trap 'kill -TERM '"$pid"' 2>/dev/null || true; wait '"$pid"' 2>/dev/null || true' EXIT
  cmd_wait || { cat "$log"; return 1; }
  cmd_test
}

case "${1:-}" in
  start) cmd_start ;;
  wait) cmd_wait ;;
  test) cmd_test ;;
  test-all) cmd_test_all ;;
  summary) shift; cmd_summary "$@" ;;
  *) echo "usage: $0 start|wait|test|test-all|summary <json>" >&2; exit 2 ;;
esac
