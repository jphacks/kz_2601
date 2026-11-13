#!/usr/bin/env bash
# Cloud Run の段階的なリリース(#372)。新しいリビジョンへトラフィックを段階的に移し(既定 5 → 25 → 100%)、
# 段階ごとに新旧のリビジョンの 5xx 率・p95 の待ち時間・Claude の失敗率・1 回あたりの費用を比べ、基準を超えたら前のリビジョンに 100% 戻す。
# 見方・閾値の選び方・deploy の workflow への組み込み方は docs/infra/staged-rollout.md。
#
# 既定は --dry-run(コマンドを出すだけ・GCP に触らない)。実際に動かすには --execute。prod は --env prod --execute の両方が要る。
#
# 使い方:
#   scripts/rollout.sh --env stg --service appconfig                        # dry-run: 出すコマンドを見る
#   scripts/rollout.sh --env stg --service ai --execute                     # stg で実行(新 = 最新に作ったリビジョン)
#   scripts/rollout.sh --env prod --service appconfig --execute \
#     --to-revision appconfig-prod-00043-xyz --steps 5,25,50,100 --wait 600
#   scripts/rollout.sh --env stg --service ai --metrics-dir DIR             # dry-run で段階ごとの指標を DIR/<割合>.json から読む(模擬)
#   scripts/rollout.sh --decide metrics.json [--max-cost-ratio N]           # 判定だけ(オフライン・テスト用)
#   scripts/rollout.sh --summarize logs.json --new REV --old REV            # gcloud logging read の JSON を指標にする(オフライン)
#
# 終了コード: 0 = 100% まで進んだ(dry-run は最後まで出した) / 2 = 使い方の誤り / 3 = 巻き戻した / 4 = 指標が足りず巻き戻した
# 要る権限(--execute): roles/run.developer(update-traffic)と roles/logging.viewer。要るコマンド: jq(と gcloud)
# 環境変数: PROJECT_ID(既定 jphacks-kz2601)・REGION(既定 asia-northeast1)と、下の閾値(ROLLOUT_*)
set -euo pipefail

# ---- 閾値(docs/infra/staged-rollout.md の「閾値の選び方」) ----------------------------------
MAX_ERROR_RATE="${ROLLOUT_MAX_ERROR_RATE:-0.05}"             # 新の 5xx 率の上限(監視のアラートと同じ 5%)
MAX_ERROR_RATE_DELTA="${ROLLOUT_MAX_ERROR_RATE_DELTA:-0.01}" # 新 - 旧 の 5xx 率の差の上限
MAX_P95_RATIO="${ROLLOUT_MAX_P95_RATIO:-1.5}"                # 新の p95 / 旧の p95 の上限
P95_SLACK_MS="${ROLLOUT_P95_SLACK_MS:-100}"                  # 差がこれ以下なら比が大きくても通す(速いエンドポイントの揺れ)
MAX_CLAUDE_FAILURE_RATE="${ROLLOUT_MAX_CLAUDE_FAILURE_RATE:-0.2}" # 新の Claude の失敗率の上限(監視のアラートと同じ 20%)
CLAUDE_FAILURE_MIN_COUNT="${ROLLOUT_CLAUDE_FAILURE_MIN_COUNT:-3}" # 失敗がこの件数未満なら率は見ない
MIN_REQUESTS="${ROLLOUT_MIN_REQUESTS:-20}"                   # 新のリクエストがこれ未満なら判定しない(insufficient)
MAX_COST_RATIO="${ROLLOUT_MAX_COST_RATIO:-2}"                # 新の 1 回あたりの費用 / 旧の上限(--max-cost-ratio でも変えられる)
COST_MIN_CALLS="${ROLLOUT_COST_MIN_CALLS:-5}"                # 新旧どちらかの費用つきの呼び出しがこれ未満なら費用は見ない

