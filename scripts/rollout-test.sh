#!/usr/bin/env bash
# scripts/rollout.sh の判定・段階・prod の安全装置を固定の入力で確かめる(gcloud 不要・jq だけ・GCP に触らない)。
set -euo pipefail
cd "$(dirname "$0")"
D=testdata/rollout
fail() { echo "FAIL: $*" >&2; exit 1; }
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# 1. 判定(decide): decide-cases.json の 1 件ずつ
n="$(jq length "$D/decide-cases.json")"
i=0
while [ "$i" -lt "$n" ]; do
  name="$(jq -r ".[$i].name" "$D/decide-cases.json")"
  want="$(jq -r ".[$i].expect" "$D/decide-cases.json")"
  reason="$(jq -r ".[$i].reason // empty" "$D/decide-cases.json")"
  jq ".[$i].metrics" "$D/decide-cases.json" >"$TMP/m.json"
  out="$(./rollout.sh --decide "$TMP/m.json")"
  got="$(printf '%s\n' "$out" | sed -n 's/^decision=//p')"
  [ "$got" = "$want" ] || fail "decide [$name]: want $want, got: $out"
  if [ -n "$reason" ]; then
    printf '%s\n' "$out" | grep -qF "reason=$reason" || fail "decide [$name]: reason '$reason' が無い: $out"
  fi
  i=$((i + 1))
done

# 閾値は環境変数で変えられる
ROLLOUT_MAX_P95_RATIO=3 ./rollout.sh --decide <(jq '.[3].metrics' "$D/decide-cases.json") | grep -qx 'decision=promote' \
  || fail "ROLLOUT_MAX_P95_RATIO で閾値が変わらない"

# 2. 集計(summarize): リビジョンで分ける・/health を除く・p95 は ms
./rollout.sh --summarize "$D/logs.json" --new ai-stg-00002-new --old ai-stg-00001-old >"$TMP/s.json"
want='{"new":{"requests":3,"errors_5xx":1,"p95_ms":2000,"claude_calls":2,"claude_errors":1},"old":{"requests":2,"errors_5xx":0,"p95_ms":50,"claude_calls":1,"claude_errors":0}}'
[ "$(jq -cS . "$TMP/s.json")" = "$(jq -cS . <<<"$want")" ] || fail "summarize: $(jq -c . "$TMP/s.json")"

# 3. 段階(dry-run): 既定は 5 → 25 → 100 の順に update-traffic を出す・GCP には触らない
out="$(./rollout.sh --env stg --service appconfig --to-revision appconfig-stg-2 --from-revision appconfig-stg-1 --wait 0 2>/dev/null)"
got="$(printf '%s\n' "$out" | sed -n 's/.*update-traffic .*--to-revisions=\([^ ]*\) .*/\1/p' | paste -sd' ' -)"
[ "$got" = "appconfig-stg-2=5,appconfig-stg-1=95 appconfig-stg-2=25,appconfig-stg-1=75 appconfig-stg-2=100" ] || fail "steps: $got"
printf '%s\n' "$out" | grep -q 'gcloud logging read' || fail "dry-run に指標の読み方が出ない"
out="$(./rollout.sh --env stg --service ai --steps 10,50,100 --wait 0 2>/dev/null)"
printf '%s\n' "$out" | grep -qF "update-traffic ai-stg --project=jphacks-kz2601 --region=asia-northeast1 '--to-revisions=<NEW_REVISION>=10,<OLD_REVISION>=90'" || fail "--steps / 仮のリビジョン名: $out"
for bad in 5,25 25,5,100 0,100 5,x,100 5,5,100 50,150; do
  if ./rollout.sh --env stg --service ai --steps "$bad" >/dev/null 2>&1; then fail "steps $bad を通した"; fi
done

# 4. 模擬(--metrics-dir): 良い版は 100% まで、悪い版は 25% で旧に 100% 戻す(終了コード 3)
./rollout.sh --env stg --service ai --to-revision new --from-revision old --wait 0 --metrics-dir "$D/sim-ok" >"$TMP/ok.out" 2>/dev/null \
  || fail "sim-ok が 0 で終わらない"
grep -q 'to-revisions=new=100' "$TMP/ok.out" || fail "sim-ok で 100% にしない"
set +e
./rollout.sh --env stg --service ai --to-revision new --from-revision old --wait 0 --metrics-dir "$D/sim-bad" >"$TMP/bad.out" 2>/dev/null
code=$?
set -e
[ "$code" = 3 ] || fail "sim-bad の終了コード 3 のはずが $code"
tail -n 1 "$TMP/bad.out" | grep -q 'to-revisions=old=100' || fail "sim-bad で旧に戻さない: $(cat "$TMP/bad.out")"
grep -q 'to-revisions=new=100' "$TMP/bad.out" && fail "sim-bad なのに 100% にした"
# 指標が読めない(25% のファイルが無い・壊れている)時は戻す(終了コード 3)
mkdir -p "$TMP/broken"
cp "$D/sim-ok/5.json" "$TMP/broken/5.json"
set +e
./rollout.sh --env stg --service ai --to-revision new --from-revision old --wait 0 --metrics-dir "$TMP/broken" >"$TMP/broken.out" 2>/dev/null
code=$?
echo 'not json' >"$TMP/broken/25.json"
./rollout.sh --env stg --service ai --to-revision new --from-revision old --wait 0 --metrics-dir "$TMP/broken" >"$TMP/broken2.out" 2>/dev/null
code2=$?
set -e
[ "$code" = 3 ] && [ "$code2" = 3 ] || fail "指標が読めない時の終了コード 3 のはずが $code / $code2"
tail -n 1 "$TMP/broken.out" | grep -q 'to-revisions=old=100' || fail "指標が読めないのに戻さない"
tail -n 1 "$TMP/broken2.out" | grep -q 'to-revisions=old=100' || fail "壊れた指標で戻さない"
# 指標が足りないまま: 既定は窓を 2 回延ばしてから巻き戻す(終了コード 4)・--on-insufficient promote なら進める
mkdir -p "$TMP/low"
jq '.[7].metrics' "$D/decide-cases.json" >"$TMP/low/5.json"
cp "$TMP/low/5.json" "$TMP/low/100.json"
set +e
./rollout.sh --env stg --service ai --to-revision new --from-revision old --wait 0 --steps 5,100 --metrics-dir "$TMP/low" >"$TMP/low.out" 2>/dev/null
code=$?
set -e
[ "$code" = 4 ] || fail "指標不足の終了コード 4 のはずが $code"
[ "$(grep -c '窓を延ばす' "$TMP/low.out")" = 2 ] || fail "窓を 2 回延ばしていない"
./rollout.sh --env stg --service ai --to-revision new --from-revision old --wait 0 --steps 5,100 --max-extends 0 \
  --on-insufficient promote --metrics-dir "$TMP/low" >/dev/null 2>&1 || fail "--on-insufficient promote で進まない"

