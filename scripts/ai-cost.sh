#!/usr/bin/env bash
# AI(Claude)の費用を日ごと・ユーザーごと・1 生成ごとに出す(#298)。読み取りだけ(gcloud logging read)。
# ログの形と見方は docs/infra/ai-cost.md。
#
# 使い方:
#   scripts/ai-cost.sh prod                    # 昨日(日本時間)と直近 7 日
#   scripts/ai-cost.sh stg --days 30           # 30 日(1 人あたりの月の費用の分布にも使う)
#   scripts/ai-cost.sh prod --day 2026-11-01   # 見る日を指定
#   scripts/ai-cost.sh prod --threshold 50     # 1 日の閾値(USD)。超えたら終了コード 3
#   scripts/ai-cost.sh prod --dry-run          # 実行せずにフィルタだけ出す
#   scripts/ai-cost.sh --input usage.json [--generate-input gen.json] --day 2026-11-01
#                                              # gcloud の JSON を読む(オフライン・テスト用)
#
# 要る権限: GCP プロジェクトの roles/logging.viewer(読み取りだけ)。要るコマンド: jq(と gcloud)
# 環境変数: PROJECT_ID(既定 jphacks-kz2601)・AI_SERVICE(既定 ai-<env>)・LIMIT(既定 50000)・
#           AI_COST_DAILY_THRESHOLD_USD(既定 20)・TOP(上位の人数、既定 10)
set -euo pipefail

ENV_NAME=""
case "${1:-}" in
  stg|prod) ENV_NAME="$1"; shift ;;
esac
DAYS=7
DAY=""
DRY=0
INPUT=""
GEN_INPUT=""
THRESHOLD="${AI_COST_DAILY_THRESHOLD_USD:-20}"
TOP="${TOP:-10}"
while [ $# -gt 0 ]; do
  case "$1" in
    --days) DAYS="${2:?--days N}"; shift 2 ;;
    --day) DAY="${2:?--day YYYY-MM-DD}"; shift 2 ;;
    --threshold) THRESHOLD="${2:?--threshold USD}"; shift 2 ;;
    --input) INPUT="${2:?--input FILE}"; shift 2 ;;
    --generate-input) GEN_INPUT="${2:?--generate-input FILE}"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done
if [ -z "$ENV_NAME" ] && [ -z "$INPUT" ]; then
  echo "usage: $0 <stg|prod> [--days N] [--day YYYY-MM-DD] [--threshold USD] [--dry-run]" >&2
  echo "       $0 --input usage.json [--generate-input gen.json] [--day YYYY-MM-DD]" >&2
  exit 2
fi
case "$DAYS" in ''|*[!0-9]*) echo "--days must be a number" >&2; exit 2 ;; esac
case "$TOP" in ''|*[!0-9]*) echo "TOP must be a number" >&2; exit 2 ;; esac
awk -v t="$THRESHOLD" 'BEGIN { exit !(t ~ /^[0-9]+(\.[0-9]+)?$/) }' || { echo "--threshold must be a number" >&2; exit 2; }

command -v jq >/dev/null || { echo "jq が必要です" >&2; exit 2; }

# 日付は日本時間(JST = UTC+9、夏時間なし)で切る
jst_date() { # jst_date <days ago>
  TZ=Asia/Tokyo date -v-"$1"d +%Y-%m-%d 2>/dev/null || TZ=Asia/Tokyo date -d "-$1 days" +%Y-%m-%d
}
[ -n "$DAY" ] || DAY="$(jst_date 1)"
case "$DAY" in [0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]) ;; *) echo "--day must be YYYY-MM-DD" >&2; exit 2 ;; esac

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if [ -n "$INPUT" ]; then
  cp "$INPUT" "$TMP/usage.json"
  if [ -n "$GEN_INPUT" ]; then cp "$GEN_INPUT" "$TMP/gen.json"; else echo '[]' >"$TMP/gen.json"; fi
  SRC="$INPUT"
