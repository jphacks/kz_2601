#!/usr/bin/env bash
# プロダクトの指標(#297)を週ごとの表で出す。読み取りだけ(gcloud logging read)。
# 定義と見方は docs/metrics/product-metrics.md。集計は backend/cmd/product-metrics。
#
# 使い方:
#   scripts/product-metrics-weekly.sh prod               # 直近 28 日(Cloud Logging の既定の保存は 30 日)
#   scripts/product-metrics-weekly.sh stg --days 63
#   scripts/product-metrics-weekly.sh prod --dry-run     # 実行せずにフィルタだけ出す
#   scripts/product-metrics-weekly.sh --input logs.json  # 手元の書き出し(gcloud の JSON か JSON lines)を集計
#
# 要る権限: GCP の roles/logging.viewer(読み取りだけ)。`gcloud auth login` 済みであること。
# 要るコマンド: gcloud(--input の時は不要)・go
# 環境変数: PROJECT_ID(既定 jphacks-kz2601)・LIMIT(最大行数、既定 200000)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_NAME=""
DAYS=28
DRY=0
INPUT=""
while [ $# -gt 0 ]; do
  case "$1" in
    stg|prod) ENV_NAME="$1"; shift ;;
    --days) DAYS="${2:?--days N}"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    --input) INPUT="${2:?--input FILE}"; shift 2 ;;
    *) echo "usage: $0 <stg|prod> [--days N] [--dry-run] | --input FILE" >&2; exit 2 ;;
  esac
done
case "$DAYS" in ''|*[!0-9]*) echo "--days must be a number" >&2; exit 2 ;; esac
# ログの保存は既定 30 日・延ばす推奨でも 90 日(docs/metrics/product-metrics.md 8 章 #3)。それより長い窓は読めないので止める
MAX_DAYS=90
if [ "$((10#$DAYS))" -lt 1 ] || [ "$((10#$DAYS))" -gt "$MAX_DAYS" ]; then
  echo "--days must be 1..$MAX_DAYS" >&2
  exit 2
fi

run() { (cd "$ROOT/backend" && go run ./cmd/product-metrics -in "$1"); }

if [ -n "$INPUT" ]; then
  case "$INPUT" in /*) ;; *) INPUT="$PWD/$INPUT" ;; esac
  run "$INPUT"
  exit 0
fi
[ -n "$ENV_NAME" ] || { echo "usage: $0 <stg|prod> [--days N] [--dry-run] | --input FILE" >&2; exit 2; }

PROJECT_ID="${PROJECT_ID:-jphacks-kz2601}"
LIMIT="${LIMIT:-200000}"
SINCE="$(date -u -v-"${DAYS}"d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d "-${DAYS} days" +%Y-%m-%dT%H:%M:%SZ)"
FILTER="resource.type=\"cloud_run_revision\" AND resource.labels.service_name=(\"ai-${ENV_NAME}\" OR \"appconfig-${ENV_NAME}\") AND timestamp>=\"${SINCE}\" AND jsonPayload.event=(\"ai.generate\" OR \"session.start\" OR \"app.opened\" OR \"app.shared\" OR \"app.published\" OR \"share.imported\")"

if [ "$DRY" = 1 ]; then
  printf 'project=%s since=%s\n\n%s\n' "$PROJECT_ID" "$SINCE" "$FILTER"
  exit 0
fi
command -v gcloud >/dev/null || { echo "gcloud が必要です" >&2; exit 2; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
gcloud logging read "$FILTER" --project="$PROJECT_ID" --limit="$LIMIT" --order=desc \
  --format="json(timestamp,jsonPayload.event,jsonPayload.uid_hash,jsonPayload.app_hash,jsonPayload.mode,jsonPayload.outcome,jsonPayload.validation_passed)" \
  >"$TMP/logs.json"
n="$(grep -c '"timestamp"' "$TMP/logs.json" || true)"
if [ "${n:-0}" -ge "$LIMIT" ]; then
  echo "warning: $n 行で LIMIT に達した。LIMIT を上げてやり直す" >&2
fi
printf '<!-- %s(ai-%s / appconfig-%s)・%s 日・%s 行 -->\n' "$PROJECT_ID" "$ENV_NAME" "$ENV_NAME" "$DAYS" "${n:-0}"
run "$TMP/logs.json"
