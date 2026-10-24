#!/usr/bin/env bash
# デモ用: docs/dsl/examples の kakeibo / training / walk を「探す」(public_apps)に公開する(#50)。
# 何度実行しても同じ appId(catalog-<name>)を上書き公開するだけ(installs は保持)。
# 使い方:
#   ローカル(AUTH_MODE=dev):  scripts/seed-public-apps.sh http://localhost:8080
#   staging(Firebase):       FIREBASE_WEB_API_KEY=<key> scripts/seed-public-apps.sh https://appconfig-stg-...run.app
# Firebase の場合は匿名ユーザーを 1 つ作り、その refresh token を ~/.tailorit-seed-token に保存して次回も同じ uid で公開する
# (uid が変わると前回の掲載の所有者と一致せず 403 になるため)。SEED_TOKEN を渡せばそれを Bearer に使う。
set -euo pipefail

BASE="${1:-http://localhost:8080}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
AUTHOR="${SEED_AUTHOR:-Tailor It チーム}"
STATE="${SEED_STATE:-$HOME/.tailorit-seed-token}"

token() {
  if [ -n "${SEED_TOKEN:-}" ]; then echo "$SEED_TOKEN"; return; fi
  if [ -z "${FIREBASE_WEB_API_KEY:-}" ]; then echo "${SEED_UID:-catalog-curator}"; return; fi
  local res
  if [ -s "$STATE" ]; then
    res="$(curl -fsS -X POST "https://securetoken.googleapis.com/v1/token?key=$FIREBASE_WEB_API_KEY" \
      -d "grant_type=refresh_token&refresh_token=$(cat "$STATE")")"
    echo "$res" | jq -r .refresh_token >"$STATE"
    echo "$res" | jq -r .id_token
  else
    res="$(curl -fsS -X POST -H 'Content-Type: application/json' -d '{"returnSecureToken":true}' \
      "https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=$FIREBASE_WEB_API_KEY")"
    (umask 077; echo "$res" | jq -r .refreshToken >"$STATE")
    echo "$res" | jq -r .idToken
  fi
}
TOKEN="$(token)"
H=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')

publish() { # name description tags(JSON array)
  local name="$1" app="catalog-$1" ver
  ver="$(curl -fsS "${H[@]}" "$BASE/v1/apps/$app/dsl" | jq -r .version)"
  jq -n --slurpfile d "$ROOT/docs/dsl/examples/$name.json" \
    '{dsl:$d[0],source:{kind:"manual",mode:"full",summary:"探す用のサンプル"}}' \
    | curl -fsS -X PUT "${H[@]}" -H "If-Match: \"$ver\"" --data-binary @- "$BASE/v1/apps/$app/dsl" >/dev/null
  jq -n --arg d "$2" --arg a "$AUTHOR" --argjson t "$3" '{description:$d,tags:$t,authorName:$a}' \
    | curl -fsS -X POST "${H[@]}" --data-binary @- "$BASE/v1/apps/$app/publish" \
    | jq -r '"published: \(.id)  \(.name)  installs=\(.installs)  parts=\(.components|join(","))"'
}

publish kakeibo  "毎日の支出をサッと記録。今月の合計とカテゴリ別グラフ、レシート写真も" '["お金","節約","記録"]'
publish training "腕立て・スクワットの回数を毎日記録。朝 8 時にリマインド" '["健康","筋トレ","習慣"]'
publish walk     "散歩で見つけたいい場所を写真とひとことで残す" '["散歩","写真","メモ"]'
curl -fsS "${H[@]}" "$BASE/v1/public-apps?sort=popular" | jq -r '"catalog: \(.items|length) apps"'
