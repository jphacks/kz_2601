#!/usr/bin/env bash
# β の品質ゲートの週の数字を出す(#322)。読み取りだけ(gcloud logging read / gh issue list)。
# 基準と見方は docs/release/beta-quality-gate.md。出た表を docs/release/beta-weekly/YYYY-MM-DD.md に写す。
#
# 使い方:
#   scripts/beta-weekly.sh prod              # 直近 7 日
#   scripts/beta-weekly.sh stg --days 14
#   scripts/beta-weekly.sh prod --dry-run    # 実行せずにフィルタだけ出す
#
# 要る権限(どれも読み取りだけ。書き込みの権限は要らない):
#   - GCP: プロジェクトの roles/logging.viewer(logging.logEntries.list)。`gcloud auth login` 済みであること
#   - GitHub: jphacks/kz_2601 の Issue を読めること(`gh auth status`)
# 要るコマンド: gcloud・jq・gh(gh が無ければ P1 の行は飛ばす)
#
# 環境変数: PROJECT_ID(既定 jphacks-kz2601)・AI_SERVICE(既定 ai-<env>)・LIMIT(1 クエリの最大行数、既定 20000)
set -euo pipefail

ENV_NAME="${1:-}"
case "$ENV_NAME" in
  stg|prod) shift ;;
  *) echo "usage: $0 <stg|prod> [--days N] [--dry-run]" >&2; exit 2 ;;
esac
DAYS=7
DRY=0
while [ $# -gt 0 ]; do
  case "$1" in
    --days) DAYS="${2:?--days N}"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done
case "$DAYS" in ''|*[!0-9]*) echo "--days must be a number" >&2; exit 2 ;; esac

PROJECT_ID="${PROJECT_ID:-jphacks-kz2601}"
AI_SERVICE="${AI_SERVICE:-ai-$ENV_NAME}"
LIMIT="${LIMIT:-20000}"
REPO="jphacks/kz_2601"

for c in gcloud jq; do command -v "$c" >/dev/null || { echo "$c が必要です" >&2; exit 2; }; done

SINCE="$(date -u -v-"${DAYS}"d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d "-${DAYS} days" +%Y-%m-%dT%H:%M:%SZ)"
UNTIL="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
BASE="resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${AI_SERVICE}\" AND timestamp>=\"${SINCE}\" AND timestamp<\"${UNTIL}\""

# 指標ごとのフィルタ(docs/release/beta-quality-gate.md の Logs Explorer のクエリと同じ)
F_GENERATE="${BASE} AND jsonPayload.event=\"ai.generate\""
F_HTTP="${BASE} AND logName=\"projects/${PROJECT_ID}/logs/run.googleapis.com%2Frequests\" AND httpRequest.requestMethod=\"POST\" AND httpRequest.requestUrl:\"/v1/generate\""
F_USAGE="${BASE} AND jsonPayload.event=\"ai.usage\""

if [ "$DRY" = 1 ]; then
  printf 'project=%s service=%s window=%s..%s\n\n' "$PROJECT_ID" "$AI_SERVICE" "$SINCE" "$UNTIL"
  printf '[ai.generate]\n%s\n\n[POST /v1/generate request log]\n%s\n\n[ai.usage]\n%s\n' "$F_GENERATE" "$F_HTTP" "$F_USAGE"
  exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# read <filter> <projection> <out>: gcloud logging read (read-only) to a JSON array
read_logs() {
  gcloud logging read "$1" --project="$PROJECT_ID" --limit="$LIMIT" --order=desc \
    --format="json($2)" >"$3"
  local n
  n="$(jq length "$3")"
  if [ "$n" -ge "$LIMIT" ]; then
    echo "warning: $n 行で LIMIT に達した。LIMIT を上げてやり直す" >&2
  fi
}

read_logs "$F_GENERATE" "jsonPayload.mode,jsonPayload.outcome,jsonPayload.validation_passed,jsonPayload.first_generation,jsonPayload.duration_ms" "$TMP/gen.json"
read_logs "$F_HTTP" "httpRequest.status,httpRequest.latency" "$TMP/http.json"
read_logs "$F_USAGE" "jsonPayload.outcome" "$TMP/usage.json"

# nearest-rank percentile of a numeric array
# shellcheck disable=SC2016 # jq code, not shell
JQ_PCT='def pct(p): sort as $s | if ($s|length)==0 then null else $s[(((($s|length)*p)|ceil)-1)] end;'

