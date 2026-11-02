#!/usr/bin/env python3
"""String Catalog (.xcstrings) を Xcode と同じ書式に整える。

Xcode の書き出しと同じ: キーをコードポイント順に並べる・インデント 2・`"key" : value`・
空のオブジェクトは `{\n\n<indent>}`・末尾改行なし。Xcode で保存しても差分が出ない。

使い方: python3 ios/scripts/xcstrings-fmt.py [--check] <files...>
  --check  書き換えずに検査。整っていないファイルがあれば列挙して exit 1。
"""
import json
import sys


def _dump(v, level):
    pad = "  " * (level + 1)
    end = "  " * level
    if isinstance(v, dict):
        if not v:
            return "{\n\n" + end + "}"
        items = [
            pad + json.dumps(k, ensure_ascii=False) + " : " + _dump(v[k], level + 1)
            for k in sorted(v)
        ]
        return "{\n" + ",\n".join(items) + "\n" + end + "}"
    if isinstance(v, list):
        if not v:
            return "[\n\n" + end + "]"
        return "[\n" + ",\n".join(pad + _dump(x, level + 1) for x in v) + "\n" + end + "]"
    return json.dumps(v, ensure_ascii=False)


def format_obj(obj):
    return _dump(obj, 0)


def _no_conflicting_dups(pairs):
    out = {}
    for k, v in pairs:
        if k in out and out[k] != v:
            raise ValueError(f"重複キーの中身が違います: {k!r}")
        out[k] = v  # 中身が同じ重複は 1 つにまとめる
    return out


def format_text(text):
    return format_obj(json.loads(text, object_pairs_hook=_no_conflicting_dups))


def main(argv):
    check = "--check" in argv
    files = [a for a in argv if a != "--check"]
    if not files:
        print(__doc__.strip(), file=sys.stderr)
        return 2
    bad = []
    for path in files:
        with open(path, encoding="utf-8") as f:
            raw = f.read()
        out = format_text(raw)
        if out == raw:
            continue
        if check:
            bad.append(path)
        else:
            with open(path, "w", encoding="utf-8") as f:
                f.write(out)
            print(f"formatted: {path}")
    if bad:
        print("xcstrings が未整形です。`python3 ios/scripts/xcstrings-fmt.py <file>` で整えてください:")
        for p in bad:
            print(f"  {p}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