# 4b. --execute の道筋を偽の gcloud で通す(GCP には触らない。PROJECT_ID も偽物にしておく)
mkdir -p "$TMP/bin"
cat >"$TMP/bin/gcloud" <<EOF
#!/usr/bin/env bash
echo "\$*" >>"$TMP/calls"
case "\$*" in
  "run services describe"*) echo '{"status":{"latestCreatedRevisionName":"ai-stg-00002-new","traffic":[{"revisionName":"ai-stg-00001-old","percent":100},{"revisionName":"ai-stg-00002-new","percent":0}]}}' ;;
  "run services update-traffic"*) ;;
  "logging read"*) cat "$PWD/$D/logs.json" ;;
  *) echo "fake gcloud: unexpected \$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$TMP/bin/gcloud"
set +e
PATH="$TMP/bin:$PATH" PROJECT_ID=fake-project ROLLOUT_MIN_REQUESTS=1 \
  ./rollout.sh --env stg --service ai --execute --wait 0 >/dev/null 2>&1
code=$?
set -e
[ "$code" = 3 ] || fail "偽 gcloud: 新の 5xx 1/3 で終了コード 3 のはずが $code"
# 巻き戻しの通知: Actions では ::error 注釈と job summary に出す(#372)
: >"$TMP/calls"
set +e
PATH="$TMP/bin:$PATH" PROJECT_ID=fake-project ROLLOUT_MIN_REQUESTS=1 GITHUB_ACTIONS=true GITHUB_STEP_SUMMARY="$TMP/summary.md" \
  ./rollout.sh --env stg --service ai --execute --wait 0 >"$TMP/notify.out" 2>/dev/null
set -e
grep -q '^::error title=rollout ai-stg rolled back::ai-stg を ai-stg-00001-old に 100%25 戻した' "$TMP/notify.out" \
  || fail "巻き戻しの ::error 注釈が無い: $(cat "$TMP/notify.out")"
grep -q '^### rollout ai-stg: rolled back' "$TMP/summary.md" || fail "巻き戻しが job summary に無い"
# dry-run(模擬)の巻き戻しは注釈にしない
GITHUB_ACTIONS=true GITHUB_STEP_SUMMARY=/dev/null ./rollout.sh --env stg --service ai --to-revision new --from-revision old \
  --wait 0 --metrics-dir "$D/sim-bad" >"$TMP/dry-notify.out" 2>/dev/null || true
grep -q 'to-revisions=old=100' "$TMP/dry-notify.out" || fail "模擬の巻き戻しが走っていない"
if grep -q '^::error' "$TMP/dry-notify.out"; then fail "dry-run で ::error 注釈を出した"; fi
grep -q 'update-traffic ai-stg --project=fake-project --region=asia-northeast1 --to-revisions=ai-stg-00002-new=5,ai-stg-00001-old=95' "$TMP/calls" \
  || fail "偽 gcloud: describe から新旧を読んでいない: $(cat "$TMP/calls")"
tail -n 1 "$TMP/calls" | grep -q -- '--to-revisions=ai-stg-00001-old=100' || fail "偽 gcloud: 旧に戻していない: $(cat "$TMP/calls")"
grep -q 'resource.labels.revision_name=("ai-stg-00002-new" OR "ai-stg-00001-old")' "$TMP/calls" || fail "偽 gcloud: ログのフィルタ"

# 5. prod の安全装置
out="$(./rollout.sh --env prod --service appconfig --wait 0 2>&1)"
printf '%s\n' "$out" | grep -q '^DRY-RUN' || fail "prod の既定が dry-run でない"
printf '%s\n' "$out" | grep -q '^EXECUTE' && fail "prod が --execute 無しで実行になった"
if ./rollout.sh --service appconfig-prod --execute >/dev/null 2>&1; then fail "--env 無しで実行できた"; fi
if ./rollout.sh --env stg --service appconfig-prod --execute >/dev/null 2>&1; then fail "--env stg で prod のサービスを実行できた"; fi
if ./rollout.sh --env prod --service appconfig --execute --metrics-dir "$D/sim-ok" >/dev/null 2>&1; then fail "--execute と --metrics-dir を一緒に使えた"; fi
if ./rollout.sh --env stg --service ai --to-revision a --from-revision a >/dev/null 2>&1; then fail "新旧が同じでも通った"; fi
if ROLLOUT_MAX_ERROR_RATE=x ./rollout.sh --env stg --service ai >/dev/null 2>&1; then fail "数でない閾値を通した"; fi
echo "ok"