gen="$(jq -r "$JQ_PCT"'
  map(.jsonPayload) as $g
  | ($g | map(select((.mode=="create" or .mode=="edit") and .outcome!="bad_request"))) as $den
  | ($den | map(select(.validation_passed==true)) | length) as $ok
  | ($g | map(select(.first_generation==true and .validation_passed==true) | .duration_ms // empty)) as $first
  | [($den|length), $ok,
     (if ($den|length)>0 then ($ok*1000/($den|length)|floor)/10 else null end),
     ($first|length), ($first|pct(0.5)), ($first|pct(0.95)),
     ($g | group_by(.outcome) | map("\(.[0].outcome)=\(length)") | join(" "))]
  | map(. // "" | tostring) | join("|")' "$TMP/gen.json")"
IFS='|' read -r GEN_N GEN_OK GEN_RATE FIRST_N FIRST_P50 FIRST_P95 GEN_OUTCOMES <<<"$gen"

http="$(jq -r "$JQ_PCT"'
  map(.httpRequest) as $h
  | ($h | map(.latency // "0s" | rtrimstr("s") | tonumber)) as $lat
  | [($h|length), ($h | map(select((.status // 0) >= 500)) | length), ($lat|pct(0.95)|if . then (.*10|round)/10 else . end)]
  | map(. // "" | tostring) | join("|")' "$TMP/http.json")"
IFS='|' read -r HTTP_N HTTP_5XX HTTP_P95 <<<"$http"

usage="$(jq -r 'map(.jsonPayload) | [length, (map(select(.outcome=="error"))|length)] | map(tostring) | join("|")' "$TMP/usage.json")"
IFS='|' read -r USE_N USE_ERR <<<"$usage"

P1="(gh なし)"
if command -v gh >/dev/null; then
  # β のフィードバックの Issue は件名が「[β]」で始まる(docs/release/beta-quality-gate.md の振り分け)
  P1="$(gh issue list -R "$REPO" --state open --label priority:P1 --limit 200 --json title \
    -q '[.[] | select(.title | startswith("[β]"))] | length' 2>/dev/null || echo "(取得失敗)")"
fi

judge() { # judge <value> <op> <threshold>
  [ -z "$1" ] && { echo "未計測"; return; }
  if awk -v v="$1" -v t="$3" -v op="$2" 'BEGIN { exit !((op==">=" && v>=t) || (op=="<=" && v<=t) || (op=="==" && v==t)) }'; then
    echo "満たす"
  else
    echo "満たさない"
  fi
}
ms_to_s() { [ -z "$1" ] || awk -v m="$1" 'BEGIN { printf "%.1f", m/1000 }'; }

FIRST_P95_S="$(ms_to_s "$FIRST_P95")"
FIRST_P50_S="$(ms_to_s "$FIRST_P50")"

printf '# β 品質ゲート %s(%s・%s〜%s UTC・%s 日)\n\n' "$ENV_NAME" "$AI_SERVICE" "$SINCE" "$UNTIL" "$DAYS"
printf '| 基準 | 値 | 基準値 | 判定 | 件数・補足 |\n|---|---|---|---|---|\n'
printf '| クラッシュなし率(ユーザー) | Crashlytics で見る | 99.5%% 以上 | 手で記入 | Firebase Console → Crashlytics |\n'
printf '| 生成の成功率(検証を通る) | %s | 98%% 以上 | %s | %s/%s 件(%s) |\n' \
  "${GEN_RATE:+$GEN_RATE%}" "$(judge "$GEN_RATE" ">=" 98)" "${GEN_OK:-0}" "${GEN_N:-0}" "${GEN_OUTCOMES:-ai.generate なし}"
printf '| 初回の生成の p95 | %s 秒 | 30 秒以内 | %s | %s 件・p50 %s 秒(サーバー側の時間) |\n' \
  "${FIRST_P95_S:-—}" "$(judge "$FIRST_P95_S" "<=" 30)" "${FIRST_N:-0}" "${FIRST_P50_S:-—}"
printf '| 7 日後に開いた率(D7) | scripts/product-metrics-weekly.sh で見る | 記録のみ | 手で記入 | #297 のイベント・docs/metrics/product-metrics.md |\n'
printf '| P1 のフィードバックの残数 | %s | 0 | %s | 件名「[β]」・priority:P1・open |\n' "$P1" "$(judge "$P1" "==" 0)"
printf '\n参考(基準ではない)\n\n| 項目 | 値 |\n|---|---|\n'
printf '| POST /v1/generate のリクエスト(Cloud Run) | %s 件・5xx %s 件・p95 %s 秒 |\n' "${HTTP_N:-0}" "${HTTP_5XX:-0}" "${HTTP_P95:-—}"
printf '| Claude の試行(ai.usage) | %s 件・error %s 件 |\n' "${USE_N:-0}" "${USE_ERR:-0}"
