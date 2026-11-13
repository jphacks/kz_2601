#!/usr/bin/env bash
# ai-eval の結果の表(go run ./cmd/ai-eval ... -report md -report-out r.md)を PR のコメントに出す(#371・#2292)。
# 印(HTML コメント)の付いた前のコメントがあれば書き換え、無ければ足す。1 つの PR に表のコメントは 1 つ。
# 使い方: scripts/ai-eval-comment.sh <pr> <report.md> [--repo owner/name] [--artifact-url URL] [--max-lines N] [--execute]
#   既定は dry-run: 前のコメントを読むだけで、何をするか(足す / 書き換える)と本文を出す。書くのは --execute の時だけ。
#   repo は --repo か GITHUB_REPOSITORY。表が --max-lines(既定 150)行より長い時は上から切り、全部は --artifact-url へのリンクに。
# 終了コード: 0 済んだ(dry-run を含む) / 1 gh の失敗 / 2 引数の誤り
set -euo pipefail

MARKER='<!-- ai-eval-report -->'
usage() {
  echo "usage: $0 <pr> <report.md> [--repo owner/name] [--artifact-url URL] [--max-lines N] [--execute]" >&2
  exit 2
}

pr="" report="" repo="${GITHUB_REPOSITORY:-}" artifact="" max_lines=150 execute=0
while [ $# -gt 0 ]; do
  case "$1" in
    -h | --help) usage ;;
    --repo) [ $# -ge 2 ] || usage; repo="$2"; shift 2 ;;
    --artifact-url) [ $# -ge 2 ] || usage; artifact="$2"; shift 2 ;;
    --max-lines) [ $# -ge 2 ] || usage; max_lines="$2"; shift 2 ;;
    --execute) execute=1; shift ;;
    -*) echo "error: 不明な引数: $1" >&2; usage ;;
    *)
      if [ -z "$pr" ]; then pr="$1"
      elif [ -z "$report" ]; then report="$1"
      else usage
      fi
      shift ;;
  esac
done
[ -n "$pr" ] && [ -n "$report" ] || usage
[[ "$pr" =~ ^[1-9][0-9]*$ ]] || { echo "error: PR の番号が数でない: $pr" >&2; exit 2; }
[[ "$max_lines" =~ ^[1-9][0-9]*$ ]] || { echo "error: --max-lines は正の数: $max_lines" >&2; exit 2; }
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "error: --repo owner/name か GITHUB_REPOSITORY が要る: '$repo'" >&2; exit 2; }
[ -f "$report" ] || { echo "error: 表のファイルが無い: $report" >&2; exit 2; }
case "$artifact" in '' | https://*) ;; *) echo "error: --artifact-url は https:// で始まる: $artifact" >&2; exit 2 ;; esac
command -v jq >/dev/null || { echo "error: jq が要る" >&2; exit 2; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
body="$tmp/body.md"

# 本文: 印・表(長すぎれば上から max_lines 行)・切った時は全部へのリンク
total="$(awk 'END { print NR }' "$report")"
{
  printf '%s\n' "$MARKER"
  if [ "$total" -le "$max_lines" ]; then
    cat "$report"
  else
    head -n "$max_lines" "$report"
    # 切った所が <details> の中なら閉じる
    opened="$(head -n "$max_lines" "$report" | grep -c '<details>' || true)"
    closed="$(head -n "$max_lines" "$report" | grep -c '</details>' || true)"
    [ "$opened" -le "$closed" ] || printf '\n</details>\n'
    printf '\n_表が長いので上から %s 行(全 %s 行)。' "$max_lines" "$total"
    if [ -n "$artifact" ]; then
      printf '全部は [CI の成果物](%s)。_\n' "$artifact"
    else
      printf '全部は CI の成果物の report を見る。_\n'
    fi
  fi
} >"$body"

# 前のコメント(印付きの最後の 1 つ)
comments="$tmp/comments.json"
if ! gh api --paginate "repos/$repo/issues/$pr/comments" --jq '.[] | {id, body}' >"$comments"; then
  echo "error: PR #$pr のコメントを読めない" >&2
  exit 1
fi
id="$(jq -rs --arg m "$MARKER" '[.[] | select(.body | startswith($m))] | last | .id // empty' "$comments")"

if [ -n "$id" ]; then
  action="書き換える: コメント $id"
else
  action="足す: PR #$pr に新しいコメント"
fi

if [ "$execute" -ne 1 ]; then
  echo "DRY-RUN ($repo) $action"
  echo "----"
  cat "$body"
  exit 0
fi

payload="$tmp/payload.json"
jq -n --rawfile b "$body" '{body: $b}' >"$payload"
if [ -n "$id" ]; then
  gh api -X PATCH "repos/$repo/issues/comments/$id" --input "$payload" >/dev/null || { echo "error: 書き換えに失敗" >&2; exit 1; }
else
  gh api -X POST "repos/$repo/issues/$pr/comments" --input "$payload" >/dev/null || { echo "error: 足すのに失敗" >&2; exit 1; }
fi
echo "EXECUTE ($repo) $action"
