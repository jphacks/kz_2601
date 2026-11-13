#!/usr/bin/env python3
"""String Catalog (.xcstrings) の書式指定がキー(日本語)と英語の訳で合っているか(#1789・#44)。

訳はあるが崩れている物を見つける(英語の抜けは別の検査)。引数ごとに「何番目の引数がどの種類か」を比べる。
- `%@` `%lld` `%d` `%.1f` など。位置指定(`%2$@`)は並べ替えてよいが、番号と種類が合わないと違反
- 複数形の置き換え(`%#@name@` + `substitutions`)は `argNum` と `formatSpecifier`、中の `%arg` をその引数として読む
- 幅・精度・旗(`%.1f` と `%f`)は比べない。`%d` と `%lld` は別の種類として扱う(64bit の Int を %d で読むと崩れる)

使い方: python3 scripts/xcstrings-format-check.py [files...]
  引数が無ければ ios/ の下の全 .xcstrings(.build・DerivedData を除く)。違反があれば列挙して exit 1。
"""
import json
import os
import re
import sys

SPEC = re.compile(
    r"%%"
    r"|%(?:(?P<sp>\d+)\$)?#@(?P<sub>\w+)@"
    r"|%(?P<arg>arg)(?![A-Za-z0-9_])"
    r"|%(?:(?P<pos>\d+)\$)?[-+ #0']*(?:\d+|\*)?(?:\.(?:\d+|\*))?(?P<len>hh|h|ll|l|q|L|z|t|j)?(?P<conv>[@dDiuUxXoOfFeEgGcCsSpaA])"
)

SKIP_DIRS = {".build", "DerivedData", "build", ".git"}


def args_of(text, substitutions=None, sub_arg=None, start=0):
    """文字列の書式指定 → [(引数の番号, 種類)]。位置指定の無い物は start の次から順に数える。"""
    out = []
    seq = start
    for m in SPEC.finditer(text):
        if m.group(0) == "%%":
            continue
        if m.group("sub"):
            name = m.group("sub")
            sub = (substitutions or {}).get(name, {})
            if m.group("sp"):
                num = int(m.group("sp"))
            elif "argNum" in sub:
                num = int(sub["argNum"])
            else:
                seq += 1
                num = seq
            out.append((num, normalize(sub.get("formatSpecifier", "@"))))
            continue
        if m.group("arg"):
            if sub_arg is not None:
                out.append(sub_arg)
            continue
        if m.group("pos"):
            num = int(m.group("pos"))
        else:
            seq += 1
            num = seq
        out.append((num, (m.group("len") or "") + m.group("conv")))
    return out


def normalize(spec):
    m = re.fullmatch(r"%?(?:\d+\$)?[-+ #0']*\d*(?:\.\d+)?(hh|h|ll|l|q|L|z|t|j)?([@dDiuUxXoOfFeEgGcCsSpaA])", spec)
    return (m.group(1) or "") + m.group(2) if m else spec


def units(node):
    """localization の下の stringUnit の value を全部(variations の中も)。"""
    if isinstance(node, dict):
        unit = node.get("stringUnit")
        if isinstance(unit, dict) and isinstance(unit.get("value"), str):
            yield unit["value"]
        for key, value in node.items():
            if key in ("stringUnit", "substitutions"):
                continue
            yield from units(value)


def en_args(en):
    """英語の訳の引数(variations の各形の和)。"""
    subs = en.get("substitutions") or {}
    found = []
    for value in units(en):
        found += args_of(value, subs)
    for name, sub in subs.items():
        arg = (int(sub.get("argNum", 0)), normalize(sub.get("formatSpecifier", "@")))
        for value in units(sub):
            found += args_of(value, subs, sub_arg=arg)
    return found


def table(found):
    """[(番号, 種類)] → {番号: {種類}}"""
    out = {}
    for num, kind in found:
        out.setdefault(num, set()).add(kind)
    return out


def describe(t):
    return " ".join(f"{n}:{'/'.join(sorted(k))}" for n, k in sorted(t.items())) or "(なし)"


def check_catalog(path):
    with open(path, encoding="utf-8") as f:
        catalog = json.load(f)
    problems = []
    for key, entry in sorted(catalog.get("strings", {}).items()):
        if not isinstance(entry, dict) or entry.get("shouldTranslate") is False:
            continue
        en = (entry.get("localizations") or {}).get("en")
        if not isinstance(en, dict) or not list(units(en)):
            continue
        want = table(args_of(key))
        got = table(en_args(en))
        if want == got:
            continue
        if set(want) != set(got):
            reason = "数が違う"
        else:
            reason = "種類が違う"
        problems.append(f"{path}: {reason}: {json.dumps(key, ensure_ascii=False)} キー={describe(want)} en={describe(got)}")
    return problems


def default_files(root):
    out = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = sorted(d for d in dirnames if d not in SKIP_DIRS)
        out += [os.path.join(dirpath, n) for n in sorted(filenames) if n.endswith(".xcstrings")]
    return out


def main(argv):
    files = argv or default_files(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "ios"))
    problems = []
    for path in files:
        problems += check_catalog(path)
    for p in problems:
        print(p)
    if problems:
        print(f"書式指定の合わないキー: {len(problems)} 件", file=sys.stderr)
        return 1
    print(f"ok: {len(files)} ファイルの書式指定が合っている")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
