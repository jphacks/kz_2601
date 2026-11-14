#!/usr/bin/env bash
# scripts/ai-eval-comment.sh を作り物の gh で確かめる(GitHub には触らない)。
# 初回(足す)・2 回目(印付きのコメントを書き換える)・長すぎる時(上から切ってリンク)・既定の dry-run。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/ai-eval-comment.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }

# 作り物の gh: コメントの一覧は $TMP/comments.jsonl(gh api --jq の 1 行 1 件の形)、
# 呼ばれた引数は $TMP/calls、書き込みの本文は $TMP/payload に残す
mkdir -p "$TMP/bin"
cat >"$TMP/bin/gh" <<EOF
#!/usr/bin/env bash
echo "\$*" >>"$TMP/calls"
case "\$*" in
  "api --paginate repos/o/r/issues/7/comments --jq "*) cat "$TMP/comments.jsonl" ;;
  "api -X POST repos/o/r/issues/7/comments --input "* | "api -X PATCH repos/o/r/issues/comments/"*" --input "*)
    cat "\${@: -1}" >"$TMP/payload" ;;
  *) echo "fake gh: unexpected \$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$TMP/bin/gh"
run() { set +e; OUT="$(PATH="$TMP/bin:$PATH" GITHUB_REPOSITORY=o/r "$SCRIPT" "$@" 2>&1)"; CODE=$?; set -e; }

cat >"$TMP/report.md" <<'EOF'
## ai-eval

| group | pass / checked | pass% | min | result | no expect |
|---|---|---|---|---|---|
| kakeibo | 9 / 10 | 90% | 90% | PASS | 0 |
EOF

# 1. 初回: 印付きのコメントが無い → 既定の dry-run は「足す」を出すだけ
printf '%s\n' '{"id":1,"body":"LGTM"}' >"$TMP/comments.jsonl"
: >"$TMP/calls"
run 7 "$TMP/report.md"
[ "$CODE" = 0 ] || fail "dry-run の終了コード $CODE"
has "DRY-RUN (o/r) 足す: PR #7 に新しいコメント"
has "<!-- ai-eval-report -->"
has "| kakeibo | 9 / 10 | 90% | 90% | PASS | 0 |"
grep -q -- "-X" "$TMP/calls" && fail "dry-run で書いた: $(cat "$TMP/calls")"

run 7 "$TMP/report.md" --execute
[ "$CODE" = 0 ] || fail "初回 --execute の終了コード $CODE"
has "EXECUTE (o/r) 足す"
grep -q "^api -X POST repos/o/r/issues/7/comments" "$TMP/calls" || fail "POST していない: $(cat "$TMP/calls")"
jq -e '.body | startswith("<!-- ai-eval-report -->\n## ai-eval")' "$TMP/payload" >/dev/null || fail "本文: $(cat "$TMP/payload")"

# 2. 2 回目: 印付きのコメント(最後の 1 つ)を書き換える。印の無いコメントには触れない
{
  printf '%s\n' '{"id":1,"body":"LGTM"}'
  printf '%s\n' '{"id":11,"body":"<!-- ai-eval-report -->\nold"}'
  printf '%s\n' '{"id":12,"body":"<!-- ai-eval-report -->\nolder but later"}'
  printf '%s\n' '{"id":13,"body":"quote: <!-- ai-eval-report -->"}'
} >"$TMP/comments.jsonl"
: >"$TMP/calls"
run 7 "$TMP/report.md"
has "DRY-RUN (o/r) 書き換える: コメント 12"
run 7 "$TMP/report.md" --execute
[ "$CODE" = 0 ] || fail "2 回目の終了コード $CODE"
grep -q "^api -X PATCH repos/o/r/issues/comments/12 " "$TMP/calls" || fail "12 を書き換えていない: $(cat "$TMP/calls")"
grep -q "POST" "$TMP/calls" && fail "2 回目に足した"

# 3. 長すぎる: 上から --max-lines 行・開いた <details> を閉じる・成果物へのリンク
{
  cat "$TMP/report.md"
  printf '\n<details><summary>通らなかったケース (40)</summary>\n\n| id | group | misses |\n|---|---|---|\n'
  for i in $(seq 1 40); do printf '| case-%02d | kakeibo | layout |\n' "$i"; done
  printf '\n</details>\n'
} >"$TMP/long.md"
printf '%s\n' '{"id":1,"body":"LGTM"}' >"$TMP/comments.jsonl"
run 7 "$TMP/long.md" --max-lines 15 --artifact-url https://github.com/o/r/actions/runs/1
[ "$CODE" = 0 ] || fail "長い時の終了コード $CODE"
has "| case-01 | kakeibo | layout |"
hasnt "| case-40 |"
has "_表が長いので上から 15 行(全 "
has "全部は [CI の成果物](https://github.com/o/r/actions/runs/1)。_"
[ "$(grep -c '</details>' <<<"$OUT")" = 1 ] || fail "<details> を閉じていない"
run 7 "$TMP/long.md" --max-lines 1000
hasnt "表が長いので"
has "| case-40 |"

# 4. 引数の誤りは 2・gh の失敗は 1
for args in "x $TMP/report.md" "7" "7 $TMP/none.md" "7 $TMP/report.md --max-lines 0" "7 $TMP/report.md --artifact-url http://x" "7 $TMP/report.md --bogus"; do
  # shellcheck disable=SC2086 # 引数を分けて渡す
  run $args
  [ "$CODE" = 2 ] || fail "引数 '$args' の終了コード 2 のはずが $CODE"
done
set +e
OUT="$(PATH="$TMP/bin:$PATH" GITHUB_REPOSITORY='' "$SCRIPT" 7 "$TMP/report.md" 2>&1)"; CODE=$?
set -e
[ "$CODE" = 2 ] || fail "repo 無しの終了コード $CODE"
run 8 "$TMP/report.md"
[ "$CODE" = 1 ] || fail "gh の失敗の終了コード 1 のはずが $CODE"

echo "ok: ai-eval-comment"
