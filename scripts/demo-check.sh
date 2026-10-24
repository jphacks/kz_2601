#!/usr/bin/env bash
# デモ前チェック兼ウォームアップ(#171)。staging / 本番の各機能を 1 回ずつ叩き、成否と所要時間を表で出す。
#   匿名ユーザー作成 → /health ×3(appconfig / ai を温める)→ 生成「家計簿アプリが欲しい」→ 対話「日記」
#   → 編集「集計のタブを足して」→ 探す一覧 → リアルタイム(自分のテスト用アプリで投稿 → SSE に届くか)
#   → 後片付け(レコード削除・テスト用アプリのアーカイブ・匿名ユーザー削除)
# Claude 呼び出しは生成・対話・編集の各 1 回(計 3 回)。探すは読むだけ。投稿はこのスクリプトが作ったアプリの中だけ。
# 使い方:
#   scripts/demo-check.sh stg
#   scripts/demo-check.sh prod        # 審査の 30 分前に 1 回
#   (cd backend && make demo-check ENV=stg)
# Firebase Web API キーは $METAMON_SECRETS_DIR/GoogleService-Info.plist(既定 ~/Desktop/JPHACKS2026/secrets)の API_KEY、
# または環境変数 FIREBASE_WEB_API_KEY。キーとトークンは画面・ログ・プロセス一覧に出さない。
# URL は APPCONFIG_URL / AI_URL で上書きできる。DEMO_CHECK_NO_AI=1 で生成・対話・編集を飛ばす(Claude を呼ばない再ウォームアップ用)。終了コードは失敗が 1 つでもあれば 1。
set -uo pipefail

ENV_NAME="${1:-}"
case "$ENV_NAME" in
  stg)
    APPCONFIG="${APPCONFIG_URL:-https://appconfig-stg-378953239426.asia-northeast1.run.app}"
    AI="${AI_URL:-https://ai-stg-378953239426.asia-northeast1.run.app}" ;;
  prod)
    APPCONFIG="${APPCONFIG_URL:-https://appconfig-prod-zgciuf3iha-an.a.run.app}"
    AI="${AI_URL:-https://ai-prod-zgciuf3iha-an.a.run.app}" ;;
  *) echo "usage: $0 <stg|prod>" >&2; exit 2 ;;
esac
for c in curl jq python3; do command -v "$c" >/dev/null || { echo "$c が必要です" >&2; exit 2; }; done

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SECRETS="${METAMON_SECRETS_DIR:-$HOME/Desktop/JPHACKS2026/secrets}"
TMP="$(umask 077; mktemp -d)"
STAMP="$(date +%s)"
GEN_APP="demochk-$ENV_NAME-$STAMP"
RT_APP="demochk-rt-$ENV_NAME-$STAMP"
LISTENER=""
trap '[ -n "$LISTENER" ] && kill "$LISTENER" 2>/dev/null; rm -rf "$TMP"' EXIT

ROWS=()   # "結果|確認|HTTP|ms|メモ"
FAILS=0
row() { ROWS+=("$1|$2|$3|$4|$5"); [ "$1" = "NG" ] && FAILS=$((FAILS + 1)); printf '  %-2s %s (%s ms) %s\n' "$1" "$2" "$4" "$5"; }
now_ms() { python3 -c 'import time;print(int(time.time()*1000))'; }

# --- API キー(画面に出さない) ---------------------------------------------------
KEY="${FIREBASE_WEB_API_KEY:-}"
if [ -z "$KEY" ] && [ -f "$SECRETS/GoogleService-Info.plist" ]; then
  KEY="$(/usr/libexec/PlistBuddy -c 'Print :API_KEY' "$SECRETS/GoogleService-Info.plist" 2>/dev/null || true)"
fi
[ -n "$KEY" ] || { echo "Firebase Web API キーが見つかりません($SECRETS/GoogleService-Info.plist か FIREBASE_WEB_API_KEY)" >&2; exit 2; }

