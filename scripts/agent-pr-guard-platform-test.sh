#!/usr/bin/env bash
# scripts/agent-pr-guard.sh の desktop/ と android/ の変更の振り分けを使い捨ての git リポジトリで確かめる(#2865・ネットワーク不要)。
# 本物の一覧(agent-pr-guard.paths)を使う。一覧の決まりは変えず、今の一覧での振り分けを固定する:
#   desktop/ は種類の範囲に入らない(秘密の形のファイルだけ止める)
#   android/ は認可(auth)・課金(billing)の main のコードだけ止め、テストやほかの画面は通す
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
GUARD="$HERE/agent-pr-guard.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
hasnt() { ! grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」がある"; }
expect_code() { # 期待コード 説明 args...
  local want="$1" what="$2" code; shift 2
  set +e; OUT="$(cd "$TMP/repo" && "$GUARD" "$@" 2>&1)"; code=$?; set -e
  [ "$code" = "$want" ] || fail "$what: 終了コード $want のはずが $code"
}

g() { git -C "$TMP/repo" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false "$@"; }
put() { mkdir -p "$TMP/repo/$(dirname "$1")"; printf '%s\n' "${2:-x}" >"$TMP/repo/$1"; }
branch() {
  local name="$1"; shift
  g checkout -q -b "$name" base
  for f in "$@"; do put "$f"; done
  g add -A && g commit -q -m "$name"
}

A=android/app/src/main/kotlin/jp/kz2601/metamon
AS=android/services/src/main/kotlin/jp/kz2601/metamon/services
D=desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop

git init -q "$TMP/repo"
put README.md
g add -A && g commit -q -m base && g branch base

# 1. desktop だけ: コードもビルドも README も通す
branch desktop-only "$D/A11y.kt" "$D/auth/SignIn.kt" "$D/purchase/Paywall.kt" desktop/app/build.gradle.kts desktop/README.md
expect_code 0 "desktop だけ" base desktop-only
has "変更 5 件"
has "OK:"

# 1b. desktop でも秘密の形のファイルは止める
branch desktop-secret "$D/A11y.kt" desktop/app/google-services.json desktop/.env
expect_code 1 "desktop の秘密" base desktop-secret
has "(2 件)"
has "secret	desktop/app/google-services.json"
has "secret	desktop/.env"
hasnt "A11y.kt"

# 2. android だけ: 認可・課金の main は止め、テスト・ほかの画面・ビルドは通す
branch android-only \
  "$A/auth/AuthSession.kt" \
  "$A/purchase/PlayBilling.kt" \
  "$AS/AuthService.kt" \
  "$AS/live/FirebaseAuthService.kt" \
  "$AS/purchase/Receipt.kt" \
  "$A/home/HomeScreen.kt" \
  android/app/src/test/kotlin/jp/kz2601/metamon/auth/AuthSessionTest.kt \
  "$AS/live/FirestoreService.kt" \
  android/app/build.gradle.kts
expect_code 1 "android だけ" base android-only
has "変更 9 件"
has "(5 件)"
has "auth	$A/auth/AuthSession.kt"
has "billing	$A/purchase/PlayBilling.kt"
has "auth	$AS/AuthService.kt"
has "auth	$AS/live/FirebaseAuthService.kt"
has "billing	$AS/purchase/Receipt.kt"
hasnt "HomeScreen.kt"
hasnt "AuthSessionTest.kt"
hasnt "FirestoreService.kt"
hasnt "build.gradle.kts"

# 2b. android の止めない場所だけなら通す
branch android-clean "$A/home/HomeScreen.kt" android/app/src/test/kotlin/jp/kz2601/metamon/auth/AuthSessionTest.kt android/dsl/x.kt
expect_code 0 "android の止めない場所" base android-clean
has "変更 3 件"

# 3. 両方: android の止める場所だけ出し、desktop は出さない
branch both "$D/A11y.kt" "$D/auth/SignIn.kt" "$A/auth/GoogleSignIn.kt" "$A/home/HomeScreen.kt"
expect_code 1 "両方" base both
has "変更 4 件"
has "(1 件)"
has "auth	$A/auth/GoogleSignIn.kt"
hasnt "desktop/"

# 3b. 両方とも止めない場所なら通す
branch both-clean "$D/A11y.kt" "$A/home/HomeScreen.kt"
expect_code 0 "両方とも止めない場所" base both-clean

# 4. docs と一緒: 正典・法務は docs として止め、ほかの docs は通す
branch with-docs "$D/A11y.kt" "$A/purchase/ProStore.kt" docs/00_decisions.md docs/legal/privacy.md docs/desktop/setup.md docs/android/setup.md
expect_code 1 "docs と一緒" base with-docs
has "変更 6 件"
has "(3 件)"
has "billing	$A/purchase/ProStore.kt"
has "docs	docs/00_decisions.md"
has "docs	docs/legal/privacy.md"
hasnt "docs/desktop/setup.md"
hasnt "docs/android/setup.md"
hasnt "A11y.kt"

# 4b. desktop・android の止めない場所と普通の docs だけなら通す
branch with-docs-clean "$D/A11y.kt" "$A/home/HomeScreen.kt" docs/desktop/setup.md docs/android/setup.md
expect_code 0 "普通の docs と一緒" base with-docs-clean

echo "ok"
