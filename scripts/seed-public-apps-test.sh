#!/usr/bin/env bash
# scripts/seed-public-apps.sh の --dry-run と入力の検査を偽の curl で確かめる(ネットワーク・Firebase に触らない)。
# 偽の curl は「メソッド URL」と本文を $TMP/calls に残し、パスごとに決まった応答を返す。
set -euo pipefail
cd "$(dirname "$0")"
fail() { echo "FAIL: $*" >&2; exit 1; }
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"

cat >"$TMP/bin/curl" <<EOF
#!/usr/bin/env bash
method=GET url="" ifm="" auth="" body=""
while [ \$# -gt 0 ]; do
  case "\$1" in
    -X) method="\$2"; shift 2 ;;
    -H) case "\$2" in If-Match:*) ifm="\$2" ;; Authorization:*) auth="\$2" ;; esac; shift 2 ;;
    --data-binary) body="\$(cat)"; shift 2 ;;
    -d) body="\$2"; shift 2 ;;
    http*) url="\$1"; shift ;;
    *) shift ;;
  esac
done
printf '%s %s|%s|%s\n' "\$method" "\$url" "\$ifm" "\$auth" >>"$TMP/calls"
[ -z "\$body" ] || printf '%s\n' "\$body" | jq -c . >>"$TMP/bodies"
case "\$method \$url" in
  "GET "*/dsl) echo '{"version":"v7"}' ;;
  "PUT "*/dsl) echo '{}' ;;
  "POST "*/publish) id="\${url%/publish}"; id="\${id##*/}"; printf '{"id":"%s","name":"n","installs":0,"components":["a","b"]}\n' "\$id" ;;
  "GET "*/public-apps*) echo '{"items":[1,2,3]}' ;;
  *) echo "fake curl: unexpected \$method \$url" >&2; exit 22 ;;
esac
EOF
chmod +x "$TMP/bin/curl"

run() { PATH="$TMP/bin:$PATH" SEED_STATE="$TMP/state" ./seed-public-apps.sh "$@"; }
expect2() { # 説明 args...
  local what="$1" code; shift
  set +e; OUT="$(run "$@" 2>&1)"; code=$?; set -e
  [ "$code" = 2 ] || { printf '%s\n' "$OUT" >&2; fail "$what: 終了コード 2 のはずが $code"; }
  [ ! -e "$TMP/calls" ] || fail "$what: curl を呼んだ: $(cat "$TMP/calls")"
}

# 1. 入力の検査(どれも curl を呼ばずに 2)
expect2 "URL が無い"
grep -qF 'usage:' <<<"$OUT" || fail "URL が無い時に使い方を出さない"
expect2 "URL が無い(--dry-run だけ)" --dry-run
expect2 "スキームが無い" localhost:8080
expect2 "本番らしい http" http://appconfig-prod-abc.a.run.app
grep -qF 'https を使う' <<<"$OUT" || fail "http の時の文: $OUT"
expect2 "localhost 以外の http" http://appconfig-stg-abc.a.run.app
expect2 "本番らしい https" https://appconfig-prod-abc.a.run.app
grep -qF 'SEED_ALLOW_PROD=1' <<<"$OUT" || fail "本番の時の文: $OUT"
SEED_ALLOW_PROD=1 expect2 "本番でも http は止める" http://appconfig-prod-abc.a.run.app
expect2 "URL が 2 つ" http://localhost:8080 http://localhost:9090
expect2 "知らないオプション" --force http://localhost:8080

# 2. --dry-run: 要求の一覧を出すだけで、curl もトークンの取得もしない(STATE も作らない)
out="$(FIREBASE_WEB_API_KEY=fake-key run --dry-run https://appconfig-stg-abc.a.run.app/)"
[ ! -e "$TMP/calls" ] || fail "--dry-run で curl を呼んだ: $(cat "$TMP/calls")"
[ ! -e "$TMP/state" ] || fail "--dry-run で STATE を書いた"
grep -qxF "DRY-RUN: https://appconfig-stg-abc.a.run.app に送る要求(送らない)・Bearer=Firebase の匿名ユーザー($TMP/state)" <<<"$out" \
  || fail "dry-run の見出し: $out"
