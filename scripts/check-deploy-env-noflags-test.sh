#!/usr/bin/env bash
# scripts/check-deploy-env.sh が --update-env-vars の無い workflow で理由を出して落ちるかを作り物で確かめる。
set -euo pipefail
cd "$(dirname "$0")/.."
fail() { echo "FAIL: $*" >&2; exit 1; }
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat >"$TMP/wf.yml" <<'YML'
# --update-env-vars を使わない作り物
jobs:
  deploy:
    steps:
      - run: gcloud run deploy svc --image=x
YML
cat >"$TMP/doc.md" <<'MD'
## デプロイが決める値

| `APP_ENV` | stg / prod |

## 運用で変える値

| `LOG_LEVEL` | info |
MD

set +e
scripts/check-deploy-env.sh "$TMP/wf.yml" "$TMP/doc.md" >"$TMP/out" 2>"$TMP/err"
code=$?
set -e
[ "$code" -ne 0 ] || fail "--update-env-vars が無いのに 0 で終わった"
grep -qF -- "--update-env-vars が無い" "$TMP/err" || fail "理由が出ない: $(cat "$TMP/err")"

echo "OK: check-deploy-env-noflags-test"
