#!/usr/bin/env bash
# scripts/github/setup-branch-protection.sh --dry-run を偽の gh で確かめる(GitHub に何も送らない)。
# 偽の gh は読む呼び出し(repo view・GET の api)にだけ答え、呼び出しを全部 $FAKE_GH_LOG に書く。
# 書き込み(-X・-f・--input)が来たら WRITE と記録して失敗する。
set -euo pipefail
cd "$(dirname "$0")"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"

cat >"$TMP/bin/gh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_GH_LOG"
for a in "$@"; do
  case "$a" in -X|-f|-F|--input|--method) echo "WRITE $*" >>"$FAKE_GH_LOG"; exit 1 ;; esac
done
case "$*" in
  "repo view --json nameWithOwner --jq .nameWithOwner") echo "jphacks/kz_2601" ;;
  "api repos/"*"/branches/develop") [ "$FAKE_DEVELOP" = yes ] && echo '{"name":"develop"}' || exit 1 ;;
  "api repos/"*"/git/ref/heads/main --jq .object.sha") echo "0123456789abcdef0123456789abcdef01234567" ;;
  "api repos/"*" --jq .owner.type") echo "$FAKE_OWNER_TYPE" ;;
  *) echo "fake gh: 知らない呼び出し: $*" >&2; exit 1 ;;
esac
EOF
chmod +x "$TMP/bin/gh"

# run 所有者 develop有無 [env...] -- args...: 出力を $OUT・標準エラーを $ERR に
run() {
  local owner="$1" develop="$2"; shift 2
  local envs=()
  while [ $# -gt 0 ] && [ "$1" != -- ]; do envs+=("$1"); shift; done
  shift
  : >"$TMP/gh.log"
  set +e
  OUT="$(env -u REPO -u HUMAN_USERS -u STRICT_STATUS_CHECKS PATH="$TMP/bin:$PATH" FAKE_GH_LOG="$TMP/gh.log" \
    FAKE_OWNER_TYPE="$owner" FAKE_DEVELOP="$develop" ${envs[@]+"${envs[@]}"} \
    ./github/setup-branch-protection.sh "$@" 2>"$TMP/err")"
  CODE=$?
  set -e
  ERR="$(cat "$TMP/err")"
  LOG="$(cat "$TMP/gh.log")"
}
fail() { printf 'OUT:\n%s\nERR:\n%s\nGH:\n%s\n' "$OUT" "$ERR" "$LOG" >&2; echo "FAIL: $1" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
no_writes() {
  [ "$CODE" = 0 ] || fail "$1: 終了コード $CODE"
  ! grep -q '^WRITE' <<<"$LOG" || fail "$1: API に書き込む呼び出しがあった"
}
# payload ブランチ: 「---- protection for <ブランチ> ----」の後の JSON
payload() {
  awk -v h="---- protection for $1 ----" '$0 == h { on = 1; next } on && /^(----|done\.)/ { exit } on' <<<"$OUT"
}
# expect ブランチ jq式 期待値
expect() {
  local got
  got="$(payload "$1" | jq -c "$2")" || fail "$1 の payload が JSON でない"
  [ "$got" = "$3" ] || fail "$1: $2 = $got (期待 $3)"
}
common() { # 設定の中身は変えない(必須のチェック・admin も PR 経由・force push / 削除なし)
  local b="$1"
  expect "$b" '[.required_status_checks.checks[].context]' '["backend-ci","ios-ci","secrets-scan"]'
  expect "$b" '[.required_status_checks.checks[].app_id] | unique' '[15368]'
  expect "$b" '.enforce_admins' 'true'
  expect "$b" '.required_pull_request_reviews.required_approving_review_count' '0'
  expect "$b" '.required_conversation_resolution' 'true'
  expect "$b" '[.allow_force_pushes, .allow_deletions, .lock_branch]' '[false,false,false]'
}

# 1. organization・develop あり: main と develop に同じ payload・人間だけが push できる
run Organization yes -- --dry-run
no_writes "organization"
has "repo: jphacks/kz_2601"
has "owner type: Organization"
has "branch exists: develop"
for b in main develop; do
  common "$b"
  expect "$b" '.required_status_checks.strict' 'false'
  expect "$b" '.restrictions' '{"users":["ko-tarou","2410as"],"teams":[],"apps":[]}'
done
[ "$(payload main | jq -S .)" = "$(payload develop | jq -S .)" ] || fail "main と develop の payload が違う"
grep -qF "repo view --json nameWithOwner" <<<"$LOG" || fail "REPO 未指定なのに gh repo view を呼んでいない"

# 2. user 所有・develop なし: restrictions は null で警告・develop は作ると言うだけで作らない
run User no -- --dry-run
no_writes "user"
has "create branch: develop from main@0123456789abcdef0123456789abcdef01234567"
grep -qF "WARN: user 所有リポ" <<<"$ERR" || fail "user 所有の警告が無い"
for b in main develop; do
  common "$b"
  expect "$b" '.restrictions' 'null'
done

# 3. 環境変数: REPO・HUMAN_USERS・STRICT_STATUS_CHECKS
run Organization yes REPO=o/r 'HUMAN_USERS=alice bob carol' STRICT_STATUS_CHECKS=true -- --dry-run
no_writes "env"
has "repo: o/r"
! grep -qF "repo view" <<<"$LOG" || fail "REPO 指定時に gh repo view を呼んだ"
grep -qF "api repos/o/r --jq .owner.type" <<<"$LOG" || fail "REPO の値で owner を見ていない"
expect main '.required_status_checks.strict' 'true'
expect main '.restrictions.users' '["alice","bob","carol"]'
common main

# 4. 知らない引数は終了コード 2 で gh を呼ばない
run Organization yes -- --dryrun
[ "$CODE" = 2 ] || fail "知らない引数: 終了コード $CODE"
[ -z "$LOG" ] || fail "知らない引数なのに gh を呼んだ"

echo "OK: setup-branch-protection --dry-run"
