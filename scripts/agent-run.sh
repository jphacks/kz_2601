#!/usr/bin/env bash
# エージェントの PR の実行の上限を確かめる起動スクリプト(#370・#2290)。既定は dry-run(何も書かない・何も起動しない)。
#
#   scripts/agent-run.sh plan  [共通]                      agent:ready の Issue から、今始めてよいものと起動のコマンドを出す
#   scripts/agent-run.sh check <issue> --elapsed-min M --cost-usd C [共通]   1 本の時間・費用が上限を超えたか(超えたら止める)
#   scripts/agent-run.sh fail  <issue> --reason R [共通]   失敗の時に Issue に付ける決まった文を出す(R: timeout|cost|ci|error)
#
# 共通: --repo owner/name(既定 GITHUB_REPOSITORY)・--max-concurrent N(既定 1)・--timeout-min M(既定 30)・
#       --max-cost-usd C(既定 5)・--run-url URL(実行の場所)・--execute(書く・起動する)
#
# 数え方(ラベルは人が付けるものなので、このスクリプトは付けない・外さない):
#   - 始めた Issue: ブランチ agent/<番号>-* か、そのブランチからの PR(状態を問わず)があるか、止めた印のコメントがある
#   - 動いている Issue: 始めたうち、止めた印が無く、閉じた・マージした PR も無いもの
#   - 始めてよい Issue: agent:ready・開いている・まだ始めていない、を番号の小さい順に「上限 - 動いている数」件
# 同じ Issue に 2 回目は始めない。失敗したら印付きの決まった文を 1 回だけコメントして止まり、再試行しない。
# --execute の plan は AGENT_RUN_CMD <番号> を timeout の下で動かす(AGENT_MAX_COST_USD を渡す。終了コード 3 は費用の超過とみなす)。
# 終了コード: 0 済んだ / 1 gh・起動の失敗 / 2 引数の誤り / 3 check で上限を超えた
set -euo pipefail

MARK='<!-- agent-run:stopped -->'
READY='agent:ready'

usage() {
  sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}
die() { echo "error: $*" >&2; exit 2; }

cmd="${1:-plan}"
case "$cmd" in plan | check | fail) [ $# -eq 0 ] || shift ;; -h | --help) usage ;; -*) cmd=plan ;; *) usage ;; esac

repo="${GITHUB_REPOSITORY:-}" max_conc=1 timeout_min=30 max_cost=5 run_url="" execute=0
issue="" elapsed="" cost="" reason=""
while [ $# -gt 0 ]; do
  case "$1" in
    --repo) [ $# -ge 2 ] || usage; repo="$2"; shift 2 ;;
    --max-concurrent) [ $# -ge 2 ] || usage; max_conc="$2"; shift 2 ;;
    --timeout-min) [ $# -ge 2 ] || usage; timeout_min="$2"; shift 2 ;;
    --max-cost-usd) [ $# -ge 2 ] || usage; max_cost="$2"; shift 2 ;;
    --run-url) [ $# -ge 2 ] || usage; run_url="$2"; shift 2 ;;
    --elapsed-min) [ $# -ge 2 ] || usage; elapsed="$2"; shift 2 ;;
    --cost-usd) [ $# -ge 2 ] || usage; cost="$2"; shift 2 ;;
    --reason) [ $# -ge 2 ] || usage; reason="$2"; shift 2 ;;
    --execute) execute=1; shift ;;
    --dry-run) execute=0; shift ;;
    -h | --help) usage ;;
    -*) die "不明な引数: $1" ;;
    *) [ -z "$issue" ] || usage; issue="$1"; shift ;;
  esac
done

num() { [[ "$1" =~ ^[0-9]+([.][0-9]+)?$ ]]; }
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "--repo owner/name か GITHUB_REPOSITORY が要る: '$repo'"
[[ "$max_conc" =~ ^[1-9][0-9]*$ ]] || die "--max-concurrent は正の整数: $max_conc"
[[ "$timeout_min" =~ ^[1-9][0-9]*$ ]] || die "--timeout-min は正の整数: $timeout_min"
if ! num "$max_cost" || ! awk -v c="$max_cost" 'BEGIN { exit !(c > 0) }'; then die "--max-cost-usd は正の数: $max_cost"; fi
case "$run_url" in '' | https://*) ;; *) die "--run-url は https:// で始まる: $run_url" ;; esac
command -v jq >/dev/null || die "jq が要る"
if [ "$cmd" != plan ]; then
  [[ "$issue" =~ ^[1-9][0-9]*$ ]] || die "$cmd には Issue の番号が要る: '$issue'"
elif [ -n "$issue" ]; then
  die "plan は Issue の番号を取らない"
fi

mode=DRY-RUN
[ "$execute" -eq 1 ] && mode=EXECUTE

# 決まった文(エージェントの書いた文は載せない)
stop_text() { # issue reason
  local why
  case "$2" in
    timeout) why="時間の上限(${timeout_min} 分)を超えた" ;;
    cost) why="費用の上限(${max_cost} USD)を超えた" ;;
    ci) why="CI が直しの後も赤" ;;
    *) why="実行が失敗した" ;;
  esac
  printf '%s\n自動の修正は止めました。人が引き取ります。\n\n- 理由: %s\n- 実行の場所: %s\n' "$MARK" "$why" "${run_url:-(記録なし)}"
}