usage() {
  cat >&2 <<'EOF'
usage: scripts/rollout.sh --env <stg|prod> --service <appconfig|ai|NAME> [--execute]
         [--to-revision REV] [--from-revision REV] [--steps 5,25,100] [--wait SEC]
         [--max-extends N] [--on-insufficient rollback|promote] [--metrics-dir DIR] [--max-cost-ratio N]
       scripts/rollout.sh --decide metrics.json [--max-cost-ratio N]
       scripts/rollout.sh --summarize logs.json --new REV --old REV
EOF
}

is_num() { awk -v t="$1" 'BEGIN { exit !(t ~ /^[0-9]+(\.[0-9]+)?$/) }'; }
is_int() { case "$1" in ''|*[!0-9]*) return 1 ;; esac; }

# ---- 純粋な関数(gcloud なし・テスト対象) ------------------------------------------------------

# parse_steps "5,25,100" → 1 行に 1 つ。1..100 の整数・増える順・最後は 100 でなければ失敗
parse_steps() {
  local prev=0 s out=""
  local IFS=,
  for s in $1; do
    is_int "$s" || { echo "steps: 整数でない: '$s'" >&2; return 1; }
    s=$((10#$s))
    if [ "$s" -lt 1 ] || [ "$s" -gt 100 ]; then echo "steps: 1..100 の外: $s" >&2; return 1; fi
    [ "$s" -gt "$prev" ] || { echo "steps: 増える順でない: $prev → $s" >&2; return 1; }
    out="$out$s"$'\n'
    prev="$s"
  done
  [ "$prev" = 100 ] || { echo "steps: 最後は 100 にする(今は $prev)" >&2; return 1; }
  printf '%s' "$out"
}

# decide <metrics.json> → 1 行目 decision=promote|rollback|insufficient、続けて reason=… を 1 行に 1 つ。
# 入力: {"new":{"requests","errors_5xx","p95_ms","claude_calls","claude_errors","cost_calls","cost_usd_per_call"},"old":{同じ}}
# 旧のリクエストが 0(100% の段階など)なら、旧との比較はせず新の絶対値だけで見る。
# 費用は新旧とも cost_calls が COST_MIN_CALLS 以上で、旧の 1 回あたりが 0 より大きい時だけ比べる。
decide() {
  jq -r \
    --argjson maxErr "$MAX_ERROR_RATE" \
    --argjson maxDelta "$MAX_ERROR_RATE_DELTA" \
    --argjson maxRatio "$MAX_P95_RATIO" \
    --argjson slack "$P95_SLACK_MS" \
    --argjson maxClaude "$MAX_CLAUDE_FAILURE_RATE" \
    --argjson claudeMin "$CLAUDE_FAILURE_MIN_COUNT" \
    --argjson minReq "$MIN_REQUESTS" \
    --argjson maxCost "$MAX_COST_RATIO" \
    --argjson costMin "$COST_MIN_CALLS" '
    def n(x): (x // 0);
    def rate(a; b): if b > 0 then a / b else 0 end;
    def r4: (. * 10000 | round) / 10000;
    def r6: (. * 1000000 | round) / 1000000; # 0.05 - 0.04 = 0.010000000000000002 を 0.01 として比べる
    (.new // {}) as $n | (.old // {}) as $o |
    # 数があるのに形が違う(文字列・欠けた項目)指標は 0 扱いで進めず、判定を失敗させる(呼ぶ側が旧に戻す)
    def shape(who; m): if (m.requests != null) and ([m.requests, m.errors_5xx, m.p95_ms] | all(type == "number") | not)
      then error("\(who) の指標の形が違う: requests / errors_5xx / p95_ms は数") else . end;
    shape("new"; $n) | shape("old"; $o) |
    n($n.requests) as $nr | n($n.errors_5xx) as $ne |
    n($o.requests) as $or | n($o.errors_5xx) as $oe |
    (rate($ne; $nr) | r6) as $nRate | (rate($oe; $or) | r6) as $oRate |
    n($n.p95_ms) as $np | n($o.p95_ms) as $op |
    n($n.claude_calls) as $nc | n($n.claude_errors) as $nce |
    (rate($nce; $nc) | r6) as $cRate |
    n($n.cost_calls) as $ncc | n($o.cost_calls) as $occ |
    n($n.cost_usd_per_call) as $ncpc | n($o.cost_usd_per_call) as $ocpc |
    [
      (if $nRate > $maxErr then "5xx rate \($nRate | r4) > \($maxErr)" else empty end),
      (if $or > 0 and (($nRate - $oRate) | r6) > $maxDelta
        then "5xx rate delta \(($nRate - $oRate) | r4) > \($maxDelta) (new \($nRate | r4) / old \($oRate | r4))" else empty end),
      (if $or > 0 and $op > 0 and $np > $op * $maxRatio and ($np - $op) > $slack
        then "p95 \($np)ms > old \($op)ms x \($maxRatio)" else empty end),
      (if $nce >= $claudeMin and $cRate > $maxClaude
        then "claude failure rate \($cRate | r4) > \($maxClaude) (\($nce)/\($nc))" else empty end),
      (if $ncc >= $costMin and $occ >= $costMin and $ocpc > 0 and ($ncpc | r6) > (($ocpc * $maxCost) | r6)
        then "cost per call \($ncpc | r6) > old \($ocpc | r6) x \($maxCost) (\($ncc)/\($occ) calls)" else empty end)
    ] as $bad |
    if ($bad | length) > 0 then "decision=rollback", ($bad[] | "reason=" + .)
    elif $nr < $minReq then "decision=insufficient", "reason=new requests \($nr) < \($minReq)"
    else "decision=promote", "reason=new 5xx \($nRate | r4) p95 \($np)ms / old 5xx \($oRate | r4) p95 \($op)ms (\($nr) req)"
    end
  ' "$1"
}

# summarize <logs.json> <new> <old> → decide の入力。logs は gcloud logging read --format=json の配列
# (jsonPayload.event = http.request / ai.usage・resource.labels.revision_name)。duration は ns(slog.Duration)
# 費用は ai.usage のうち est_cost_usd がある行(単価の無いモデルの行は数えない)の平均(1 回あたり)
summarize() {
  jq --arg new "$2" --arg old "$3" '
    def p95: sort | if length == 0 then 0 else .[((length * 0.95 | ceil) - 1)] end;
    def one(rev):
      [ .[] | select(.resource.labels.revision_name == rev) ] as $e |
      [ $e[] | .jsonPayload | select(.event == "http.request" and .path != "/health") ] as $h |
      [ $e[] | .jsonPayload | select(.event == "ai.usage") ] as $u |
      { requests: ($h | length),
        errors_5xx: ([ $h[] | select((.status // 0) >= 500) ] | length),
        p95_ms: ([ $h[] | (.duration // 0) / 1000000 ] | p95 | round),
        claude_calls: ($u | length),
        claude_errors: ([ $u[] | select(.outcome == "error") ] | length) } +
      ([ $u[] | .est_cost_usd | numbers ] as $c |
        { cost_calls: ($c | length),
          cost_usd_per_call: (if ($c | length) > 0 then (($c | add) / ($c | length) * 1000000 | round) / 1000000 else 0 end) });
    if type != "array" then error("logs が配列でない(gcloud logging read のエラーの JSON か)") else . end
    | { new: one($new), old: one($old) }
  ' "$1"
}

# ---- 判定だけ / 集計だけ(オフライン) ----------------------------------------------------------
case "${1:-}" in
  --decide)
    if [ $# -eq 4 ] && [ "$3" = --max-cost-ratio ]; then MAX_COST_RATIO="$4"; set -- "$1" "$2"; fi
    [ $# -eq 2 ] || { usage; exit 2; }
    { is_num "$MAX_COST_RATIO" && is_num "$COST_MIN_CALLS"; } || { echo "閾値が数でない: $MAX_COST_RATIO / $COST_MIN_CALLS" >&2; exit 2; }
    command -v jq >/dev/null || { echo "jq が必要です" >&2; exit 2; }
    decide "$2"
    exit 0 ;;
  --summarize)
    [ $# -eq 6 ] && [ "$3" = --new ] && [ "$5" = --old ] || { usage; exit 2; }
    command -v jq >/dev/null || { echo "jq が必要です" >&2; exit 2; }
    summarize "$2" "$4" "$6"
    exit 0 ;;
esac

# ---- 引数 --------------------------------------------------------------------------------------
ENV_NAME=""
SERVICE=""
EXECUTE=0
TO_REV=""
FROM_REV=""
STEPS="5,25,100"
WAIT=300
MAX_EXTENDS=2
ON_INSUFFICIENT=rollback
METRICS_DIR=""
while [ $# -gt 0 ]; do
  case "$1" in
    --env) ENV_NAME="${2:?--env stg|prod}"; shift 2 ;;
    --service) SERVICE="${2:?--service NAME}"; shift 2 ;;
    --execute) EXECUTE=1; shift ;;
    --dry-run) EXECUTE=0; shift ;;
    --to-revision) TO_REV="${2:?--to-revision REV}"; shift 2 ;;
    --from-revision) FROM_REV="${2:?--from-revision REV}"; shift 2 ;;
    --steps) STEPS="${2:?--steps 5,25,100}"; shift 2 ;;
    --wait) WAIT="${2:?--wait SEC}"; shift 2 ;;
    --max-extends) MAX_EXTENDS="${2:?--max-extends N}"; shift 2 ;;
    --on-insufficient) ON_INSUFFICIENT="${2:?--on-insufficient rollback|promote}"; shift 2 ;;
    --metrics-dir) METRICS_DIR="${2:?--metrics-dir DIR}"; shift 2 ;;
    --max-cost-ratio) MAX_COST_RATIO="${2:?--max-cost-ratio N}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage; exit 2 ;;
  esac
done
case "$ENV_NAME" in
  stg|prod) ;;
  "") echo "--env が要る(stg か prod)" >&2; usage; exit 2 ;;
  *) echo "--env は stg か prod: $ENV_NAME" >&2; exit 2 ;;
