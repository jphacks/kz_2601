#!/usr/bin/env bash
# scripts/agent-run.sh を作り物の gh・timeout・エージェントで確かめる(GitHub にもエージェントにも触らない)。
# 上限(同時の数)・二重に始めない・失敗の後に止まる(印付きの文を 1 回だけ・再試行しない)・既定の dry-run。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/agent-run.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }

# 作り物の gh: 入力は $TMP/gh/ready(番号の行)・prs(「番号 状態」の行)・branches(番号の行)・
# comments-<番号>(本文。無ければ空)。呼ばれた引数は $TMP/calls、コメントの本文は $TMP/posted-<番号>
mkdir -p "$TMP/bin" "$TMP/gh"
cat >"$TMP/bin/gh" <<EOF
#!/usr/bin/env bash
echo "\$*" >>"$TMP/calls"
D="$TMP/gh"
case "\$*" in
  "issue list --repo o/r --label agent:ready --state open "*) cat "\$D/ready" ;;
  "pr list --repo o/r --state all "*) cat "\$D/prs" ;; # --jq の後の形(番号 状態)
  "api --paginate repos/o/r/branches --jq "*) cat "\$D/branches" ;;
  "api --paginate repos/o/r/issues/"*"/comments --jq "*)
    n="\${3#repos/o/r/issues/}"; n="\${n%/comments}"; cat "\$D/comments-\$n" 2>/dev/null || true ;;
  "api -X POST repos/o/r/issues/"*"/comments --input -")
    n="\${4#repos/o/r/issues/}"; n="\${n%/comments}"; cat >"$TMP/posted-\$n"; cat "$TMP/posted-\$n" | jq -r .body >>"\$D/comments-\$n" ;;
  *) echo "fake gh: unexpected \$*" >&2; exit 1 ;;
