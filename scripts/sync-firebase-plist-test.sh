#!/usr/bin/env bash
# ios/scripts/sync-firebase-plist.sh を使い捨ての git リポジトリで確かめる(#2860・ネットワーク・本物の secrets 不要)。
# スクリプトと本物の .gitignore を一時フォルダの ios/scripts/ へ写し、偽の secrets フォルダから置かせる。
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }

# 新しい repo を作る。$1 = .gitignore の元(空なら .gitignore 無し)
setup() {
  rm -rf "$TMP/repo" "$TMP/secrets"
  mkdir -p "$TMP/repo/ios/scripts" "$TMP/repo/ios/Metamon/Resources" "$TMP/secrets"
  git init -q "$TMP/repo"
  cp "$ROOT/ios/scripts/sync-firebase-plist.sh" "$TMP/repo/ios/scripts/"
  [ -z "$1" ] || cp "$1" "$TMP/repo/.gitignore"
}
secret() { printf '<plist><dict><key>NAME</key><string>%s</string></dict></plist>\n' "$1" >"$TMP/secrets/$1"; }
expect_code() { # 期待コード 説明
  local want="$1" what="$2" code
  set +e; OUT="$(METAMON_SECRETS_DIR="$TMP/secrets" sh "$TMP/repo/ios/scripts/sync-firebase-plist.sh" 2>&1)"; code=$?; set -e
  [ "$code" = "$want" ] || fail "$what: 終了コード $want のはずが $code"
}
FB="$TMP/repo/ios/Firebase"

# 1. prod だけ: prod を置き、stg は置かない(前の stg の残りは消す)
setup "$ROOT/.gitignore"
secret GoogleService-Info.plist
mkdir -p "$FB" && echo old >"$FB/GoogleService-Info-Staging.plist"
echo old >"$TMP/repo/ios/Metamon/Resources/GoogleService-Info.plist"
expect_code 0 "prod だけ"
cmp -s "$TMP/secrets/GoogleService-Info.plist" "$FB/GoogleService-Info.plist" || fail "prod の plist が写っていない"
[ ! -e "$FB/GoogleService-Info-Staging.plist" ] || fail "前の stg の plist が残っている"
[ ! -e "$TMP/repo/ios/Metamon/Resources/GoogleService-Info.plist" ] || fail "旧置き場のコピーが残っている"
has "copied -> $FB/GoogleService-Info.plist (git-ignored)"
has "stg の plist は無し"
[ -z "$(git -C "$TMP/repo" status --porcelain --untracked-files=all -- ios/Firebase)" ] || fail "置いた plist が git に見えている"

# 2. 両方: prod と stg を置く
secret GoogleService-Info-Staging.plist
expect_code 0 "両方"
cmp -s "$TMP/secrets/GoogleService-Info-Staging.plist" "$FB/GoogleService-Info-Staging.plist" || fail "stg の plist が写っていない"
has "copied -> $FB/GoogleService-Info-Staging.plist (git-ignored)"
[ -z "$(git -C "$TMP/repo" status --porcelain --untracked-files=all -- ios/Firebase)" ] || fail "置いた plist が git に見えている"

# 3. prod が無い: stg があっても止まり、何も置かない
setup "$ROOT/.gitignore"
secret GoogleService-Info-Staging.plist
expect_code 1 "prod が無い"
has "GoogleService-Info.plist がありません"
[ -z "$(ls -A "$FB")" ] || fail "prod が無いのに置いた: $(ls -A "$FB")"

# 4. コピー先が ignore されていない: 止まり、置いた物を消す
setup ""
secret GoogleService-Info.plist
expect_code 1 "ignore されていない"
has ".gitignore されていません"
[ ! -e "$FB/GoogleService-Info.plist" ] || fail "ignore されていないのにコピーが残っている"

echo "ok"