else
  command -v gcloud >/dev/null || { echo "gcloud が必要です" >&2; exit 2; }
  PROJECT_ID="${PROJECT_ID:-jphacks-kz2601}"
  AI_SERVICE="${AI_SERVICE:-ai-$ENV_NAME}"
  LIMIT="${LIMIT:-50000}"
  # 窓: 見る日と、その前の DAYS 日のうち早い方の 0 時(JST)から今まで
  SINCE_DAY="$(jst_date "$DAYS")"
  [[ "$DAY" < "$SINCE_DAY" ]] && SINCE_DAY="$DAY"
  SINCE="${SINCE_DAY}T00:00:00+09:00"
  BASE="resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${AI_SERVICE}\" AND timestamp>=\"${SINCE}\""
  F_USAGE="${BASE} AND jsonPayload.event=\"ai.usage\""
  F_GENERATE="${BASE} AND jsonPayload.event=\"ai.generate\""
  if [ "$DRY" = 1 ]; then
    printf 'project=%s service=%s since=%s day=%s threshold=%s USD\n\n' "$PROJECT_ID" "$AI_SERVICE" "$SINCE" "$DAY" "$THRESHOLD"
    printf '[ai.usage]\n%s\n\n[ai.generate]\n%s\n' "$F_USAGE" "$F_GENERATE"
    exit 0
  fi
  read_logs() { # read_logs <filter> <projection> <out>
    gcloud logging read "$1" --project="$PROJECT_ID" --limit="$LIMIT" --order=desc --format="json($2)" >"$3"
    if [ "$(jq length "$3")" -ge "$LIMIT" ]; then
      echo "warning: LIMIT($LIMIT 行)に達した。数字が少なく出ている。LIMIT を上げてやり直す" >&2
    fi
  }
  read_logs "$F_USAGE" "timestamp,jsonPayload.operation,jsonPayload.outcome,jsonPayload.uid_hash,jsonPayload.est_cost_usd" "$TMP/usage.json"
  read_logs "$F_GENERATE" "timestamp,jsonPayload.mode,jsonPayload.outcome,jsonPayload.est_cost_usd" "$TMP/gen.json"
  SRC="${AI_SERVICE}(${PROJECT_ID})"
fi

