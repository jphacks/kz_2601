#!/usr/bin/env bash
# scripts/runner-usage.sh の集計を固定の入力で確かめる(gcloud 不要・jq だけ)。
# 期待値を直す時: scripts/runner-usage-test.sh --update
set -euo pipefail
cd "$(dirname "$0")"
D=testdata/runner-usage
run() {
  ./runner-usage.sh --input "$D/usage.json" --day 2026-11-01 --threshold 2
}
set +e
out="$(run)"
code=$?
set -e
[ "$code" = 3 ] || { echo "FAIL: 閾値超えで終了コード 3 のはずが $code" >&2; exit 1; }
if [ "${1:-}" = "--update" ]; then
  printf '%s\n' "$out" >"$D/expected.md"
  echo "updated $D/expected.md"
  exit 0
fi
diff -u "$D/expected.md" <(printf '%s\n' "$out") || { echo "FAIL: 期待値と違う" >&2; exit 1; }
./runner-usage.sh --input "$D/usage.json" --day 2026-11-01 --threshold 3 >/dev/null || { echo "FAIL: 閾値ちょうどで 0 以外" >&2; exit 1; }
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
echo '[]' >"$TMP/empty.json"
./runner-usage.sh --input "$TMP/empty.json" --day 2026-11-01 >/dev/null || { echo "FAIL: 空のログで 0 以外" >&2; exit 1; }
./runner-usage.sh prod --dry-run | grep -q 'jsonPayload.event="runner.usage"' || { echo "FAIL: dry-run のフィルタ" >&2; exit 1; }
if ./runner-usage.sh prod --threshold x >/dev/null 2>&1; then echo "FAIL: 数でない閾値で 0" >&2; exit 1; fi
echo "ok"
