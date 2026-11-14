#!/usr/bin/env bash
# scripts/beta-weekly.sh の表を偽の gcloud・gh で確かめる(GCP・GitHub に触らない・jq だけ)。
# 偽の gcloud はフィルタの event で testdata/beta-weekly/{gen,http,usage}.json を返し、偽の gh は issues.json に -q の jq をかける。
# 期待値を直す時: scripts/beta-weekly-test.sh --update
set -euo pipefail
cd "$(dirname "$0")"
D=testdata/beta-weekly
fail() { echo "FAIL: $*" >&2; exit 1; }
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"

cat >"$TMP/bin/gcloud" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"$TMP/gcloud.calls"
case "\$*" in
  "logging read"*'jsonPayload.event="ai.generate"'*) cat "$PWD/$D/gen.json" ;;
  "logging read"*'run.googleapis.com%2Frequests'*) cat "$PWD/$D/http.json" ;;
  "logging read"*'jsonPayload.event="ai.usage"'*) cat "$PWD/$D/usage.json" ;;
  *) echo "fake gcloud: unexpected \$*" >&2; exit 1 ;;
esac
EOF
cat >"$TMP/bin/gh" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"$TMP/gh.calls"
[ -n "\${FAKE_GH_FAIL:-}" ] && exit 1
q=""
while [ \$# -gt 0 ]; do case "\$1" in -q) q="\$2"; shift 2 ;; *) shift ;; esac; done
jq -r "\$q" "$PWD/$D/issues.json"
EOF
chmod +x "$TMP/bin/gcloud" "$TMP/bin/gh"

run() { PATH="$TMP/bin:$PATH" PROJECT_ID=fake-project ./beta-weekly.sh "$@"; }
# 窓の時刻は実行時刻で変わるので伏せて比べる
norm() { sed -E 's/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z/<TS>/g'; }

# 1. 表(偽の gcloud・gh)
out="$(run prod | norm)"
if [ "${1:-}" = "--update" ]; then
  printf '%s\n' "$out" >"$D/expected.md"
  echo "updated $D/expected.md"
  exit 0
fi
diff -u "$D/expected.md" <(printf '%s\n' "$out") || fail "期待値と違う"
[ "$(grep -c '^logging read' "$TMP/gcloud.calls")" = 3 ] || fail "gcloud logging read が 3 回でない: $(cat "$TMP/gcloud.calls")"
grep -q -- '--project=fake-project' "$TMP/gcloud.calls" || fail "PROJECT_ID が渡っていない"
grep -q 'resource.labels.service_name="ai-prod"' "$TMP/gcloud.calls" || fail "既定の AI_SERVICE が ai-<env> でない"
grep -q -- '--label priority:P1' "$TMP/gh.calls" || fail "gh に priority:P1 を渡していない"

# 2. --days は見出しに出て、AI_SERVICE で上書きできる
: >"$TMP/gcloud.calls"
out="$(PATH="$TMP/bin:$PATH" AI_SERVICE=ai-custom ./beta-weekly.sh stg --days 14)"
grep -qF '# β 品質ゲート stg(ai-custom・' <<<"$out" || fail "AI_SERVICE が見出しに無い: $out"
grep -qF '・14 日)' <<<"$out" || fail "--days 14 が見出しに無い"
grep -q 'service_name="ai-custom"' "$TMP/gcloud.calls" || fail "AI_SERVICE がフィルタに無い"

# 3. gh が失敗したら P1 は「(取得失敗)」で、表は出る
out="$(FAKE_GH_FAIL=1 run prod)"
grep -qF '| P1 のフィードバックの残数 | (取得失敗) |' <<<"$out" || fail "gh 失敗の時の P1: $out"

# 4. --dry-run はフィルタだけ出し、gcloud・gh を呼ばない
rm -f "$TMP/gcloud.calls" "$TMP/gh.calls"
out="$(run prod --dry-run --days 3)"
[ ! -e "$TMP/gcloud.calls" ] || fail "--dry-run で gcloud を呼んだ: $(cat "$TMP/gcloud.calls")"
[ ! -e "$TMP/gh.calls" ] || fail "--dry-run で gh を呼んだ"
grep -q '^project=fake-project service=ai-prod window=' <<<"$out" || fail "dry-run の見出し: $out"
for k in '[ai.generate]' '[POST /v1/generate request log]' '[ai.usage]' 'jsonPayload.event="ai.generate"' \
  'logName="projects/fake-project/logs/run.googleapis.com%2Frequests"' 'jsonPayload.event="ai.usage"'; do
  grep -qF -- "$k" <<<"$out" || fail "dry-run に $k が無い"
done

# 5. 引数の誤りは終了コード 2
expect2() {
  local code
  set +e; run "$@" >/dev/null 2>&1; code=$?; set -e
  [ "$code" = 2 ] || fail "引数 [$*] で終了コード 2 のはずが $code"
}
expect2
expect2 dev
expect2 --days 7
expect2 prod --days x
expect2 prod --days -1
expect2 prod --bogus
# --days の値が無い時は ${2:?} で止まる(終了コードは 1)
if run prod --days >/dev/null 2>&1; then fail "--days の値が無いのに通った"; fi
[ ! -e "$TMP/gcloud.calls" ] || fail "引数の誤りで gcloud を呼んだ"
echo "ok"
