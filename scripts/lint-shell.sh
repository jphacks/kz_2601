#!/usr/bin/env bash
# CI(scripts-ci)と同じ版の shellcheck を手元で回す(#2866)。
# 版は .github/workflows/scripts-ci.yml の SHELLCHECK_VERSION から読む。PATH の shellcheck が同じ版ならそれを使い、
# 違う・無い時は公式 Release から一時フォルダに取ってきて使い、終わったら消す。
# 使い方:
#   scripts/lint-shell.sh               # 追跡中の *.sh 全部(CI と同じ)
#   scripts/lint-shell.sh a.sh b.sh     # 渡したファイルだけ
# 終了コード: shellcheck の結果(0 指摘なし / 1 指摘あり)。2 は版が読めない・取ってこられないなど
set -euo pipefail

ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
WF="$ROOT/.github/workflows/scripts-ci.yml"
VERSION="$(sed -n 's/^[[:space:]]*SHELLCHECK_VERSION:[[:space:]]*"\{0,1\}\([0-9][0-9.]*\)"\{0,1\}[[:space:]]*$/\1/p' "$WF" 2>/dev/null | head -n 1)"
[ -n "$VERSION" ] || { echo "lint-shell: $WF から SHELLCHECK_VERSION が読めない" >&2; exit 2; }

version_of() { "$1" --version 2>/dev/null | sed -n 's/^version:[[:space:]]*//p'; }

TMP=""
trap '[ -z "$TMP" ] || rm -rf "$TMP"' EXIT

SC=""
if command -v shellcheck >/dev/null && [ "$(version_of shellcheck)" = "$VERSION" ]; then
  SC="$(command -v shellcheck)"
else
  case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) echo "lint-shell: この OS は未対応: $(uname -s)" >&2; exit 2 ;; esac
  case "$(uname -m)" in arm64 | aarch64) arch=aarch64 ;; x86_64 | amd64) arch=x86_64 ;; *) echo "lint-shell: この CPU は未対応: $(uname -m)" >&2; exit 2 ;; esac
  have="$(command -v shellcheck >/dev/null && version_of shellcheck || true)"
  echo "lint-shell: 手元の shellcheck は ${have:-無し}。CI と同じ v$VERSION を一時フォルダに取ってくる" >&2
  TMP="$(mktemp -d "${TMPDIR:-/tmp}/lint-shell.XXXXXX")"
  url="https://github.com/koalaman/shellcheck/releases/download/v${VERSION}/shellcheck-v${VERSION}.${os}.${arch}.tar.xz"
  curl -fsSL --retry 3 --retry-delay 2 -o "$TMP/shellcheck.tar.xz" "$url" \
    || { echo "lint-shell: 取得に失敗: $url" >&2; exit 2; }
  tar xJf "$TMP/shellcheck.tar.xz" -C "$TMP" || { echo "lint-shell: 展開に失敗: $url" >&2; exit 2; }
  SC="$TMP/shellcheck-v${VERSION}/shellcheck"
  [ -x "$SC" ] && [ "$(version_of "$SC")" = "$VERSION" ] || { echo "lint-shell: 取ってきた shellcheck の版が v$VERSION でない" >&2; exit 2; }
fi

echo "lint-shell: shellcheck v$VERSION ($SC)" >&2
cd "$ROOT"
status=0
if [ $# -gt 0 ]; then
  "$SC" "$@" || status=$?
else
  git ls-files -z '*.sh' | xargs -0 "$SC" || status=$?
  # xargs はコマンドの失敗を 123 にする。shellcheck の 1(指摘あり)に揃える
  [ "$status" != 123 ] || status=1
fi
[ "$status" = 0 ] && echo "lint-shell: OK" >&2
exit "$status"
