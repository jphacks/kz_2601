#!/usr/bin/env bash
# runner(自動の動き)の使った量を、アプリごと・作成者ごと・日 / 月ごとに出す(#345・#338)。
# 読み取りだけ(gcloud logging read)。見るだけで、止める仕組み(backend/internal/runner/quota)とは別。
# ログの形と見方は docs/infra/runner-usage.md。
#
# 使い方:
#   scripts/runner-usage.sh prod                    # 昨日(日本時間)と直近 7 日
#   scripts/runner-usage.sh prod --days 30          # 30 日(月ごとの合計・期間の上位)
#   scripts/runner-usage.sh prod --day 2026-11-01   # 見る日を指定
#   scripts/runner-usage.sh prod --threshold 500    # 1 アプリ 1 日の実行回数の閾値。超えたら終了コード 3
#   scripts/runner-usage.sh prod --dry-run          # 実行せずにフィルタだけ出す
#   scripts/runner-usage.sh --input usage.json --day 2026-11-01
#                                                   # gcloud の JSON を読む(オフライン・テスト用)
#
# runner は #288 の分離まで prod だけで動く(stg には出さない)。stg を指定しても空になる。
# 要る権限: GCP プロジェクトの roles/logging.viewer(読み取りだけ)。要るコマンド: jq(と gcloud)
# 環境変数: PROJECT_ID(既定 jphacks-kz2601)・RUNNER_SERVICE(既定 runner-<env>)・LIMIT(既定 50000)・
#           RUNNER_USAGE_APP_EXEC_THRESHOLD(既定 2000 = Pro の 1 アプリ 1 日の上限)・TOP(上位の数、既定 10)
set -euo pipefail

ENV_NAME=""
case "${1:-}" in
  stg|prod) ENV_NAME="$1"; shift ;;
esac
DAYS=7
DAY=""
DRY=0
INPUT=""
THRESHOLD="${RUNNER_USAGE_APP_EXEC_THRESHOLD:-2000}"
TOP="${TOP:-10}"
while [ $# -gt 0 ]; do
  case "$1" in
    --days) DAYS="${2:?--days N}"; shift 2 ;;
    --day) DAY="${2:?--day YYYY-MM-DD}"; shift 2 ;;
    --threshold) THRESHOLD="${2:?--threshold N}"; shift 2 ;;
    --input) INPUT="${2:?--input FILE}"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done
if [ -z "$ENV_NAME" ] && [ -z "$INPUT" ]; then
  echo "usage: $0 <prod|stg> [--days N] [--day YYYY-MM-DD] [--threshold N] [--dry-run]" >&2
  echo "       $0 --input usage.json [--day YYYY-MM-DD] [--threshold N]" >&2
  exit 2
fi
case "$DAYS" in ''|*[!0-9]*) echo "--days must be a number" >&2; exit 2 ;; esac
case "$TOP" in ''|*[!0-9]*) echo "TOP must be a number" >&2; exit 2 ;; esac
case "$THRESHOLD" in ''|*[!0-9]*) echo "--threshold must be a whole number" >&2; exit 2 ;; esac

command -v jq >/dev/null || { echo "jq が必要です" >&2; exit 2; }

# 日付は日本時間(JST = UTC+9、夏時間なし)で切る。上限の日(quota)と同じ区切り
jst_date() { # jst_date <days ago>
  TZ=Asia/Tokyo date -v-"$1"d +%Y-%m-%d 2>/dev/null || TZ=Asia/Tokyo date -d "-$1 days" +%Y-%m-%d
}
[ -n "$DAY" ] || DAY="$(jst_date 1)"
case "$DAY" in [0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]) ;; *) echo "--day must be YYYY-MM-DD" >&2; exit 2 ;; esac

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if [ -n "$INPUT" ]; then
  cp "$INPUT" "$TMP/usage.json"
  SRC="$INPUT"