esac
[ -n "$SERVICE" ] || { echo "--service が要る(appconfig / ai など)" >&2; usage; exit 2; }
case "$SERVICE" in *-stg|*-prod) ;; *) SERVICE="${SERVICE}-${ENV_NAME}" ;; esac
case "$SERVICE" in
  *-"$ENV_NAME") ;;
  *) echo "--service $SERVICE は --env $ENV_NAME のサービスでない" >&2; exit 2 ;;
esac
is_int "$WAIT" || { echo "--wait は秒(整数)" >&2; exit 2; }
is_int "$MAX_EXTENDS" || { echo "--max-extends は整数" >&2; exit 2; }
case "$ON_INSUFFICIENT" in rollback|promote) ;; *) echo "--on-insufficient は rollback か promote" >&2; exit 2 ;; esac
[ -z "$METRICS_DIR" ] || [ "$EXECUTE" = 0 ] || { echo "--metrics-dir は dry-run だけ(模擬)" >&2; exit 2; }
for v in "$MAX_ERROR_RATE" "$MAX_ERROR_RATE_DELTA" "$MAX_P95_RATIO" "$P95_SLACK_MS" "$MAX_CLAUDE_FAILURE_RATE" \
         "$CLAUDE_FAILURE_MIN_COUNT" "$MIN_REQUESTS" "$MAX_COST_RATIO" "$COST_MIN_CALLS"; do
  is_num "$v" || { echo "閾値が数でない: $v" >&2; exit 2; }
