#!/usr/bin/env bash
# backend の go test -cover で 0.0% の関数を一覧に出す(#3268)。落とさない(常に exit 0)。
# エミュレータが無いと回らない物は別の段に分ける:
#   ファイル名に firestore を含む / ファイルの中に FIRESTORE_EMULATOR_HOST が出てくる
# 使い方:
#   scripts/go-cover-zero.sh [<go test のパッケージ>...]   既定 ./... を backend/ で回す(エミュレータ・ネットワーク無し)
#   scripts/go-cover-zero.sh --func <file>                  go tool cover -func の出力を読むだけ(回さない。テスト用)
# 出力(標準出力): [0% の関数] と [0% のうちエミュレータが要る物] の 2 段。形は「<ファイル>:<行>\t<関数>」。
# BACKEND_DIR で backend の場所を差し替えられる(--func の時にファイルの中身を見る先)。
set -euo pipefail

BACKEND="${BACKEND_DIR:-$(cd "$(dirname "$0")/../backend" && pwd)}"
func_file=""
pkgs=()
while [ $# -gt 0 ]; do
  case "$1" in
    -h | --help) sed -n '2,10p' "$0"; exit 0 ;;
    --func) [ $# -ge 2 ] || { echo "usage: $0 [--func <file>] [<pkg>...]" >&2; exit 0; }; func_file="$2"; shift 2 ;;
    *) pkgs+=("$1"); shift ;;
  esac
done

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [ -z "$func_file" ]; then
  [ ${#pkgs[@]} -gt 0 ] || pkgs=(./...)
  # テストの失敗では止めない(出せる分だけ出す)
  (cd "$BACKEND" && go test -count=1 -coverprofile="$tmp/cover.out" "${pkgs[@]}" >"$tmp/test.log" 2>&1) || echo "go-cover-zero: go test が失敗した(出せる分だけ出す)。ログ: 末尾 5 行" >&2
  [ -s "$tmp/test.log" ] && tail -n 5 "$tmp/test.log" >&2
  if [ ! -s "$tmp/cover.out" ]; then
    echo "go-cover-zero: cover の出力が無い" >&2
    exit 0
  fi
  (cd "$BACKEND" && go tool cover -func="$tmp/cover.out") >"$tmp/func.txt"
  func_file="$tmp/func.txt"
fi
[ -f "$func_file" ] || { echo "go-cover-zero: ファイルが無い: $func_file" >&2; exit 0; }

plain=() emu=()
# 行の形: <module>/<path>/<file>.go:<行>:<TAB><関数><TAB><割合>
while IFS=$'\t' read -r loc fn pct; do
  [ "${pct//[[:space:]]/}" = "0.0%" ] || continue
  loc="${loc%:}"          # 末尾の : を落とす(<file>:<行>:)
  file="${loc%:*}"        # <module>/<path>/<file>.go
  rel="${file#*/backend/}" # backend からの相対(無ければそのまま)
  fn="${fn//[[:space:]]/}"
  row="$rel:${loc##*:}"$'\t'"$fn"
  if [[ "$rel" == *firestore* ]] || grep -qs 'FIRESTORE_EMULATOR_HOST' "$BACKEND/$rel" 2>/dev/null; then
    emu+=("$row")
  else
    plain+=("$row")
  fi
done <"$func_file"

echo "[0% の関数] ${#plain[@]} 件"
printf '%s\n' ${plain[@]+"${plain[@]}"}
echo "[0% のうちエミュレータが要る物(firestore のファイル・FIRESTORE_EMULATOR_HOST を使うファイル)] ${#emu[@]} 件"
printf '%s\n' ${emu[@]+"${emu[@]}"}
exit 0
