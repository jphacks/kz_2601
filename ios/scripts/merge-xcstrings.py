#!/usr/bin/env python3
"""git の merge driver: .xcstrings をキー単位で 3-way マージする。

使い方(git が呼ぶ): python3 ios/scripts/merge-xcstrings.py %O %A %B
  %O=共通祖先 %A=自分(結果をここに書く) %B=相手
片側だけが変えたキーはその変更を採る。両側が同じキーを違う内容に変えた時だけ exit 1
(%A には自分側を残すので、手で直して整形し直す)。結果は xcstrings-fmt.py と同じ書式。
"""
import importlib.util
import json
import os
import sys

sys.dont_write_bytecode = True  # repo に __pycache__ を残さない
_spec = importlib.util.spec_from_file_location(
    "xcstrings_fmt", os.path.join(os.path.dirname(os.path.abspath(__file__)), "xcstrings-fmt.py")
)
_fmt = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_fmt)

_MISSING = object()


def merge3(base, ours, theirs, path=""):
    """dict を再帰的に 3-way マージ。(結果, 衝突したキーのパス一覧) を返す。"""
    conflicts = []
    out = {}
    for k in set(base) | set(ours) | set(theirs):
        b, o, t = base.get(k, _MISSING), ours.get(k, _MISSING), theirs.get(k, _MISSING)
        p = f"{path}/{k}"
        if o == t:
            v = o
        elif o == b:
            v = t
        elif t == b:
            v = o
        elif all(isinstance(x, dict) for x in (o, t)) and (b is _MISSING or isinstance(b, dict)):
            v, sub = merge3({} if b is _MISSING else b, o, t, p)
            conflicts += sub
        else:
            conflicts.append(p)
            v = o
        if v is not _MISSING:
            out[k] = v
    return out, conflicts


def _load(path):
    try:
        with open(path, encoding="utf-8") as f:
            text = f.read()
    except FileNotFoundError:
        return {}
    return json.loads(text) if text.strip() else {}


def main(argv):
    if len(argv) != 3:
        print(__doc__.strip(), file=sys.stderr)
        return 2
    o_path, a_path, b_path = argv
    try:
        base, ours, theirs = _load(o_path), _load(a_path), _load(b_path)
    except json.JSONDecodeError as e:
        print(f"merge-xcstrings: JSON として読めません ({e})", file=sys.stderr)
        return 1
    merged, conflicts = merge3(base, ours, theirs)
    with open(a_path, "w", encoding="utf-8") as f:
        f.write(_fmt.format_obj(merged))
    if conflicts:
        print("merge-xcstrings: 両側で違う変更があるキー(自分側を残しました):", file=sys.stderr)
        for c in conflicts:
            print(f"  {c}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