done
STEP_LIST="$(parse_steps "$STEPS")" || exit 2
command -v jq >/dev/null || { echo "jq が必要です" >&2; exit 2; }

PROJECT_ID="${PROJECT_ID:-jphacks-kz2601}"
REGION="${REGION:-asia-northeast1}"
GC=(--project="$PROJECT_ID" --region="$REGION")

# prod の誤操作を防ぐ: prod で実行するのは --env prod と --execute を両方書いた時だけ。
# サービス名の接尾辞と --env が食い違う指定(--env stg --service appconfig-prod)は上で止めている。
if [ "$EXECUTE" = 1 ]; then
  command -v gcloud >/dev/null || { echo "gcloud が必要です" >&2; exit 2; }
  echo "EXECUTE: $SERVICE ($ENV_NAME) project=$PROJECT_ID region=$REGION steps=$(echo "$STEP_LIST" | paste -sd, -)" >&2
else
  echo "DRY-RUN: コマンドを出すだけ(GCP には触らない)。実行は --execute" >&2
fi

TMP="$(mktemp -d)"
# shellcheck disable=SC2329 # trap から呼ぶ
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

# 実行するコマンド: dry-run は出すだけ、--execute は出してから実行する
quote() { # そのまま貼れる形: 安全な文字だけなら素のまま、それ以外は '…'
  case "$1" in
    *[!A-Za-z0-9_./=,:@%+-]*|'') printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")" ;;
    *) printf '%s' "$1" ;;
  esac
}
show() { printf '+'; for a in "$@"; do printf ' %s' "$(quote "$a")"; done; printf '\n'; }
run() {
  show "$@"
  [ "$EXECUTE" = 1 ] || return 0
  "$@"
}

