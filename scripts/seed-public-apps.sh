#!/usr/bin/env bash
# デモ用: docs/dsl/examples の kakeibo / training / walk を「探す」(public_apps)に公開する(#50)。
# 何度実行しても同じ appId(catalog-<name>)を上書き公開するだけ(installs は保持)。
# 使い方:
#   ローカル(AUTH_MODE=dev):  scripts/seed-public-apps.sh http://localhost:8080
#   staging(Firebase):       FIREBASE_WEB_API_KEY=<key> scripts/seed-public-apps.sh https://appconfig-stg-...run.app
#   送る要求を見るだけ:        scripts/seed-public-apps.sh --dry-run https://appconfig-stg-...run.app
# Firebase の場合は匿名ユーザーを 1 つ作り、その refresh token を ~/.tailorit-seed-token に保存して次回も同じ uid で公開する
# (uid が変わると前回の掲載の所有者と一致せず 403 になるため)。SEED_TOKEN を渡せばそれを Bearer に使う。
# 止める入力(終了コード 2): URL が無い・http(s):// でない・localhost 以外への http・ホスト名に prod を含む(SEED_ALLOW_PROD=1 で通す)
# --dry-run: メソッド・パス・本文の要約を出すだけ。トークンも取らず、何も送らない
set -euo pipefail

usage() { echo "usage: $0 [--dry-run] <appconfig URL>" >&2; exit 2; }
DRY=0
BASE=""
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY=1; shift ;;
    -*) usage ;;
    *) [ -z "$BASE" ] || usage; BASE="${1%/}"; shift ;;
  esac
done
[ -n "$BASE" ] || { echo "appconfig の URL を渡す(例: http://localhost:8080)" >&2; usage; }
case "$BASE" in
  http://*|https://*) ;;
  *) echo "URL は http:// か https:// で始める: $BASE" >&2; exit 2 ;;
esac
host="${BASE#*://}"; host="${host%%/*}"
case "$host" in \[*\]*) host="${host%%]*}]" ;; *) host="${host%%:*}" ;; esac
case "$BASE" in
  http://*)
    case "$host" in
      localhost|127.0.0.1|\[::1\]) ;;
      *) echo "http は手元(localhost)だけ。$host には https を使う(トークンを平文で送らない)" >&2; exit 2 ;;
    esac ;;
esac
case "$host" in
  *prod*)
    if [ "${SEED_ALLOW_PROD:-}" != 1 ]; then
      echo "$host は本番らしい。デモのサンプルを本番の「探す」に載せるなら SEED_ALLOW_PROD=1 を付ける" >&2
      exit 2
    fi ;;
esac

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

if [ "$DRY" = 1 ]; then
  # トークンの中身は出さない。どこから取るかだけ
  if [ -n "${SEED_TOKEN:-}" ]; then auth="SEED_TOKEN"
  elif [ -n "${FIREBASE_WEB_API_KEY:-}" ]; then auth="Firebase の匿名ユーザー($STATE)"
  else auth="dev の uid ${SEED_UID:-catalog-curator}"
  fi
  echo "DRY-RUN: $BASE に送る要求(送らない)・Bearer=$auth"
else
  TOKEN="$(token)"
  H=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')
fi

publish() { # name description tags(JSON array)
  local name="$1" app="catalog-$1" ver meta dsl="$ROOT/docs/dsl/examples/$1.json"
  meta="$(jq -cn --arg d "$2" --arg a "$AUTHOR" --argjson t "$3" '{description:$d,tags:$t,authorName:$a}')"
  if [ "$DRY" = 1 ]; then
    jq -e . "$dsl" >/dev/null || { echo "$dsl が JSON でない" >&2; exit 1; }
    echo "GET  /v1/apps/$app/dsl"
    echo "PUT  /v1/apps/$app/dsl  If-Match: <GET の version>  dsl=docs/dsl/examples/$name.json($(wc -c <"$dsl" | tr -d ' ') bytes) source.kind=manual"
    echo "POST /v1/apps/$app/publish  $meta"
    return
  fi
  ver="$(curl -fsS "${H[@]}" "$BASE/v1/apps/$app/dsl" | jq -r .version)"
  jq -n --slurpfile d "$dsl" \
    '{dsl:$d[0],source:{kind:"manual",mode:"full",summary:"探す用のサンプル"}}' \
    | curl -fsS -X PUT "${H[@]}" -H "If-Match: \"$ver\"" --data-binary @- "$BASE/v1/apps/$app/dsl" >/dev/null
  printf '%s' "$meta" \
    | curl -fsS -X POST "${H[@]}" --data-binary @- "$BASE/v1/apps/$app/publish" \
    | jq -r '"published: \(.id)  \(.name)  installs=\(.installs)  parts=\(.components|join(","))"'
}

publish kakeibo  "毎日の支出をサッと記録。今月の合計とカテゴリ別グラフ、レシート写真も" '["お金","節約","記録"]'
publish training "腕立て・スクワットの回数を毎日記録。朝 8 時にリマインド" '["健康","筋トレ","習慣"]'
publish walk     "散歩で見つけたいい場所を写真とひとことで残す" '["散歩","写真","メモ"]'
if [ "$DRY" = 1 ]; then
  echo "GET  /v1/public-apps?sort=popular"
  exit 0
fi
curl -fsS "${H[@]}" "$BASE/v1/public-apps?sort=popular" | jq -r '"catalog: \(.items|length) apps"'