else
  [ "$ENV_NAME" = stg ] && echo "note: runner は #288 まで prod だけで動く。stg は空のはず" >&2
  PROJECT_ID="${PROJECT_ID:-jphacks-kz2601}"
  RUNNER_SERVICE="${RUNNER_SERVICE:-runner-$ENV_NAME}"
  LIMIT="${LIMIT:-50000}"
  SINCE_DAY="$(jst_date "$DAYS")"
  [[ "$DAY" < "$SINCE_DAY" ]] && SINCE_DAY="$DAY"
  SINCE="${SINCE_DAY}T00:00:00+09:00"
  FILTER="resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${RUNNER_SERVICE}\" AND timestamp>=\"${SINCE}\" AND jsonPayload.event=\"runner.usage\""
  if [ "$DRY" = 1 ]; then
    printf 'project=%s service=%s since=%s day=%s threshold=%s executions/app/day\n\n' "$PROJECT_ID" "$RUNNER_SERVICE" "$SINCE" "$DAY" "$THRESHOLD"
    printf '[runner.usage]\n%s\n' "$FILTER"
    exit 0
  fi
  command -v gcloud >/dev/null || { echo "gcloud が必要です" >&2; exit 2; }
  P="timestamp,jsonPayload.app_id,jsonPayload.creator_uid_hash,jsonPayload.trigger,jsonPayload.outcome,jsonPayload.limit"
  P="$P,jsonPayload.reads,jsonPayload.writes,jsonPayload.writes_dropped,jsonPayload.notify_recipients"
  P="$P,jsonPayload.recipients_dropped,jsonPayload.est_cost_usd"
  gcloud logging read "$FILTER" --project="$PROJECT_ID" --limit="$LIMIT" --order=desc --format="json($P)" >"$TMP/usage.json"
  if [ "$(jq length "$TMP/usage.json")" -ge "$LIMIT" ]; then
    echo "warning: LIMIT($LIMIT 行)に達した。数字が少なく出ている。LIMIT を上げてやり直す" >&2
  fi
  SRC="${RUNNER_SERVICE}(${PROJECT_ID})"
fi

# 共通の jq。実行 = outcome が ok / error(quota が実行として数えるもの)。skipped_* は止めた分
# shellcheck disable=SC2016 # jq code, not shell
JQ_LIB='
def jst: .timestamp | sub("\\.[0-9]+"; "") | sub("\\+00:00$"; "Z") | fromdateiso8601 + 32400;
# 1 実行は 0.000016 USD ほどなので小数 6 桁まで(指数表記にしない)
def usd: if . == null then "—" else (. * 1000000 | round) as $m
  | "\($m / 1000000 | floor).\("000000\($m % 1000000)" | .[-6:])" end;
