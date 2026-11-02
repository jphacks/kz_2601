#!/usr/bin/env bash
# scripts/ai-cost.sh の集計を固定の入力で確かめる(gcloud 不要・jq だけ)。
# 期待値を直す時: scripts/ai-cost-test.sh --update
set -euo pipefail
cd "$(dirname "$0")"
D=testdata/ai-cost
run() {
  ./ai-cost.sh --input "$D/usage.json" --generate-input "$D/generate.json" --day 2026-11-01 --threshold 2
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
./ai-cost.sh --input "$D/usage.json" --day 2026-11-01 --threshold 100 >/dev/null || { echo "FAIL: 閾値以下で 0 以外" >&2; exit 1; }
echo "ok"
