#!/usr/bin/env bash
# scripts/runner-usage.sh の gcloud を読む側を偽の gcloud で確かめる(#2862・GCP 不要・本物の gcloud は呼ばない)。
# 偽の gcloud は引数を $FAKE_GCLOUD_ARGS に 1 行ずつ書き、$FAKE_GCLOUD_OUT の JSON を返す。
set -euo pipefail
cd "$(dirname "$0")"
D=testdata/runner-usage
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"

cat >"$TMP/bin/gcloud" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$FAKE_GCLOUD_ARGS"
cat "$FAKE_GCLOUD_OUT"
EOF
chmod +x "$TMP/bin/gcloud"

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
run() { # 期待コード args...
  local want="$1" code; shift
  set +e
  OUT="$(PATH="$TMP/bin:$PATH" FAKE_GCLOUD_ARGS="$TMP/args" FAKE_GCLOUD_OUT="${FAKE_OUT:-$D/usage.json}" \
    PROJECT_ID=demo-project ./runner-usage.sh "$@" 2>&1)"
  code=$?
  set -e
  [ "$code" = "$want" ] || fail "終了コード $want のはずが $code"
}
arg() { grep -qxF -- "$1" "$TMP/args" || { cat "$TMP/args" >&2; fail "gcloud の引数に「$1」が無い"; }; }

# 1. 偽の gcloud の出力から表を作る。--input で同じ JSON を読んだ時と見出しの 1 行目以外は同じ
run 3 prod --day 2026-11-01 --threshold 2
arg logging
arg read
arg --project=demo-project
arg --limit=50000
arg --order=desc
grep -q '^resource.type="cloud_run_revision" AND resource.labels.service_name="runner-prod" AND timestamp>="[0-9-]*T00:00:00+09:00" AND jsonPayload.event="runner.usage"$' "$TMP/args" \
  || { cat "$TMP/args" >&2; fail "gcloud のフィルタの形"; }
grep -q '^--format=json(timestamp,jsonPayload.app_id,.*,jsonPayload.est_cost_usd)$' "$TMP/args" || fail "gcloud の --format"
has "# runner の使った量 runner-prod(demo-project)(見る日 2026-11-01・日本時間)"
diff -u <(tail -n +2 "$D/expected.md") <(tail -n +2 <<<"$OUT") || fail "--input の期待値と違う"

# 2. 週の表(日ごと): 7 日の範囲の日が新しい順に並ぶ
has "## 日ごと"
has "| 2026-11-02 | 1 | 0 | 5 | 0 | 1 | 0.000026 |"
has "| 2026-11-01 | 4 | 2 | 3 | 13 | 2 | 0.000122 |"
has "| 2026-10-31 | 1 | 0 | 1 | 2 | 1 | 0.000019 |"
days="$(sed -n '/^## 日ごと/,/^## 月ごと/p' <<<"$OUT" | grep -o '^| 20[0-9-]* ' | tr -d '| ')"
[ "$days" = "$(printf '2026-11-02\n2026-11-01\n2026-10-31')" ] || fail "日ごとの並び: $days"

# 3. 見る日が期間より前なら、その日から読む。RUNNER_SERVICE・LIMIT を渡す
RUNNER_SERVICE=runner-x LIMIT=8 run 0 prod --day 2020-01-02 --days 1 --threshold 0
grep -qF 'timestamp>="2020-01-02T00:00:00+09:00"' "$TMP/args" || fail "見る日から読んでいない"
grep -qF 'service_name="runner-x"' "$TMP/args" || fail "RUNNER_SERVICE"
arg --limit=8
has "warning: LIMIT(8 行)に達した"

# 4. 空のログ
echo '[]' >"$TMP/empty.json"
FAKE_OUT="$TMP/empty.json" run 0 stg --day 2026-11-01
has "note: runner は #288 まで prod だけで動く"
has "| (無し) | 0 | 0 | 0 | 0 | 0 | 0 | 0 |"
grep -qF 'service_name="runner-stg"' "$TMP/args" || fail "stg の service"

# 5. --dry-run は gcloud を呼ばず、フィルタとコマンドだけ出す
rm -f "$TMP/args"
run 0 prod --day 2026-11-01 --dry-run
[ ! -e "$TMP/args" ] || fail "--dry-run で gcloud を呼んだ"
has "[runner.usage]"
has "[command]"
has "gcloud logging read "
has "--project=demo-project --limit=50000 --order=desc"
has "gcloud logging read 'resource.type=\"cloud_run_revision\" AND "
has "'--format=json(timestamp,"
# 出したコマンドはそのまま貼って動く(偽の gcloud で同じ引数になる)
cmd="$(sed -n '/^\[command\]$/{n;p;}' <<<"$OUT")"
run 3 prod --day 2026-11-01 --threshold 2
cp "$TMP/args" "$TMP/args.real"
PATH="$TMP/bin:$PATH" FAKE_GCLOUD_ARGS="$TMP/args" FAKE_GCLOUD_OUT="$D/usage.json" bash -c "$cmd" >/dev/null
diff -u "$TMP/args.real" "$TMP/args" || fail "--dry-run のコマンドと実際の引数が違う"

echo "ok"
