#!/usr/bin/env bash
# scripts/ai-eval-needed.sh を使い捨ての git リポジトリで確かめる(ネットワーク不要)。
# 本物の一覧(ai-eval-needed.paths)を使い、一覧の各行が今のリポジトリに実在するかも確かめる。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/ai-eval-needed.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }
run() { # 期待コード 説明 args...
  local want="$1" what="$2" code; shift 2
  set +e; OUT="$(cd "$TMP/repo" && "$SCRIPT" "$@" 2>&1)"; code=$?; set -e
  [ "$code" = "$want" ] || fail "$what: 終了コード $want のはずが $code"
}
# 標準出力だけ(GITHUB_OUTPUT に足す部分)
stdout() { (cd "$TMP/repo" && "$SCRIPT" "$@" 2>/dev/null); }

g() { git -C "$TMP/repo" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false "$@"; }
put() { mkdir -p "$TMP/repo/$(dirname "$1")"; printf '%s\n' "${2:-x}" >"$TMP/repo/$1"; }
branch() {
  local name="$1"; shift
  g checkout -q -b "$name" base
  for f in "$@"; do put "$f"; done
  g add -A && g commit -q -m "$name"
}

git init -q "$TMP/repo"
put README.md
put backend/internal/ai/prompt.go
put docs/dsl/dsl.schema.json "{}"
g add -A && g commit -q -m base && g branch base

# 1. 関わらない差分(テスト・ドキュメント・ほかの場所)
branch clean backend/internal/ai/prompt_test.go backend/pkg/dsl/x_test.go docs/backend/x.md ios/A.swift backend/internal/store/s.go
run 0 "関わらない" base clean
[ "$(stdout base clean)" = "$(printf 'needed=false\nmax_cases=0\nmatched=0')" ] || fail "関わらない時の標準出力"
has "生成に関わる差分なし"

# 2. 種類ごとに関わる(テストは数えない)
branch touch \
  backend/internal/ai/rules.go \
  backend/internal/ai/ev/new_cases.json \
  backend/cmd/ai-eval/main.go \
  backend/pkg/dsl/validate.go \
  docs/dsl/dsl.schema.json \
  backend/internal/ai/rules_test.go
run 0 "関わる" base touch
has "needed=true"
has "max_cases=20"
has "matched=5"
has "ai	backend/internal/ai/rules.go"
has "ai	backend/internal/ai/ev/new_cases.json"
has "ai	backend/cmd/ai-eval/main.go"
has "dsl	backend/pkg/dsl/validate.go"
has "schema	docs/dsl/dsl.schema.json"
hasnt "rules_test.go"

# 3. 1 件だけでも回す・上限は引数と環境変数で変えられる
branch one backend/pkg/dsl/one.go
[ "$(stdout --max-cases 5 base one)" = "$(printf 'needed=true\nmax_cases=5\nmatched=1')" ] || fail "--max-cases 5"
[ "$(stdout --max-cases=7 base one | sed -n 2p)" = "max_cases=7" ] || fail "--max-cases=7"
[ "$( (cd "$TMP/repo" && AI_EVAL_MAX_CASES=3 "$SCRIPT" base one 2>/dev/null) | sed -n 2p)" = "max_cases=3" ] || fail "AI_EVAL_MAX_CASES"

# 4. 既存の変更・削除・名前の変更(前の名前も後の名前も)
g checkout -q -b moved base
mkdir -p "$TMP/repo/backend/internal/x"
g mv backend/internal/ai/prompt.go backend/internal/x/prompt.go
g commit -q -m moved
run 0 "名前の変更(前の名前)" base moved
has "needed=true"
has "ai	backend/internal/ai/prompt.go"
g checkout -q -b removed base
g rm -q docs/dsl/dsl.schema.json
g commit -q -m removed
run 0 "削除" base removed
has "schema	docs/dsl/dsl.schema.json"

# 5. base 側だけの変更は数えない(base...head)
g checkout -q base
put backend/pkg/dsl/later.go && g add -A && g commit -q -m later
run 0 "base 側の変更" base clean
has "needed=false"
g checkout -q -B base HEAD~1

# 6. 似た名前は関わらない
branch similar backend/internal/aix/a.go backend/pkg/dslx/b.go docs/dsl/README.md backend/cmd/ai/main.go
run 0 "似た名前" base similar
has "needed=false"

# 7. 引数の誤り
run 2 "無いコミット" base no-such-ref
run 2 "不明な引数" --bogus
run 2 "引数が多い" base clean extra
run 2 "上限が数でない" --max-cases x base clean
run 2 "上限が 0" --max-cases 0 base clean
printf 'ai\n' >"$TMP/bad.paths"
set +e; OUT="$(cd "$TMP/repo" && AI_EVAL_NEEDED_PATHS="$TMP/bad.paths" "$SCRIPT" base clean 2>&1)"; code=$?; set -e
[ "$code" = 2 ] || fail "形の違う一覧: 終了コード 2 のはずが $code"
has "一覧の行の形が違う"

# 8. 一覧の各行がこのリポジトリに実在する(skip は除く)
ROOT="$(cd "$HERE/.." && pwd)"
tracked="$(git -C "$ROOT" ls-files)"
missing=""
while read -r kind pat _; do
  case "$kind" in '' | '#'* | skip) continue ;; esac
  case "$pat" in */) pat="$pat*" ;; esac
  found=""
  while IFS= read -r f; do
    # shellcheck disable=SC2053
    if [[ "$f" == $pat ]]; then found=1; break; fi
  done <<<"$tracked"
  [ -n "$found" ] || missing="$missing $pat"
done <"$HERE/ai-eval-needed.paths"
[ -z "$missing" ] || { OUT=""; fail "一覧にあるが repo に無い:$missing"; }

echo "ok"