esac
EOF
# 作り物の timeout: 「--kill-after=1m 30m コマンド...」の時間を記録してそのまま動かす
cat >"$TMP/bin/timeout" <<EOF
#!/usr/bin/env bash
echo "timeout \$1 \$2 cost=\$AGENT_MAX_COST_USD" >>"$TMP/calls"
shift 2
exec "\$@"
EOF
# 作り物のエージェント: $TMP/agent-exit-<番号> の終了コードで終わる(既定 0)
cat >"$TMP/bin/fake-agent" <<EOF
#!/usr/bin/env bash
echo "agent \$1" >>"$TMP/calls"
exit "\$(cat "$TMP/agent-exit-\$1" 2>/dev/null || echo 0)"
EOF
chmod +x "$TMP/bin/gh" "$TMP/bin/timeout" "$TMP/bin/fake-agent"
run() { set +e; OUT="$(PATH="$TMP/bin:$PATH" GITHUB_REPOSITORY=o/r "$SCRIPT" "$@" 2>&1)"; CODE=$?; set -e; }
reset() { rm -f "$TMP"/gh/* "$TMP"/posted-* "$TMP"/agent-exit-*; : >"$TMP/calls"; touch "$TMP/gh/ready" "$TMP/gh/prs" "$TMP/gh/branches"; }

# 1. 上限: 同時 1 本。#5 は PR が開いている(動いている)→ 何も始めない
reset
printf '%s\n' 3 5 9 >"$TMP/gh/ready"
printf '%s\n' "5 OPEN" "2 MERGED" >"$TMP/gh/prs"
printf '%s\n' 5 >"$TMP/gh/branches"
run plan
[ "$CODE" = 0 ] || fail "plan の終了コード $CODE"
has "DRY-RUN (o/r) 動いている 1 / 上限 1: #5"
has "#3(上限で次回)"
has "#5(もう始めた)"
has "始める Issue なし"
hasnt "START"

# 上限 3 なら番号の小さい順に 2 本(#3・#9)。コマンドに時間と費用の上限が入る
run plan --max-concurrent 3 --timeout-min 20 --max-cost-usd 2.5
has "動いている 1 / 上限 3"
has "START #3: AGENT_MAX_COST_USD=2.5 timeout --kill-after=1m 20m <AGENT_RUN_CMD> 3"
has "START #9:"
hasnt "START #5"
grep -q -- "-X POST" "$TMP/calls" && fail "dry-run で書いた"
grep -q "^agent " "$TMP/calls" && fail "dry-run で起動した"

# 2. 二重に始めない: マージ済み・閉じた PR やブランチだけある Issue、止めた印のある Issue は始めない
reset
printf '%s\n' 2 4 6 8 >"$TMP/gh/ready"
printf '%s\n' "2 MERGED" "4 CLOSED" >"$TMP/gh/prs"
printf '%s\n' 6 >"$TMP/gh/branches"
printf '%s\n' '<!-- agent-run:stopped -->' '自動の修正は止めました。人が引き取ります。' >"$TMP/gh/comments-6"
printf '%s\n' '<!-- agent-run:stopped -->' >"$TMP/gh/comments-8"
run plan --max-concurrent 5
has "動いている 0 / 上限 5: なし" # #6 はブランチがあるが止めた印がある
has "#2(もう始めた)"
has "#4(もう始めた)"
has "#6(もう始めた)"
has "#8(止めた印がある)"
has "始める Issue なし"

# 3. 失敗の後に止まる: --execute で起動し、時間切れ(124)・費用(3)・その他は決まった文を 1 回だけ付け、再試行しない
reset
printf '%s\n' 11 12 13 14 >"$TMP/gh/ready"
echo 124 >"$TMP/agent-exit-11"
echo 3 >"$TMP/agent-exit-12"
echo 1 >"$TMP/agent-exit-13"
run plan --max-concurrent 4 --execute
[ "$CODE" = 2 ] || fail "AGENT_RUN_CMD 無しの --execute は 2 のはずが $CODE"
grep -q "^agent " "$TMP/calls" && fail "AGENT_RUN_CMD 無しで起動した"
set +e
OUT="$(PATH="$TMP/bin:$PATH" GITHUB_REPOSITORY=o/r AGENT_RUN_CMD=fake-agent "$SCRIPT" plan --max-concurrent 4 --timeout-min 30 --max-cost-usd 5 \
  --run-url https://github.com/o/r/actions/runs/9 --execute 2>&1)"; CODE=$?
set -e
[ "$CODE" = 0 ] || fail "--execute の終了コード $CODE"
has "EXECUTE (o/r) 動いている 0 / 上限 4"
has "DONE #14"
grep -q "^timeout --kill-after=1m 30m cost=5$" "$TMP/calls" || fail "timeout に上限が渡っていない: $(cat "$TMP/calls")"
for n in 11 12 13; do
  [ -f "$TMP/posted-$n" ] || fail "#$n にコメントしていない"
  jq -e '.body | startswith("<!-- agent-run:stopped -->\n自動の修正は止めました。人が引き取ります。")' "$TMP/posted-$n" >/dev/null || fail "#$n の文"
done
[ ! -f "$TMP/posted-14" ] || fail "成功した #14 にコメントした"
jq -e '.body | contains("時間の上限(30 分)を超えた") and contains("https://github.com/o/r/actions/runs/9")' "$TMP/posted-11" >/dev/null || fail "#11 の理由"
jq -e '.body | contains("費用の上限(5 USD)を超えた")' "$TMP/posted-12" >/dev/null || fail "#12 の理由"
jq -e '.body | contains("実行が失敗した")' "$TMP/posted-13" >/dev/null || fail "#13 の理由"
[ "$(grep -c '^agent ' "$TMP/calls")" = 4 ] || fail "起動は 1 Issue 1 回のはず: $(cat "$TMP/calls")"
# 次の回: 止めた #11-13 は始めない(エージェントの PR は作り物なので #14 もブランチ無しのまま残る)
: >"$TMP/calls"
run plan --max-concurrent 4
has "#11(止めた印がある)"
has "#12(止めた印がある)"
has "#13(止めた印がある)"
has "START #14:"
# 2 回目の fail はコメントしない
rm -f "$TMP/posted-11"
run fail 11 --reason ci --execute
has "もう止めた印がある"
[ ! -f "$TMP/posted-11" ] || fail "2 回目にコメントした"

# 4. check: 上限の内なら 0、超えたら 3 で止める文(dry-run では出すだけ)
reset
run check 21 --elapsed-min 10 --cost-usd 1
[ "$CODE" = 0 ] || fail "上限の内の終了コード $CODE"
has "OK #21"
run check 21 --elapsed-min 31 --cost-usd 1
[ "$CODE" = 3 ] || fail "時間超過の終了コード 3 のはずが $CODE"
has "DRY-RUN #21: 止める(timeout)"
has "  | 自動の修正は止めました。人が引き取ります。"
[ ! -f "$TMP/posted-21" ] || fail "dry-run でコメントした"
run check 21 --elapsed-min 5 --cost-usd 5.01
[ "$CODE" = 3 ] || fail "費用超過の終了コード $CODE"
has "止める(cost)"

# 5. 引数の誤りは 2
for args in "bogus" "plan 7" "check" "check 7 --elapsed-min x --cost-usd 1" "fail 7" "fail 7 --reason maybe" \
  "plan --max-concurrent 0" "plan --max-cost-usd 0" "plan --run-url http://x" "plan --nope"; do
  # shellcheck disable=SC2086 # 引数を分けて渡す
  run $args
  [ "$CODE" = 2 ] || fail "引数 '$args' の終了コード 2 のはずが $CODE"
done

echo "ok: agent-run"