notify() { # notify <見出し> <本文>
  echo "[$1] $2" >&2
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    printf '### rollout %s: %s\n\n%s\n' "$SERVICE" "$1" "$2" >>"$GITHUB_STEP_SUMMARY"
  fi
  # GitHub Actions で実際に戻した時は注釈(run の画面・Checks に出る)にもする。ジョブも失敗するので、いつもの失敗通知も飛ぶ
  if [ "${GITHUB_ACTIONS:-}" = true ] && [ "$EXECUTE" = 1 ] && [ "$1" = "rolled back" ]; then
    echo "::error title=rollout ${SERVICE} rolled back::$(printf '%s' "$2" | sed 's/%/%25/g' | tr '\n' ' ')"
  fi
}

# ---- 新旧のリビジョン ---------------------------------------------------------------------------
if [ -z "$TO_REV" ] || [ -z "$FROM_REV" ]; then
  if [ "$EXECUTE" = 1 ]; then
    gcloud run services describe "$SERVICE" "${GC[@]}" --format=json >"$TMP/svc.json"
    # 新 = 最後に作ったリビジョン(deploy --no-traffic の結果)。旧 = 今いちばん多く受けているリビジョン
    [ -n "$TO_REV" ] || TO_REV="$(jq -r '.status.latestCreatedRevisionName // empty' "$TMP/svc.json")"
    [ -n "$FROM_REV" ] || FROM_REV="$(jq -r '[.status.traffic[]? | select(.revisionName != null)] | max_by(.percent // 0) | .revisionName // empty' "$TMP/svc.json")"
  else
    show gcloud run services describe "$SERVICE" "${GC[@]}" --format=json
    [ -n "$TO_REV" ] || TO_REV="<NEW_REVISION>"
    [ -n "$FROM_REV" ] || FROM_REV="<OLD_REVISION>"
  fi
fi
[ -n "$TO_REV" ] && [ -n "$FROM_REV" ] || { echo "新旧のリビジョンが読めない(--to-revision / --from-revision で指定)" >&2; exit 2; }
[ "$TO_REV" != "$FROM_REV" ] || { echo "新と旧が同じリビジョン: $TO_REV(もう 100% か、--no-traffic で deploy していない)" >&2; exit 2; }
echo "new=$TO_REV old=$FROM_REV" >&2

rollback() { # rollback <理由>
  trap - INT TERM
  run gcloud run services update-traffic "$SERVICE" "${GC[@]}" --to-revisions="${FROM_REV}=100" --quiet
  notify "rolled back" "$SERVICE を $FROM_REV に 100% 戻した(新 $TO_REV)。理由: $1"
}
# 人が途中で止める: Ctrl-C / ジョブの取り消し(SIGINT → SIGTERM)で旧に 100% 戻して終わる
# shellcheck disable=SC2329 # trap から呼ぶ
on_signal() { echo "中断: 旧のリビジョンに戻す" >&2; rollback "人が止めた(signal)"; exit 3; }
trap on_signal INT TERM