# req METHOD URL [BODY_FILE] [AUTH=1] → $CODE $MS、本文は $TMP/body。URL とヘッダーは curl -K 経由でプロセス一覧に出さない。
req() {
  local method="$1" url="$2" body="${3:-}" auth="${4:-1}" max="${MAX_TIME:-20}" out
  {
    printf 'url = "%s"\n' "$url"
    printf 'request = "%s"\n' "$method"
    [ "$auth" = 1 ] && printf 'header = "Authorization: Bearer %s"\n' "$TOKEN"
    [ -n "$body" ] && printf 'header = "Content-Type: application/json"\ndata-binary = "@%s"\n' "$body"
    [ -n "${IF_MATCH:-}" ] && printf 'header = "If-Match: \\"%s\\""\n' "$IF_MATCH"
  } >"$TMP/curlrc"
  out="$(curl -sS -K "$TMP/curlrc" --max-time "$max" -o "$TMP/body" -w '%{http_code} %{time_total}' 2>"$TMP/err")" || true
  rm -f "$TMP/curlrc"
  CODE="${out%% *}"; CODE="${CODE:-000}"
  MS="$(awk -v t="${out##* }" 'BEGIN{printf "%d", t*1000}')"
}
jbody() { jq -r "$1" "$TMP/body" 2>/dev/null; }
reason() { jbody '.error.message // .message // .error // empty' | head -c 80; }

echo "== demo-check $ENV_NAME =="
echo "  appconfig: $APPCONFIG"
echo "  ai:        $AI"

# --- 1. 匿名ユーザー -------------------------------------------------------------
TOKEN=""
printf '{"returnSecureToken":true}' >"$TMP/signup.json"
req POST "https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=$KEY" "$TMP/signup.json" 0
if [ "$CODE" = 200 ]; then
  TOKEN="$(jbody .idToken)"; UID_="$(jbody .localId)"
  row OK "匿名ユーザー作成(accounts:signUp)" "$CODE" "$MS" "uid ${UID_:0:6}…"
else
  row NG "匿名ユーザー作成(accounts:signUp)" "$CODE" "$MS" "$(jbody '.error.message // empty')"
fi
rm -f "$TMP/body"

# --- 2. /health ×3(ウォームアップ) ------------------------------------------------
for svc in appconfig ai; do
  base="$APPCONFIG"; [ "$svc" = ai ] && base="$AI"
  times=(); ok=1; codes=""
  for _ in 1 2 3; do
    req GET "$base/health" "" 0
    times+=("$MS"); codes="$CODE"; [ "$CODE" = 200 ] || ok=0
  done
  [ "$ok" = 1 ] && r=OK || r=NG
  row "$r" "$svc GET /health ×3" "$codes" "${times[2]}" "1 回目 ${times[0]} / 2 回目 ${times[1]} / 3 回目 ${times[2]} ms"
done

if [ -z "$TOKEN" ]; then
  echo "トークンが無いので以降を省略します" >&2
else
  # --- 3. 生成 -------------------------------------------------------------------
  if [ "${DEMO_CHECK_NO_AI:-}" = 1 ]; then
    echo "  -- DEMO_CHECK_NO_AI=1: 生成・対話・編集を飛ばします(ai は /health のみ)"
  else
  req GET "$APPCONFIG/v1/apps/$GEN_APP/dsl"
  [ "$CODE" = 200 ] && row OK "appconfig GET /dsl(新規 seed)" "$CODE" "$MS" "version $(jbody .version)" \
                    || row NG "appconfig GET /dsl(新規 seed)" "$CODE" "$MS" "$(reason)"

  jq -n --arg a "$GEN_APP" '{appId:$a,text:"家計簿アプリが欲しい",mode:"create",locale:"ja"}' >"$TMP/gen.json"
  MAX_TIME=60 req POST "$AI/v1/generate" "$TMP/gen.json"
  if [ "$CODE" = 200 ] && [ "$(jbody .source)" = claude ]; then
    row OK "生成「家計簿アプリが欲しい」" "$CODE" "$MS" "$(jbody '"\(.mode) v\(.previousVersion)→\(.version) 「\(.dsl.app.name)」 画面\(.dsl.screens|length) Claude \(.latencyMs.claude // "-") ms"')"
  else
    row NG "生成「家計簿アプリが欲しい」" "$CODE" "$MS" "source=$(jbody '.source // empty') $(reason)"
  fi
  GEN_VER="$(jbody '.version // empty')"

  # --- 4. 対話 -------------------------------------------------------------------
  jq -n --arg a "$GEN_APP" '{appId:$a,messages:[{role:"user",text:"日記"}],locale:"ja"}' >"$TMP/chat.json"
  MAX_TIME=45 req POST "$AI/v1/chat" "$TMP/chat.json"
  kind="$(jbody '.kind // empty')"
  if [ "$CODE" = 200 ] && { [ "$kind" = question ] || [ "$kind" = ready ]; }; then
    row OK "対話「日記」" "$CODE" "$MS" "$kind: $(jbody .text | head -c 60)"
  else
    row NG "対話「日記」" "$CODE" "$MS" "kind=$kind $(reason)"
  fi
  req DELETE "$AI/v1/chat/$GEN_APP"
  case "$CODE" in 2*) row OK "対話の状態を消す(DELETE /v1/chat)" "$CODE" "$MS" "" ;;
                  *)  row NG "対話の状態を消す(DELETE /v1/chat)" "$CODE" "$MS" "$(reason)" ;; esac

  # --- 5. 編集 -------------------------------------------------------------------
  jq -n --arg a "$GEN_APP" '{appId:$a,text:"集計のタブを足して",mode:"edit",locale:"ja"}' >"$TMP/edit.json"
  MAX_TIME=60 req POST "$AI/v1/generate" "$TMP/edit.json"
  if [ "$CODE" = 200 ] && [ "$(jbody .source)" = claude ] && [ "$(jbody .version)" != "${GEN_VER:-x}" ]; then
    row OK "編集「集計のタブを足して」" "$CODE" "$MS" "$(jbody '"\(.mode) v\(.previousVersion)→\(.version) タブ\(.dsl.app.tabs // [] | length) Claude \(.latencyMs.claude // "-") ms"')"
  else
    row NG "編集「集計のタブを足して」" "$CODE" "$MS" "source=$(jbody '.source // empty') v$(jbody '.version // empty') $(reason)"
  fi

  fi

  # --- 6. 探す(読むだけ) ---------------------------------------------------------
  req GET "$APPCONFIG/v1/public-apps?sort=popular"
  if [ "$CODE" = 200 ]; then
    n="$(jbody '.items|length')"
    [ "${n:-0}" -gt 0 ] && r=OK || r=NG
    row "$r" "探す GET /public-apps" "$CODE" "$MS" "$n 件 $(jbody '[.items[:3][].name]|join(" / ")')"
  else
    row NG "探す GET /public-apps" "$CODE" "$MS" "$(reason)"
  fi

  # --- 7. リアルタイム(自分のテスト用アプリの中だけ) -------------------------------
  rt_ok=0
  req GET "$APPCONFIG/v1/apps/$RT_APP/dsl"
  if [ "$CODE" = 200 ]; then
    jq -n --slurpfile d "$ROOT/docs/dsl/examples/kakeibo.json" \
      '{dsl:$d[0],source:{kind:"manual",mode:"full",summary:"demo-check"}}' >"$TMP/rt.json"
    IF_MATCH="$(jbody .version)" req PUT "$APPCONFIG/v1/apps/$RT_APP/dsl" "$TMP/rt.json"
    case "$CODE" in 2*) rt_ok=1 ;; *) row NG "リアルタイム用アプリ作成(PUT /dsl)" "$CODE" "$MS" "$(reason)" ;; esac
  else
    row NG "リアルタイム用アプリ作成(GET /dsl)" "$CODE" "$MS" "$(reason)"
  fi
  if [ "$rt_ok" = 1 ]; then
    printf 'url = "%s"\nheader = "Authorization: Bearer %s"\nheader = "Accept: text/event-stream"\n' \
      "$APPCONFIG/v1/apps/$RT_APP/events" "$TOKEN" >"$TMP/sse.rc"
    curl -sN -K "$TMP/sse.rc" --max-time 30 2>/dev/null \
      | while IFS= read -r line; do printf '%s %s\n' "$(python3 -c 'import time;print(int(time.time()*1000))')" "$line"; done \
      >"$TMP/events.log" &
    LISTENER=$!
    for _ in $(seq 1 100); do grep -q 'event: hello' "$TMP/events.log" 2>/dev/null && break; sleep 0.1; done
    rm -f "$TMP/sse.rc"
    if ! grep -q 'event: hello' "$TMP/events.log" 2>/dev/null; then
      row NG "リアルタイム SSE 購読(hello)" "-" "-" "10 秒以内に hello が来ない"
    else
      printf '{"data":{"amount":1234,"category":"食費","memo":"demo-check"}}' >"$TMP/rec.json"
      SENT="$(now_ms)"
      req POST "$APPCONFIG/v1/apps/$RT_APP/entities/expense/records" "$TMP/rec.json"
      post_code="$CODE"
      for _ in $(seq 1 100); do grep -q 'event: records' "$TMP/events.log" && break; sleep 0.1; done
      GOT="$(grep -m1 'event: records' "$TMP/events.log" | cut -d' ' -f1)"
      if [ -n "$GOT" ]; then
        row OK "リアルタイム 投稿 → SSE records" "$post_code" "$((GOT - SENT))" "投稿から通知まで"
      else
        row NG "リアルタイム 投稿 → SSE records" "$post_code" "-" "10 秒以内に records が来ない"
      fi
    fi
    { pkill -P "$LISTENER"; kill "$LISTENER"; wait "$LISTENER"; } 2>/dev/null; LISTENER=""
  fi

  # --- 8. 後片付け ---------------------------------------------------------------
  if [ "$rt_ok" = 1 ]; then
    req DELETE "$APPCONFIG/v1/apps/$RT_APP/entities/expense/records?confirm=true"
    case "$CODE" in 2*) row OK "後片付け: テスト用レコード削除" "$CODE" "$MS" "" ;;
                    *)  row NG "後片付け: テスト用レコード削除" "$CODE" "$MS" "$(reason)" ;; esac
  fi
  apps=("$RT_APP"); [ "${DEMO_CHECK_NO_AI:-}" = 1 ] || apps=("$GEN_APP" "$RT_APP")
  for app in "${apps[@]}"; do
    req DELETE "$APPCONFIG/v1/apps/$app"
    case "$CODE" in 204|404) row OK "後片付け: アーカイブ $app" "$CODE" "$MS" "" ;;
                    *)       row NG "後片付け: アーカイブ $app" "$CODE" "$MS" "$(reason)" ;; esac
  done
  jq -n --arg t "$TOKEN" '{idToken:$t}' >"$TMP/del.json"
  req POST "https://identitytoolkit.googleapis.com/v1/accounts:delete?key=$KEY" "$TMP/del.json" 0
  rm -f "$TMP/del.json"
  [ "$CODE" = 200 ] && row OK "後片付け: 匿名ユーザー削除(accounts:delete)" "$CODE" "$MS" "" \
                    || row NG "後片付け: 匿名ユーザー削除(accounts:delete)" "$CODE" "$MS" "$(jbody '.error.message // empty')"
fi
TOKEN=""; KEY=""

# --- 結果の表 --------------------------------------------------------------------
echo
echo "## demo-check $ENV_NAME ($(date '+%Y-%m-%d %H:%M'))"
echo
echo "| 結果 | 確認 | HTTP | ms | メモ |"
echo "|---|---|---|---|---|"
for r in "${ROWS[@]}"; do
  IFS='|' read -r a b c d e <<<"$r"
  echo "| $a | $b | $c | $d | ${e//|//} |"
done
echo
if [ "$FAILS" -gt 0 ]; then echo "NG: $FAILS 件失敗"; exit 1; fi
echo "OK: すべて成功"
