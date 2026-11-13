#!/usr/bin/env bash
# scripts/demo-check.sh --list を確かめる(#2863・ネットワーク・API キー不要)。
# 偽の curl は呼ばれたら印を残す。--list は何も叩かないこと、一覧が結果の表の「確認」の名前を全部持つことを見る。
set -euo pipefail
cd "$(dirname "$0")"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"
cat >"$TMP/bin/curl" <<'EOF'
#!/usr/bin/env bash
: >"$FAKE_CURL_CALLED"
exit 7
EOF
chmod +x "$TMP/bin/curl"

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
run() { # 期待コード args...
  local want="$1" code; shift
  set +e
  OUT="$(PATH="$TMP/bin:$PATH" FAKE_CURL_CALLED="$TMP/called" FIREBASE_WEB_API_KEY='' ./demo-check.sh "$@" 2>&1)"
  code=$?
  set -e
  [ "$code" = "$want" ] || fail "$*: 終了コード $want のはずが $code"
}

# 1. 環境を指定しなくても一覧を出し、何も叩かない
run 0 --list
[ ! -e "$TMP/called" ] || fail "--list で curl を呼んだ"
[ "$(sed -n 1p <<<"$OUT")" = "| # | 確認 | 何を見るか | --version-only | DEMO_CHECK_NO_AI=1 |" ] || fail "見出しの行"
[ "$(sed -n 2p <<<"$OUT")" = "|---|---|---|---|---|" ] || fail "区切りの行"

# 2. 表の形: 各行 5 列・番号は 1 から連番・モードの列は「見る」か「飛ばす」
n=0
while IFS= read -r line; do
  n=$((n + 1))
  IFS='|' read -r _ num name what vo noai rest <<<"$line"
  [ -z "$rest" ] || fail "列が多い: $line"
  [ "$(echo "$num" | tr -d ' ')" = "$n" ] || fail "番号が連番でない: $line"
  [ -n "${name// /}" ] && [ -n "${what// /}" ] || fail "名前か説明が空: $line"
  for m in "$vo" "$noai"; do
    case "$(echo "$m" | tr -d ' ')" in 見る | 飛ばす) ;; *) fail "モードの列: $line" ;; esac
  done
done < <(tail -n +3 <<<"$OUT")
[ "$n" -ge 10 ] || fail "項目が少ない: $n"

# 3. スクリプトの結果の表の名前(row の 2 番目の引数)が全部一覧にある。$svc は「appconfig / ai」、$app は <アプリ>
names="$(cut -d'|' -f3 <<<"$OUT" | sed 's/^ //; s/ $//')"
labels="$(grep -o 'row [^ ]* "[^"]*"' demo-check.sh | sed 's/^row [^ ]* "//; s/"$//' | sort -u)"
[ -n "$labels" ] || fail "結果の表の名前が取れない"
while IFS= read -r l; do
  l="${l//\$svc/appconfig / ai}"; l="${l//\$app/<アプリ>}"
  grep -qxF -- "$l" <<<"$names" || fail "一覧に無い項目: $l"
done <<<"$labels"

# 4. --version-only の行は「見る」が /health の 3 つだけ
seen="$(tail -n +3 <<<"$OUT" | awk -F'|' '$5 ~ /見る/ {gsub(/^ +| +$/, "", $3); print $3}')"
[ "$seen" = "$(printf 'appconfig / ai GET /health ×3\nappconfig / ai env(/health)\nappconfig / ai 版(/health version)')" ] \
  || fail "--version-only で見る項目: $seen"

# 5. 他の引数と一緒には使えない。既定の振る舞い(環境が要る)は変わらない
run 2 --list stg
run 2 stg --list
run 2
[ ! -e "$TMP/called" ] || fail "引数の誤りで curl を呼んだ"

echo "ok"