has_stop_mark() { # issue -> 0 if the issue already has the stop comment
  local bodies
  bodies="$(gh api --paginate "repos/$repo/issues/$1/comments" --jq '.[].body')" || { echo "error: #$1 のコメントを読めない" >&2; exit 1; }
  grep -qF -- "$MARK" <<<"$bodies"
}

do_fail() { # issue reason
  case "$2" in timeout | cost | ci | error) ;; *) die "--reason は timeout|cost|ci|error: '$2'" ;; esac
  if has_stop_mark "$1"; then
    echo "$mode #$1: もう止めた印がある。2 回目はコメントしない"
    return 0
  fi
  local body
  body="$(stop_text "$1" "$2")"
  echo "$mode #$1: 止める($2)。Issue にコメントする:"
  printf '%s\n' "$body" | sed 's/^/  | /'
  if [ "$execute" -eq 1 ]; then
    jq -n --arg b "$body" '{body: $b}' | gh api -X POST "repos/$repo/issues/$1/comments" --input - >/dev/null \
      || { echo "error: #$1 にコメントできない" >&2; exit 1; }
  fi
}

case "$cmd" in
  fail)
    [ -n "$reason" ] || die "fail には --reason が要る"
    do_fail "$issue" "$reason"
    exit 0
    ;;
  check)
    num "$elapsed" || die "check には --elapsed-min(数)が要る"
    num "$cost" || die "check には --cost-usd(数)が要る"
    over=""
    if awk -v e="$elapsed" -v m="$timeout_min" 'BEGIN { exit !(e > m) }'; then over=timeout
    elif awk -v c="$cost" -v m="$max_cost" 'BEGIN { exit !(c > m) }'; then over=cost
    fi
    if [ -z "$over" ]; then
      echo "OK #$issue: ${elapsed} 分 / ${timeout_min} 分・${cost} USD / ${max_cost} USD"
      exit 0
    fi
    echo "$mode #$issue: 上限を超えた(${elapsed} 分 / ${timeout_min} 分・${cost} USD / ${max_cost} USD)。止める"
    do_fail "$issue" "$over"
    exit 3
    ;;
esac

# plan
ready="$(gh issue list --repo "$repo" --label "$READY" --state open --limit 200 --json number --jq '.[].number')" \
  || { echo "error: $READY の Issue を読めない" >&2; exit 1; }
prs="$(gh pr list --repo "$repo" --state all --limit 500 --json headRefName,state \
  --jq '.[] | select(.headRefName | test("^agent/[0-9]+-")) | "\(.headRefName | capture("^agent/(?<n>[0-9]+)-").n) \(.state)"')" \
  || { echo "error: PR を読めない" >&2; exit 1; }
branches="$(gh api --paginate "repos/$repo/branches" --jq '.[].name | select(test("^agent/[0-9]+-")) | capture("^agent/(?<n>[0-9]+)-").n')" \
  || { echo "error: ブランチを読めない" >&2; exit 1; }

# 始めた番号(ブランチか PR がある)と、閉じた・マージした PR の番号
started="$( { printf '%s\n' "$branches"; awk '{ print $1 }' <<<"$prs"; } | grep -E '^[0-9]+$' | sort -un || true)"
finished="$(awk '$2 == "CLOSED" || $2 == "MERGED" { print $1 }' <<<"$prs" | sort -un || true)"

running=()
for n in $started; do
  grep -qx "$n" <<<"$finished" && continue
  has_stop_mark "$n" && continue
  running+=("$n")
done
free=$((max_conc - ${#running[@]}))
[ "$free" -ge 0 ] || free=0

starts=() skipped=()
while read -r n; do
  [ -n "$n" ] || continue
  if grep -qx "$n" <<<"$started"; then
    skipped+=("#$n(もう始めた)")
  elif has_stop_mark "$n"; then
    skipped+=("#$n(止めた印がある)")
  elif [ "${#starts[@]}" -lt "$free" ]; then
    starts+=("$n")
  else
    skipped+=("#$n(上限で次回)")
  fi
done < <(sort -n <<<"$ready")

running_list="なし"
[ ${#running[@]} -eq 0 ] || running_list="$(printf '#%s ' "${running[@]}")"
echo "$mode ($repo) 動いている ${#running[@]} / 上限 $max_conc: $running_list"
[ ${#skipped[@]} -eq 0 ] || echo "始めない: ${skipped[*]}"
if [ ${#starts[@]} -eq 0 ]; then
  echo "始める Issue なし"
  exit 0
fi
agent="${AGENT_RUN_CMD:-<AGENT_RUN_CMD>}"
for n in "${starts[@]}"; do
  echo "START #$n: AGENT_MAX_COST_USD=$max_cost timeout --kill-after=1m ${timeout_min}m $agent $n"
done
[ "$execute" -eq 1 ] || exit 0

[ -n "${AGENT_RUN_CMD:-}" ] || die "--execute には AGENT_RUN_CMD(エージェントを 1 本起動するコマンド)が要る"
read -r -a agent_cmd <<<"$AGENT_RUN_CMD"
for n in "${starts[@]}"; do
  set +e
  AGENT_MAX_COST_USD="$max_cost" timeout --kill-after=1m "${timeout_min}m" "${agent_cmd[@]}" "$n"
  code=$?
  set -e
  case "$code" in
    0) echo "DONE #$n" ;;
    124 | 137) do_fail "$n" timeout ;;
    3) do_fail "$n" cost ;;
    *) do_fail "$n" error ;;
  esac
done
