#!/usr/bin/env bash
# G13 Day2 (#82) のリアルタイム通知を curl だけで確かめる。
#   u1 が public entity を持つアプリを作る → u2 が GET /events を購読 → u1 が投稿 → u2 に records イベントが届くまでの秒数を出す。
# 使い方:
#   ローカル(AUTH_MODE=dev): scripts/realtime-smoke.sh http://localhost:8080
#   staging(Firebase):      U1_TOKEN=<idToken> U2_TOKEN=<idToken> scripts/realtime-smoke.sh https://appconfig-stg-...run.app
set -euo pipefail

BASE="${1:-http://localhost:8080}"
U1="${U1_TOKEN:-rt-user-1}"
U2="${U2_TOKEN:-rt-user-2}"
APP="RT-SMOKE-$(date +%s)"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$(mktemp -d)"

# kakeibo の expense を public にした DSL
DSL="$(sed 's/"id": "expense",/"id": "expense", "scope": "public",/' "$ROOT/docs/dsl/examples/kakeibo.json")"

curl -fsS -H "Authorization: Bearer $U1" "$BASE/v1/apps/$APP/dsl" >/dev/null
printf '{"dsl":%s,"source":{"kind":"ai","mode":"full","summary":"smoke"}}' "$DSL" \
  | curl -fsS -X PUT -H "Authorization: Bearer $U1" -H 'If-Match: "1"' -H 'Content-Type: application/json' \
      --data-binary @- "$BASE/v1/apps/$APP/dsl" >/dev/null
curl -fsS -H "Authorization: Bearer $U2" "$BASE/v1/apps/$APP/dsl" >/dev/null # u2 が共有アプリを入れる

# u2 の購読(行ごとに受信時刻 ms を付ける)
curl -sN -H "Authorization: Bearer $U2" -H 'Accept: text/event-stream' "$BASE/v1/apps/$APP/events" \
  | while IFS= read -r line; do printf '%s %s\n' "$(python3 -c 'import time;print(int(time.time()*1000))')" "$line"; done \
  >"$OUT/events.log" &
LISTENER=$!
trap 'kill $LISTENER 2>/dev/null || true' EXIT

for _ in $(seq 1 50); do grep -q 'event: hello' "$OUT/events.log" 2>/dev/null && break; sleep 0.1; done
grep -q 'event: hello' "$OUT/events.log" || { echo "FAIL: hello が来ない"; cat "$OUT/events.log"; exit 1; }

SENT="$(python3 -c 'import time;print(int(time.time()*1000))')"
curl -fsS -X POST -H "Authorization: Bearer $U1" -H 'Content-Type: application/json' \
  -d '{"data":{"amount":1234,"memo":"realtime smoke"},"authorName":"u1"}' \
  "$BASE/v1/apps/$APP/entities/expense/records" >/dev/null

for _ in $(seq 1 100); do grep -q 'event: records' "$OUT/events.log" && break; sleep 0.1; done
GOT="$(grep -m1 'event: records' "$OUT/events.log" | cut -d' ' -f1 || true)"
cat "$OUT/events.log"
if [ -z "$GOT" ]; then echo "FAIL: records イベントが 10 秒以内に来ない"; exit 1; fi
echo "OK: app=$APP 投稿→通知 $((GOT - SENT)) ms"
