#!/usr/bin/env bash
# scripts/product-metrics-weekly.sh の引数と出力の形を偽の gcloud で確かめる(GCP に触らない)。
# 偽の gcloud は backend/cmd/product-metrics/testdata/logs.json を返す。表は go run で作り、同じ所の weekly.golden.md と比べる。
# 表の部分だけ go が要る(無ければ飛ばす)。引数・--dry-run の部分は go も gcloud も呼ばない。
set -euo pipefail
cd "$(dirname "$0")"
G=../backend/cmd/product-metrics/testdata
fail() { echo "FAIL: $*" >&2; exit 1; }
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin" "$TMP/spy"

cat >"$TMP/bin/gcloud" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"$TMP/gcloud.calls"
case "\$*" in
  "logging read"*) cat "$PWD/$G/logs.json" ;;
  *) echo "fake gcloud: unexpected \$*" >&2; exit 1 ;;
esac
EOF
# 呼ばれてはいけない所で go を見張る
cat >"$TMP/spy/go" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"$TMP/go.calls"
exit 1
EOF
chmod +x "$TMP/bin/gcloud" "$TMP/spy/go"

# 1. --dry-run はフィルタだけ出し、gcloud も go も呼ばない
out="$(PATH="$TMP/spy:$TMP/bin:$PATH" PROJECT_ID=fake-project ./product-metrics-weekly.sh stg --dry-run --days 14)"
[ ! -e "$TMP/gcloud.calls" ] || fail "--dry-run で gcloud を呼んだ"
[ ! -e "$TMP/go.calls" ] || fail "--dry-run で go を呼んだ"
grep -Eq '^project=fake-project since=[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:]{8}Z$' <<<"$out" || fail "dry-run の見出し: $out"
for k in 'resource.labels.service_name=("ai-stg" OR "appconfig-stg")' 'jsonPayload.event=("ai.generate" OR "session.start" OR "app.opened" OR "app.shared" OR "app.published" OR "share.imported")'; do
  grep -qF -- "$k" <<<"$out" || fail "dry-run に $k が無い"
done
# since は --days ぶん前(日付だけ比べる)
want_since="$(date -u -v-14d +%Y-%m-%d 2>/dev/null || date -u -d '-14 days' +%Y-%m-%d)"
grep -qF "since=$want_since" <<<"$out" || fail "--days 14 の since が $want_since でない: $out"

# 2. 引数の誤りは終了コード 2(gcloud も go も呼ばない)
expect2() {
  local code
  set +e; PATH="$TMP/spy:$TMP/bin:$PATH" ./product-metrics-weekly.sh "$@" >/dev/null 2>&1; code=$?; set -e
  [ "$code" = 2 ] || fail "引数 [$*] で終了コード 2 のはずが $code"
}
expect2
expect2 dev
expect2 prod --bogus
expect2 prod --days x
expect2 prod --days 0
expect2 prod --days 91
expect2 prod --days 1000 --dry-run
[ ! -e "$TMP/gcloud.calls" ] || fail "引数の誤りで gcloud を呼んだ"
[ ! -e "$TMP/go.calls" ] || fail "引数の誤りで go を呼んだ"
# 上限ちょうどと先頭 0 は通る
PATH="$TMP/spy:$TMP/bin:$PATH" ./product-metrics-weekly.sh prod --days 90 --dry-run >/dev/null || fail "--days 90 が通らない"
PATH="$TMP/spy:$TMP/bin:$PATH" ./product-metrics-weekly.sh prod --days 07 --dry-run >/dev/null || fail "--days 07 が通らない"

# 3. 表(偽の gcloud → go run)。1 行目は窓の注釈、その後は golden と同じ
if ! command -v go >/dev/null; then
  echo "skip: go が無いので表の比較を飛ばす"
  echo "ok"
  exit 0
fi
out="$(PATH="$TMP/bin:$PATH" PROJECT_ID=fake-project ./product-metrics-weekly.sh prod --days 21)"
grep -c '^logging read' "$TMP/gcloud.calls" | grep -qx 1 || fail "gcloud logging read が 1 回でない"
grep -q -- '--project=fake-project' "$TMP/gcloud.calls" || fail "PROJECT_ID が渡っていない"
grep -q -- '--limit=200000' "$TMP/gcloud.calls" || fail "既定の LIMIT"
n="$(grep -c '"timestamp"' "$G/logs.json")"
[ "$(head -n 1 <<<"$out")" = "<!-- fake-project(ai-prod / appconfig-prod)・21 日・$n 行 -->" ] || fail "1 行目: $(head -n 1 <<<"$out")"
diff -u "$G/weekly.golden.md" <(tail -n +2 <<<"$out") || fail "表が golden と違う"

# LIMIT に達したら警告(表は出す)
err="$(PATH="$TMP/bin:$PATH" LIMIT="$n" ./product-metrics-weekly.sh prod 2>&1 >/dev/null)"
grep -qF "warning: $n 行で LIMIT に達した" <<<"$err" || fail "LIMIT の警告が無い: $err"

# --input(相対パス)は gcloud を呼ばずに同じ表
: >"$TMP/gcloud.calls"
diff -u "$G/weekly.golden.md" <(PATH="$TMP/bin:$PATH" ./product-metrics-weekly.sh --input "$G/logs.json") || fail "--input の表が golden と違う"
[ ! -s "$TMP/gcloud.calls" ] || fail "--input で gcloud を呼んだ"
echo "ok"
