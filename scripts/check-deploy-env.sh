#!/usr/bin/env bash
# deploy-cloudrun.yml の環境変数の組み立てを docs/infra/runtime-config.md と突き合わせる(#509)。
#   - --set-env-vars を使っていない(全置換で手で入れた値が消える)
#   - --update-env-vars の各キーが「デプロイが決める値」の表にあり、「運用で変える値」の表に無い
#   - 「デプロイが決める値」の表のキーがどこかの --update-env-vars に入っている
#   - 1 つの --update-env-vars の中でキーが重複せず、値が空でない
# 使い方: scripts/check-deploy-env.sh [workflow] [doc]  (repo のルートから)
set -euo pipefail

wf="${1:-.github/workflows/deploy-cloudrun.yml}"
doc="${2:-docs/infra/runtime-config.md}"
fail=0
err() { echo "NG: $*" >&2; fail=1; }

# コメント行を除いた本文
body="$(grep -v '^[[:space:]]*#' "$wf")"

if grep -q -- '--set-env-vars' <<<"$body"; then
  err "$wf で --set-env-vars を使っている(--update-env-vars にする)"
fi

# 表の 1 列目の `KEY` を見出し(## ...)ごとに取り出す(BSD awk は locale で日本語の == を誤るので C で比べる)
table_keys() {
  LC_ALL=C awk -v want="$1" '
    /^## / { insec = ($0 == "## " want); next }
    insec && /^\| `/ { split($0, c, "`"); print c[2] }
  ' "$doc"
}
deploy_keys="$(table_keys 'デプロイが決める値' | sort -u)"
ops_keys="$(table_keys '運用で変える値' | sort -u)"
[ -n "$deploy_keys" ] || err "$doc の「デプロイが決める値」の表が読めない"
[ -n "$ops_keys" ] || err "$doc の「運用で変える値」の表が読めない"

flags="$(grep -o -- '--update-env-vars="[^"]*"' <<<"$body" | sed 's/^--update-env-vars="//; s/"$//')"
[ -n "$flags" ] || err "$wf に --update-env-vars が無い"

used=""
while IFS= read -r flag; do
  [ -n "$flag" ] || continue
  keys="$(tr ',' '\n' <<<"$flag" | cut -d= -f1)"
  dup="$(sort <<<"$keys" | uniq -d)"
  [ -z "$dup" ] || err "同じ --update-env-vars にキーが重複: $dup"
  while IFS= read -r pair; do
    k="${pair%%=*}"
    v="${pair#*=}"
    [ "$pair" != "$k" ] && [ -n "$v" ] || err "$k の値が空"
    grep -qxF -- "$k" <<<"$deploy_keys" || err "$k が $doc の「デプロイが決める値」に無い"
    if grep -qxF -- "$k" <<<"$ops_keys"; then
      err "$k は「運用で変える値」なのにデプロイが上書きしている"
    fi
  done < <(tr ',' '\n' <<<"$flag")
  used="$used"$'\n'"$keys"
done <<<"$flags"

while IFS= read -r k; do
  [ -n "$k" ] || continue
  grep -qxF -- "$k" <<<"$used" || err "$k が「デプロイが決める値」にあるのに $wf で渡していない"
done <<<"$deploy_keys"

if [ "$fail" -ne 0 ]; then
  echo "deploy の環境変数と $doc がずれています。どちらかを直してください。" >&2
  exit 1
fi
echo "OK: $(sort -u <<<"$used" | grep -c .) keys (--update-env-vars) / $doc と一致"