log_filter() { # log_filter <窓の始まり RFC3339>
  printf '%s' "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${SERVICE}\" AND resource.labels.revision_name=(\"${TO_REV}\" OR \"${FROM_REV}\") AND timestamp>=\"$1\" AND (jsonPayload.event=\"http.request\" OR jsonPayload.event=\"ai.usage\")"
}
# 段階の窓の指標を <出力> に書く。失敗は 0 以外を返す(if の中で呼ぶので set -e は効かない。1 つずつ見る)
fetch_metrics() { # fetch_metrics <割合> <窓の始まり RFC3339> <出力>
  if [ -n "$METRICS_DIR" ]; then
    cp "$METRICS_DIR/$1.json" "$3" || return 1
    return 0
  fi
  gcloud logging read "$(log_filter "$2")" --project="$PROJECT_ID" --format=json --limit="${LIMIT:-20000}" >"$TMP/logs.json" || return 1
  summarize "$TMP/logs.json" "$TO_REV" "$FROM_REV" >"$3" || return 1
}

now_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }
# sleep を裏で動かして wait する: 待っている間の SIGINT / SIGTERM ですぐ trap(巻き戻し)に入るように
pause() { if [ "$EXECUTE" = 1 ]; then sleep "$1" & wait "$!"; else echo "+ sleep $1"; fi; }

# ---- 段階を進める --------------------------------------------------------------------------------
for pct in $STEP_LIST; do
  if [ "$pct" = 100 ]; then
    run gcloud run services update-traffic "$SERVICE" "${GC[@]}" --to-revisions="${TO_REV}=100" --quiet
  else
    run gcloud run services update-traffic "$SERVICE" "${GC[@]}" --to-revisions="${TO_REV}=${pct},${FROM_REV}=$((100 - pct))" --quiet
  fi
  start="$(now_utc)"
  extends=0
  while :; do
    pause "$WAIT"
    if [ "$EXECUTE" = 0 ] && [ -z "$METRICS_DIR" ]; then
      show gcloud logging read "$(log_filter "$start")" --project="$PROJECT_ID" --format=json --limit="${LIMIT:-20000}"
      echo "# step ${pct}%: dry-run なので指標は読まずに次へ(--metrics-dir で模擬できる)"
      break
    fi
    # 指標が読めない・判定できない時は、悪い版かどうか分からないので戻す
    if ! fetch_metrics "$pct" "$start" "$TMP/metrics.json" || ! result="$(decide "$TMP/metrics.json")"; then
      rollback "step ${pct}%: 指標が読めない(ログの権限・API・入力を確かめる)"
      exit 3
    fi
    decision="$(printf '%s\n' "$result" | sed -n 's/^decision=//p')"
    reasons="$(printf '%s\n' "$result" | sed -n 's/^reason=//p' | paste -sd';' -)"
    echo "step ${pct}%: $decision — $reasons"
    case "$decision" in
      promote) break ;;
      rollback) rollback "step ${pct}%: $reasons"; exit 3 ;;
      insufficient)
        if [ "$extends" -lt "$MAX_EXTENDS" ]; then
          extends=$((extends + 1))
          echo "step ${pct}%: 指標が足りないので窓を延ばす ($extends/$MAX_EXTENDS)"
          continue
        fi
        if [ "$ON_INSUFFICIENT" = promote ]; then
          echo "step ${pct}%: 指標が足りないまま進める(--on-insufficient promote)"
          break
        fi
        rollback "step ${pct}%: $reasons(窓を $MAX_EXTENDS 回延ばしても足りない)"
        exit 4 ;;
      *) echo "判定が読めない: $result" >&2; rollback "判定が読めない"; exit 3 ;;
    esac
  done
done
trap - INT TERM
if [ "$EXECUTE" = 1 ]; then
  notify "done" "$SERVICE は $TO_REV に 100%(段階: $(echo "$STEP_LIST" | paste -sd, -))"
else
  echo "dry-run: 終わり(何も変えていない)" >&2
fi
exit 0
