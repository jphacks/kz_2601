#!/usr/bin/env bash
# iOS で文字の大きさを固定している所の一覧(#1789)。Dynamic Type に付いてこない所を見える化する。落とさない(数を出すだけ)。
# `.font(.system(size: …))` で、行に relativeTo が無く、size が @ScaledMetric の変数でもない物をファイルごとに数える。
# 絵(SF Symbols・同じ修飾の並びに Image(systemName:) がある)と文字に分けて出す。
#
# 使い方: scripts/ios-fixed-font-report.sh [ios のディレクトリ(既定: リポジトリの ios)]
set -euo pipefail
root="${1:-$(cd "$(dirname "$0")/.." && pwd)/ios}"
[ -d "$root" ] || { echo "ディレクトリが無い: $root" >&2; exit 2; }

python3 -I - "$root" <<'PY'
import os
import re
import sys

root = sys.argv[1]
SKIP_DIRS = {".build", "DerivedData", "build", ".git"}
CALL = ".font(.system(size:"
SCALED = re.compile(r"@ScaledMetric\b[^\n]*?\bvar\s+(\w+)")
IDENT = re.compile(r"(?:self\.)?([A-Za-z_]\w*)")


def size_arg(line, start):
    """`.font(.system(size:` の後ろの size の式(最初の , か ) まで)。"""
    depth = 0
    out = []
    for ch in line[start:]:
        if ch in "([":
            depth += 1
        elif ch in ")]":
            if depth == 0:
                break
            depth -= 1
        elif ch == "," and depth == 0:
            break
        out.append(ch)
    return "".join(out).strip()


def is_symbol(lines, i):
    """同じ修飾の並び(前の行が . で始まる間・その頭の行まで)に Image(systemName: があるか。"""
    j = i
    while True:
        if "Image(systemName:" in lines[j]:
            return True
        if j == 0 or not lines[j].lstrip().startswith("."):
            return False
        j -= 1


def scan(path):
    with open(path, encoding="utf-8", errors="replace") as f:
        lines = f.read().split("\n")
    scaled = set(SCALED.findall("\n".join(lines)))
    text = symbol = 0
    for i, line in enumerate(lines):
        stripped = line.lstrip()
        if stripped.startswith("//") or CALL not in line or "relativeTo" in line:
            continue
        fixed = False
        pos = line.find(CALL)
        while pos >= 0:
            m = IDENT.match(size_arg(line, pos + len(CALL)))
            if not (m and m.group(1) in scaled):
                fixed = True
            pos = line.find(CALL, pos + 1)
        if not fixed:
            continue
        if is_symbol(lines, i):
            symbol += 1
        else:
            text += 1
    return text, symbol


rows = []
for dirpath, dirnames, filenames in os.walk(root):
    dirnames[:] = sorted(d for d in dirnames if d not in SKIP_DIRS)
    for name in sorted(filenames):
        if name.endswith(".swift"):
            path = os.path.join(dirpath, name)
            text, symbol = scan(path)
            if text or symbol:
                rows.append((os.path.relpath(path, os.path.dirname(os.path.abspath(root))), text, symbol))

rows.sort(key=lambda r: (-(r[1] + r[2]), r[0]))
print("文字\t絵\tファイル")
for path, text, symbol in rows:
    print(f"{text}\t{symbol}\t{path}")
total_text = sum(r[1] for r in rows)
total_symbol = sum(r[2] for r in rows)
print(f"合計: 文字 {total_text} 行・絵 {total_symbol} 行({len(rows)} ファイル)")
PY
