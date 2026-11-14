#!/usr/bin/env bash
# scripts/lint-shell.sh を使い捨ての git リポジトリと偽の shellcheck・curl で確かめる(#2866・ネットワーク不要)。
#   偽の shellcheck: --version で $FAKE_SC_VERSION を名乗り、受けた引数を $FAKE_STATE/sc-args に書く。$FAKE_SC_EXIT で終わる
#   偽の curl: -o の先に、偽の shellcheck(v$FAKE_DL_VERSION)を入れた tar.xz を作る。URL を $FAKE_STATE/url に書く
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/state" "$TMP/tmpdir" "$TMP/bin-same" "$TMP/bin-old" "$TMP/fake"

fake_sc() { # 書き出す先 名乗る版
  cat >"$1" <<EOF
#!/usr/bin/env bash
if [ "\${1:-}" = --version ]; then printf 'ShellCheck - shell script analysis tool\nversion: %s\nlicense: x\n' "$2"; exit 0; fi
printf '%s\n' "\$@" >"\$FAKE_STATE/sc-args"
exit "\${FAKE_SC_EXIT:-0}"
EOF
  chmod +x "$1"
}
fake_sc "$TMP/bin-same/shellcheck" 9.9.9
fake_sc "$TMP/bin-old/shellcheck" 0.1.0
cat >"$TMP/fake/curl" <<'EOF'
#!/usr/bin/env bash
out="" url=""
while [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift 2 ;; --retry | --retry-delay) shift 2 ;; -*) shift ;; *) url="$1"; shift ;; esac; done
echo "$url" >"$FAKE_STATE/url"
[ "${FAKE_CURL_FAIL:-}" = 1 ] && exit 22
d="$(mktemp -d)"
mkdir -p "$d/shellcheck-v$FAKE_DL_VERSION"
cp "$FAKE_SC_SRC" "$d/shellcheck-v$FAKE_DL_VERSION/shellcheck"
tar cJf "$out" -C "$d" "shellcheck-v$FAKE_DL_VERSION"
rm -rf "$d"
EOF
chmod +x "$TMP/fake/curl"
# curl が取ってくる偽の shellcheck(v9.9.9)
fake_sc "$TMP/state/dl-shellcheck" 9.9.9

fail() { printf '%s\n' "${OUT:-}" >&2; echo "FAIL: $*" >&2; exit 1; }
has() { grep -qF -- "$1" <<<"$OUT" || fail "出力に「$1」が無い"; }
run() { # 期待コード <PATH の前に置く dir> args...
  local want="$1" bin="$2" code; shift 2
  rm -f "$TMP/state/sc-args" "$TMP/state/url"
  set +e
  OUT="$(cd "$TMP/repo" && PATH="$bin:$TMP/fake:$PATH" FAKE_STATE="$TMP/state" FAKE_SC_SRC="$TMP/state/dl-shellcheck" \
    FAKE_DL_VERSION="${FAKE_DL_VERSION:-9.9.9}" TMPDIR="$TMP/tmpdir" scripts/lint-shell.sh "$@" 2>&1)"
  code=$?
  set -e
  [ "$code" = "$want" ] || fail "$*: 終了コード $want のはずが $code"
  [ -z "$(ls -A "$TMP/tmpdir")" ] || fail "一時フォルダが残った: $(ls -A "$TMP/tmpdir")"
}

git init -q "$TMP/repo"
mkdir -p "$TMP/repo/scripts" "$TMP/repo/.github/workflows" "$TMP/repo/sub dir"
cp "$HERE/lint-shell.sh" "$TMP/repo/scripts/"
printf 'env:\n  SHELLCHECK_VERSION: "9.9.9"\n  ACTIONLINT_VERSION: "1.0.0"\n' >"$TMP/repo/.github/workflows/scripts-ci.yml"
: >"$TMP/repo/a.sh"
: >"$TMP/repo/sub dir/b.sh"
: >"$TMP/repo/untracked.sh"
: >"$TMP/repo/c.py"
git -C "$TMP/repo" add scripts a.sh "sub dir/b.sh" c.py .github

# 1. 手元の shellcheck が同じ版ならそれで追跡中の *.sh を全部回す(取ってこない)
run 0 "$TMP/bin-same"
[ ! -e "$TMP/state/url" ] || fail "同じ版なのに取ってきた"
has "lint-shell: shellcheck v9.9.9 ($TMP/bin-same/shellcheck)"
has "lint-shell: OK"
[ "$(sort "$TMP/state/sc-args")" = "$(printf 'a.sh\nscripts/lint-shell.sh\nsub dir/b.sh')" ] || fail "回した物: $(cat "$TMP/state/sc-args")"

# 2. 指摘があれば 1(xargs の 123 にしない)
FAKE_SC_EXIT=1 run 1 "$TMP/bin-same"
FAKE_SC_EXIT=1 run 1 "$TMP/bin-same" a.sh

# 3. ファイルを渡したらそれだけ
run 0 "$TMP/bin-same" a.sh "sub dir/b.sh"
[ "$(cat "$TMP/state/sc-args")" = "$(printf 'a.sh\nsub dir/b.sh')" ] || fail "渡した物: $(cat "$TMP/state/sc-args")"

# 4. 手元の版が違えば CI と同じ版を一時フォルダに取ってきて使い、終わったら消す
run 0 "$TMP/bin-old"
case "$(uname -s)" in Darwin) os=darwin ;; *) os=linux ;; esac
case "$(uname -m)" in arm64 | aarch64) arch=aarch64 ;; *) arch=x86_64 ;; esac
[ "$(cat "$TMP/state/url")" = "https://github.com/koalaman/shellcheck/releases/download/v9.9.9/shellcheck-v9.9.9.$os.$arch.tar.xz" ] \
  || fail "取ってくる URL: $(cat "$TMP/state/url")"
has "手元の shellcheck は 0.1.0。CI と同じ v9.9.9 を一時フォルダに取ってくる"
has "lint-shell: shellcheck v9.9.9 ($TMP/tmpdir/"
[ -s "$TMP/state/sc-args" ] || fail "取ってきた shellcheck で回していない"
FAKE_SC_EXIT=1 run 1 "$TMP/bin-old"

# 5. 取れない・取った物の版が違う時は 2(一時フォルダは消す)
FAKE_CURL_FAIL=1 run 2 "$TMP/bin-old"
has "lint-shell: 取得に失敗: https://github.com/koalaman/shellcheck/releases/download/v9.9.9/"
FAKE_DL_VERSION=1.2.3 run 2 "$TMP/bin-old" # 中のフォルダ名が違う
has "取ってきた shellcheck の版が v9.9.9 でない"
fake_sc "$TMP/state/dl-shellcheck" 8.8.8
run 2 "$TMP/bin-old"
has "取ってきた shellcheck の版が v9.9.9 でない"
fake_sc "$TMP/state/dl-shellcheck" 9.9.9

# 6. workflow から版が読めなければ 2
printf 'env:\n  ACTIONLINT_VERSION: "1.0.0"\n' >"$TMP/repo/.github/workflows/scripts-ci.yml"
run 2 "$TMP/bin-same"
has "SHELLCHECK_VERSION が読めない"

# 7. 本物の workflow の版が読める(取得はしない)
grep -q '^  SHELLCHECK_VERSION: "[0-9][0-9.]*"$' "$HERE/../.github/workflows/scripts-ci.yml" || { OUT=""; fail "scripts-ci.yml の SHELLCHECK_VERSION の形が変わった"; }

echo "ok"
