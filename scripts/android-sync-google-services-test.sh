#!/usr/bin/env bash
# android/scripts/sync-google-services.sh のテスト(#2856)。android/ は変えずに、一時フォルダの git リポへ
# スクリプトと android/.gitignore を写して回す。secrets は METAMON_SECRETS_DIR で偽のフォルダに向ける。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# 一時リポ: <repo>/android/scripts/sync-google-services.sh。ignore=yes なら android/.gitignore も写す
make_repo() { # 名前 ignore(yes/no)
  local repo="$TMP/$1"
  mkdir -p "$repo/android/scripts"
  git -C "$repo" init -q
  cp "$ROOT/android/scripts/sync-google-services.sh" "$repo/android/scripts/"
  if [ "$2" = yes ]; then cp "$ROOT/android/.gitignore" "$repo/android/.gitignore"; fi
  printf '%s\n' "$repo/android"
}
secrets() { # 名前 置くファイル...
  local dir="$TMP/secrets-$1"; shift
  mkdir -p "$dir"
  local f
  for f in "$@"; do printf '{"name":"%s"}\n' "$f" >"$dir/$f"; done
  printf '%s\n' "$dir"
}
run() { # android フォルダ secrets フォルダ
  set +e
  OUT="$(METAMON_SECRETS_DIR="$2" sh "$1/scripts/sync-google-services.sh" 2>&1)"
  CODE=$?
  set -e
}
fail() { printf '%s\n' "$OUT" >&2; echo "FAIL: $1" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
same() { cmp -s "$1" "$2" || fail "$2 の中身が $1 と違う"; }

# 1. prod だけ: prod へコピーし、stg は消して警告だけ
A="$(make_repo prod-only yes)"
S="$(secrets prod-only google-services.json)"
mkdir -p "$A/app/src/stg" && echo old >"$A/app/src/stg/google-services.json"
run "$A" "$S"
[ "$CODE" = 0 ] || fail "prod だけ: 終了コード 0 のはずが $CODE"
same "$S/google-services.json" "$A/app/src/prod/google-services.json"
[ ! -e "$A/app/src/stg/google-services.json" ] || fail "prod だけ: 前の stg の json が残っている"
has "copied -> $A/app/src/prod/google-services.json (git-ignored)"
has "stg の json は無し"

# 2. prod と stg: 両方を flavor ごとのフォルダへ
A="$(make_repo both yes)"
S="$(secrets both google-services.json google-services-stg.json)"
run "$A" "$S"
[ "$CODE" = 0 ] || fail "prod と stg: 終了コード 0 のはずが $CODE"
same "$S/google-services.json" "$A/app/src/prod/google-services.json"
same "$S/google-services-stg.json" "$A/app/src/stg/google-services.json"
has "copied -> $A/app/src/stg/google-services.json (git-ignored)"

# 3. prod が無い: 1 で止まり、何もコピーしない
A="$(make_repo no-prod yes)"
S="$(secrets no-prod google-services-stg.json)"
run "$A" "$S"
[ "$CODE" = 1 ] || fail "prod が無い: 終了コード 1 のはずが $CODE"
has "google-services.json がありません"
[ ! -e "$A/app/src/stg/google-services.json" ] || fail "prod が無い: stg をコピーしている"

# 4. コピー先が .gitignore されていない: 消して 1 で止まる
A="$(make_repo not-ignored no)"
S="$(secrets not-ignored google-services.json google-services-stg.json)"
run "$A" "$S"
[ "$CODE" = 1 ] || fail "ignore 無し: 終了コード 1 のはずが $CODE"
has ".gitignore されていません"
[ ! -e "$A/app/src/prod/google-services.json" ] || fail "ignore 無し: コピー先を消していない"
[ ! -e "$A/app/src/stg/google-services.json" ] || fail "ignore 無し: prod で止まらず stg まで進んだ"

echo "OK: android-sync-google-services-test"