# 共通の jq: JST の日付・最近順位法のパーセンタイル・USD の丸め
# shellcheck disable=SC2016 # jq code, not shell
JQ_LIB='
def jst_day: .timestamp | sub("\\.[0-9]+"; "") | sub("\\+00:00$"; "Z") | fromdateiso8601 + 32400 | strftime("%Y-%m-%d");
def pct(p): sort as $s | if ($s|length)==0 then null else $s[((($s|length)*p)|ceil|if .<1 then 1 else . end)-1] end;
def usd: if . == null then "—" else (. * 10000 | round / 10000 | tostring) end;
def rows: map({day: jst_day, op: (.jsonPayload.operation // "unknown"), outcome: .jsonPayload.outcome,
  uid: .jsonPayload.uid_hash, cost: .jsonPayload.est_cost_usd, mode: .jsonPayload.mode});
'

printf '# AI 費用 %s(見る日 %s・日本時間)\n\n' "$SRC" "$DAY"
printf '見積もり(料金表 × トークン)。請求額は Anthropic のコンソールで月に 1 回突き合わせる。\n\n'

# 1) 見る日の合計・1 人あたりの中央値と上位 1%
read -r TOTAL <<<"$(jq -r --arg d "$DAY" "$JQ_LIB"'
  rows | map(select(.day == $d)) as $c
  | ($c | map(.cost // 0) | add // 0) as $total
  | ($c | map(select(.uid != null)) | group_by(.uid) | map({uid: .[0].uid, cost: (map(.cost // 0) | add), calls: length})) as $u
  | ($u | map(.cost)) as $uc
  | ($uc | pct(0.99)) as $p99
  | "## 1 日の合計\n\n| 項目 | 値 |\n|---|---|\n"
    + "| 合計(USD) | \($total | usd) |\n"
    + "| Claude の呼び出し | \($c | length) 回(error \($c | map(select(.outcome == "error")) | length) 回) |\n"
    + "| 未ログインの分(USD) | \($c | map(select(.uid == null) | .cost // 0) | add // 0 | usd) |\n"
    + "| 料金表にない呼び出し | \($c | map(select(.cost == null)) | length) 回(0 でなければ料金表を足す) |\n"
    + "| ユーザー数 | \($u | length) 人 |\n"
    + "| 1 人あたりの中央値(USD) | \($uc | pct(0.5) | usd) |\n"
    + "| 1 人あたりの上位 1%(p99, USD) | \($p99 | usd) |\n"
    + "| 1 人あたりの最大(USD) | \($uc | max | usd) |\n"
    + "\n## 上位のユーザー(\($d))\n\n| uid_hash | USD | 呼び出し | 上位 1% |\n|---|---|---|---|\n"
    + ($u | sort_by(-.cost) | .[:'"$TOP"'] | map("| \(.uid) | \(.cost | usd) | \(.calls) | \(if $p99 != null and .cost >= $p99 then "○" else "" end) |") | join("\n"))
    + "\nTOTAL=\($total)"
  ' "$TMP/usage.json" | tee "$TMP/day.md" | sed -n 's/^TOTAL=//p')"
sed '/^TOTAL=/d' "$TMP/day.md"

# 2) 処理ごと(見る日)
jq -r --arg d "$DAY" "$JQ_LIB"'
  rows | map(select(.day == $d)) | group_by(.op)
  | map({op: .[0].op, calls: length, cost: (map(.cost // 0) | add)}) | sort_by(-.cost)
  | "\n## 処理ごと(\($d))\n\n| 処理 | 呼び出し | USD |\n|---|---|---|\n"
    + (map("| \(.op) | \(.calls) | \(.cost | usd) |") | join("\n"))
' "$TMP/usage.json"

# 3) 日ごと
jq -r "$JQ_LIB"'
  rows | group_by(.day) | reverse
  | map({day: .[0].day, calls: length, users: (map(.uid // empty) | unique | length), cost: (map(.cost // 0) | add)})
  | "\n## 日ごと\n\n| 日 | USD | 呼び出し | ユーザー |\n|---|---|---|---|\n"
    + (map("| \(.day) | \(.cost | usd) | \(.calls) | \(.users) |") | join("\n"))
' "$TMP/usage.json"

# 4) 期間の 1 人あたりの費用の分布(--days 30 で月の分布。Pro の価格 #325・上限 #317 の根拠)
jq -r "$JQ_LIB"'
  rows | (map(.day) | min) as $from | (map(.day) | max) as $to
  | map(select(.uid != null)) | group_by(.uid) | map(map(.cost // 0) | add) as $u
  | "\n## 期間の 1 人あたりの費用(\($from // "—")〜\($to // "—"))\n\n| 人数 | p50 | p90 | p99 | 最大 |\n|---|---|---|---|---|\n"
    + "| \($u | length) | \($u | pct(0.5) | usd) | \($u | pct(0.9) | usd) | \($u | pct(0.99) | usd) | \($u | max | usd) |"
' "$TMP/usage.json"

# 5) 1 生成ごとの費用(ai.generate の est_cost_usd。#298 より前のログには無い)
jq -r "$JQ_LIB"'
  rows | map(select(.cost != null)) | group_by(.mode)
  | map({mode: .[0].mode, n: length, c: map(.cost)})
  | "\n## 1 生成(POST /v1/generate)ごとの費用(期間)\n\n| mode | 件数 | 平均 | p50 | p95 |\n|---|---|---|---|---|\n"
    + (if length == 0 then "| (ai.generate に費用が無い) | 0 | — | — | — |"
       else map("| \(.mode) | \(.n) | \(.c | add / length | usd) | \(.c | pct(0.5) | usd) | \(.c | pct(0.95) | usd) |") | join("\n") end)
' "$TMP/gen.json"

# 6) 閾値
printf '\n## 閾値\n\n'
if awk -v v="$TOTAL" -v t="$THRESHOLD" 'BEGIN { exit !(v > t) }'; then
  printf '%s の合計 %s USD は閾値 %s USD を**超えた**。上位のユーザーと処理ごとを見る\n' "$DAY" "$(awk -v v="$TOTAL" 'BEGIN { printf "%.4f", v }')" "$THRESHOLD"
  exit 3
fi
printf '%s の合計は閾値 %s USD 以下\n' "$DAY" "$THRESHOLD"