[ "$(grep -c '^GET  /v1/apps/catalog-[a-z]*/dsl$' <<<"$out")" = 3 ] || fail "GET dsl が 3 件でない: $out"
[ "$(grep -c '^PUT  /v1/apps/catalog-[a-z]*/dsl  If-Match: <GET の version>  dsl=docs/dsl/examples/[a-z]*.json([0-9]* bytes) source.kind=manual$' <<<"$out")" = 3 ] \
  || fail "PUT dsl が 3 件でない: $out"
grep -qxF 'POST /v1/apps/catalog-walk/publish  {"description":"散歩で見つけたいい場所を写真とひとことで残す","tags":["散歩","写真","メモ"],"authorName":"Tailor It チーム"}' <<<"$out" \
  || fail "publish の本文の要約: $out"
[ "$(tail -n 1 <<<"$out")" = 'GET  /v1/public-apps?sort=popular' ] || fail "最後の要求: $out"
grep -qF 'fake-key' <<<"$out" && fail "dry-run に API キーが出た"
out="$(SEED_TOKEN=secret-token run --dry-run http://localhost:8080)"
grep -qF 'Bearer=SEED_TOKEN' <<<"$out" || fail "SEED_TOKEN の時の見出し: $out"
grep -qF 'secret-token' <<<"$out" && fail "dry-run にトークンが出た"
out="$(SEED_UID=u1 run --dry-run 'http://[::1]:8080')"
grep -qF 'Bearer=dev の uid u1' <<<"$out" || fail "dev の時の見出し: $out"
[ ! -e "$TMP/calls" ] || fail "--dry-run で curl を呼んだ"

# 3. 送る時(dev の uid・偽の curl): 3 つのアプリを GET → PUT(If-Match) → POST し、最後に一覧
out="$(SEED_UID=u1 run http://localhost:8080)"
want="GET http://localhost:8080/v1/apps/catalog-kakeibo/dsl||Authorization: Bearer u1
PUT http://localhost:8080/v1/apps/catalog-kakeibo/dsl|If-Match: \"v7\"|Authorization: Bearer u1
POST http://localhost:8080/v1/apps/catalog-kakeibo/publish||Authorization: Bearer u1
GET http://localhost:8080/v1/apps/catalog-training/dsl||Authorization: Bearer u1
PUT http://localhost:8080/v1/apps/catalog-training/dsl|If-Match: \"v7\"|Authorization: Bearer u1
POST http://localhost:8080/v1/apps/catalog-training/publish||Authorization: Bearer u1
GET http://localhost:8080/v1/apps/catalog-walk/dsl||Authorization: Bearer u1
PUT http://localhost:8080/v1/apps/catalog-walk/dsl|If-Match: \"v7\"|Authorization: Bearer u1
POST http://localhost:8080/v1/apps/catalog-walk/publish||Authorization: Bearer u1
GET http://localhost:8080/v1/public-apps?sort=popular||Authorization: Bearer u1"
diff -u <(printf '%s\n' "$want") "$TMP/calls" || fail "送った要求が違う"
grep -qxF 'published: catalog-walk  n  installs=0  parts=a,b' <<<"$out" || fail "publish の結果の行: $out"
[ "$(tail -n 1 <<<"$out")" = 'catalog: 3 apps' ] || fail "一覧の行: $out"
jq -e 'select(.source) | .source.kind == "manual" and (.dsl | type) == "object"' "$TMP/bodies" >/dev/null || fail "PUT の本文: $(cat "$TMP/bodies")"
grep -qF '"tags":["お金","節約","記録"]' "$TMP/bodies" || fail "publish の本文: $(cat "$TMP/bodies")"
echo "ok"
