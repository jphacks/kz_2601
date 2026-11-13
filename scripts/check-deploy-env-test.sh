#!/usr/bin/env bash
# scripts/check-deploy-env.sh の検査が、わざと崩した workflow と表で落ちることを確かめる(GCP・GitHub 不要)。
# 正しい組は testdata/check-deploy-env/{workflow.yml,runtime-config.md}。崩した組は sed で一時ファイルに作る。
set -euo pipefail
cd "$(dirname "$0")"
D=testdata/check-deploy-env
fail() { echo "FAIL: $*" >&2; exit 1; }
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# expect_ng <説明> <出力に含む文> <workflow> <doc>
expect_ng() {
  local what="$1" want="$2" out code
  set +e; out="$(./check-deploy-env.sh "$3" "$4" 2>&1)"; code=$?; set -e
  [ "$code" = 1 ] || { printf '%s\n' "$out" >&2; fail "$what: 終了コード 1 のはずが $code"; }
  grep -qF -- "$want" <<<"$out" || { printf '%s\n' "$out" >&2; fail "$what: 「${want}」が無い"; }
}

# 0. 正しい組は通る(コメントの --set-env-vars は数えない)
out="$(./check-deploy-env.sh "$D/workflow.yml" "$D/runtime-config.md")" || fail "正しい組で落ちた"
[ "$out" = "OK: 3 keys (--update-env-vars) / $D/runtime-config.md と一致" ] || fail "正しい組の出力: $out"

# 1. --set-env-vars を使う
sed 's/--update-env-vars="APP_ENV=stg,AI_MODEL/--set-env-vars="APP_ENV=stg,AI_MODEL/' "$D/workflow.yml" >"$TMP/set.yml"
expect_ng "--set-env-vars" "NG: $TMP/set.yml で --set-env-vars を使っている" "$TMP/set.yml" "$D/runtime-config.md"

# 2. 表に無いキーを渡す
sed 's/AI_MODEL=claude-test/AI_MODEL=claude-test,NEW_FLAG=1/' "$D/workflow.yml" >"$TMP/unknown.yml"
expect_ng "表に無いキー" "NG: NEW_FLAG が $D/runtime-config.md の「デプロイが決める値」に無い" "$TMP/unknown.yml" "$D/runtime-config.md"

# 3. 表にあって使っていないキー
# shellcheck disable=SC2016 # awk の文字列の ` はそのまま
awk '{ print } /^\| `GIT_SHA` / { print "| `UNUSED_KEY` | 使っていない |" }' "$D/runtime-config.md" >"$TMP/unused.md"
grep -q 'UNUSED_KEY' "$TMP/unused.md" || fail "崩した表を作れていない"
expect_ng "使っていないキー" "NG: UNUSED_KEY が「デプロイが決める値」にあるのに $D/workflow.yml で渡していない" "$D/workflow.yml" "$TMP/unused.md"

# 4. 同じ --update-env-vars の中で重複
sed 's/APP_ENV=stg,GIT_SHA/APP_ENV=stg,APP_ENV=prod,GIT_SHA/' "$D/workflow.yml" >"$TMP/dup.yml"
expect_ng "重複" "NG: 同じ --update-env-vars にキーが重複: APP_ENV" "$TMP/dup.yml" "$D/runtime-config.md"

# 5. ついでに: 値が空・運用で変える値の上書き・表が読めない
sed 's/AI_MODEL=claude-test/AI_MODEL=/' "$D/workflow.yml" >"$TMP/empty.yml"
expect_ng "値が空" "NG: AI_MODEL の値が空" "$TMP/empty.yml" "$D/runtime-config.md"
sed 's/AI_MODEL=claude-test/AI_MODEL=claude-test,RATE_LIMIT=5/' "$D/workflow.yml" >"$TMP/ops.yml"
expect_ng "運用で変える値" "NG: RATE_LIMIT は「運用で変える値」なのにデプロイが上書きしている" "$TMP/ops.yml" "$D/runtime-config.md"
sed 's/^## 運用で変える値$/## 運用の値/' "$D/runtime-config.md" >"$TMP/noops.md"
expect_ng "表が読めない" "の「運用で変える値」の表が読めない" "$D/workflow.yml" "$TMP/noops.md"
echo "ok"