def rows: map(.jsonPayload as $p | (jst) as $t | {
  day: ($t | strftime("%Y-%m-%d")), month: ($t | strftime("%Y-%m")),
  app: ($p.app_id // "—"), creator: ($p.creator_uid_hash // "—"),
  outcome: ($p.outcome // "unknown"), limit: ($p.limit // ""),
  exec: (if $p.outcome == "ok" or $p.outcome == "error" then 1 else 0 end),
  err: (if $p.outcome == "error" then 1 else 0 end),
  skipped: (if ($p.outcome // "") | startswith("skipped") then 1 else 0 end),
  reads: ($p.reads // 0), writes: ($p.writes // 0), writes_dropped: ($p.writes_dropped // 0),
  notify: ($p.notify_recipients // 0), recipients_dropped: ($p.recipients_dropped // 0),
  cost: ($p.est_cost_usd // 0)});
def sums: {exec: (map(.exec) | add // 0), err: (map(.err) | add // 0), skipped: (map(.skipped) | add // 0),
  reads: (map(.reads) | add // 0), writes: (map(.writes) | add // 0),
  writes_dropped: (map(.writes_dropped) | add // 0), notify: (map(.notify) | add // 0),
  recipients_dropped: (map(.recipients_dropped) | add // 0), cost: (map(.cost) | add // 0)};
def ranked(key): group_by(key) | map({k: (.[0] | key), apps: (map(.app) | unique | length)} + sums)
  | sort_by(-.exec, -.writes, -.notify, .k);
def app_table(title): "\n## \(title)\n\n| app_id | 実行 | error | 止めた | 書き込み | 通知の宛先 | 読み取り | USD |\n|---|---|---|---|---|---|---|---|\n"
  + (if length == 0 then "| (無し) | 0 | 0 | 0 | 0 | 0 | 0 | 0 |"
     else map("| \(.k) | \(.exec) | \(.err) | \(.skipped) | \(.writes) | \(.notify) | \(.reads) | \(.cost | usd) |") | join("\n") end);
def creator_table(title): "\n## \(title)\n\n| creator_uid_hash | アプリ | 実行 | 止めた | 書き込み | 通知の宛先 | USD |\n|---|---|---|---|---|---|---|\n"
  + (if length == 0 then "| (無し) | 0 | 0 | 0 | 0 | 0 | 0 |"
     else map("| \(.k) | \(.apps) | \(.exec) | \(.skipped) | \(.writes) | \(.notify) | \(.cost | usd) |") | join("\n") end);
'

printf '# runner の使った量 %s(見る日 %s・日本時間)\n\n' "$SRC" "$DAY"
printf '見るだけの集計。止めるのは runner の上限(RUNNER_LIMIT_*)。USD は #338 の単価の見積もり。\n\n'

# 1) 見る日の合計
jq -r --arg d "$DAY" "$JQ_LIB"'
  rows | map(select(.day == $d)) as $c | ($c | sums) as $s
  | "## 1 日の合計\n\n| 項目 | 値 |\n|---|---|\n"
    + "| 実行 | \($s.exec) 回(error \($s.err) 回) |\n"
    + "| 止めた(skipped_*) | \($s.skipped) 回 |\n"
    + "| 書き込み | \($s.writes) 件(上限で落とした \($s.writes_dropped) 件) |\n"
    + "| 通知の宛先 | \($s.notify) 人(上限で落とした \($s.recipients_dropped) 人) |\n"
    + "| 読み取り | \($s.reads) 件 |\n"
    + "| アプリ数 | \($c | map(select(.exec > 0) | .app) | unique | length) |\n"
    + "| 作成者数 | \($c | map(select(.exec > 0) | .creator) | unique | length) |\n"
    + "| 見積もり(USD) | \($s.cost | usd) |"
' "$TMP/usage.json"

# 2) 止めた理由(見る日)
jq -r --arg d "$DAY" "$JQ_LIB"'
  rows | map(select(.day == $d and (.skipped == 1 or .limit != "")))
  | group_by([.outcome, .limit]) | map({outcome: .[0].outcome, limit: .[0].limit, n: length, apps: (map(.app) | unique | length)})
  | sort_by(-.n, .outcome, .limit)
  | "\n## 止めた・削った理由(\($d))\n\n| outcome | limit | 回数 | アプリ数 |\n|---|---|---|---|\n"
    + (if length == 0 then "| (無し) | | 0 | 0 |"
       else map("| \(.outcome) | \(if .limit == "" then "—" else .limit end) | \(.n) | \(.apps) |") | join("\n") end)
' "$TMP/usage.json"

# 3) 4) 見る日の上位のアプリ・作成者
jq -r --arg d "$DAY" --argjson top "$TOP" "$JQ_LIB"'
  rows | map(select(.day == $d))
  | (ranked(.app) | .[:$top] | app_table("上位のアプリ(\($d))")),
    (ranked(.creator) | .[:$top] | creator_table("上位の作成者(\($d))"))
' "$TMP/usage.json"

# 5) 日ごと
jq -r "$JQ_LIB"'
  rows | group_by(.day) | reverse
  | map({day: .[0].day, apps: (map(select(.exec > 0) | .app) | unique | length)} + sums)
  | "\n## 日ごと\n\n| 日 | 実行 | 止めた | 書き込み | 通知の宛先 | アプリ | USD |\n|---|---|---|---|---|---|---|\n"
    + (map("| \(.day) | \(.exec) | \(.skipped) | \(.writes) | \(.notify) | \(.apps) | \(.cost | usd) |") | join("\n"))
' "$TMP/usage.json"

# 6) 月ごと(日本時間の暦月。期間の途中から始まる月は一部だけ)。上限はかけない、見るだけ
jq -r "$JQ_LIB"'
  rows | (map(.day) | min) as $from | group_by(.month) | reverse
  | map({month: .[0].month, partial: ((.[0].month + "-01") < ($from // "")),
         apps: (map(select(.exec > 0) | .app) | unique | length),
         creators: (map(select(.exec > 0) | .creator) | unique | length)} + sums)
  | "\n## 月ごと(日本時間)\n\n| 月 | 実行 | 止めた | 書き込み | 通知の宛先 | アプリ | 作成者 | USD |\n|---|---|---|---|---|---|---|---|\n"
    + (map("| \(.month)\(if .partial then "(期間の途中から)" else "" end) | \(.exec) | \(.skipped) | \(.writes) | \(.notify) | \(.apps) | \(.creators) | \(.cost | usd) |") | join("\n"))
' "$TMP/usage.json"

# 7) 期間の上位(--days 30 で月の上位)
jq -r --argjson top "$TOP" "$JQ_LIB"'
  rows | (map(.day) | min) as $from | (map(.day) | max) as $to
  | (ranked(.app) | .[:$top] | app_table("期間の上位のアプリ(\($from // "—")〜\($to // "—"))")),
    (ranked(.creator) | .[:$top] | creator_table("期間の上位の作成者(\($from // "—")〜\($to // "—"))"))
' "$TMP/usage.json"

# 8) 閾値: 見る日に 1 アプリの実行回数が閾値を超えたか
read -r MAX_APP MAX_EXEC <<<"$(jq -r --arg d "$DAY" "$JQ_LIB"'
  rows | map(select(.day == $d)) | ranked(.app) | (.[0] // {k: "—", exec: 0}) | "\(.k) \(.exec)"
' "$TMP/usage.json")"
printf '\n## 閾値\n\n'
if [ "$MAX_EXEC" -gt "$THRESHOLD" ]; then
  printf '%s に実行回数の最も多いアプリ %s は %s 回で、閾値 %s 回を**超えた**。上位のアプリと止めた理由を見る\n' "$DAY" "$MAX_APP" "$MAX_EXEC" "$THRESHOLD"
  exit 3
fi
printf '%s の 1 アプリあたりの実行回数はどれも閾値 %s 回以下(最大 %s 回)\n' "$DAY" "$THRESHOLD" "$MAX_EXEC"
