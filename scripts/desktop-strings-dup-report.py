#!/usr/bin/env python3
"""デスクトップの strings_desktop.xml で、Android の strings に同じ日本語の値がある desktop_ のキーを一覧に出す(#3086)。

README の決まり(Android に無い文言だけ desktop_ で足す)を守れているかを見る。読むだけ。
落とさない(終了コードは 0。引数や XML の誤りだけ 2)。標準ライブラリだけ。

使い方: scripts/desktop-strings-dup-report.py [--desktop FILE] [--android DIR]
  既定: desktop/app/src/main/resources/i18n/desktop/values/strings_desktop.xml と android/
  Android 側は DIR の下の values/strings*.xml(values-en などの言語違いは見ない)を全部読む。
出力: 「desktop_<キー>\t<Android のキー>(,...)\t<値>」を 1 行ずつ + 件数。
"""
import argparse
import os
import sys
import xml.etree.ElementTree as ET

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DESKTOP = "desktop/app/src/main/resources/i18n/desktop/values/strings_desktop.xml"


def load(path):
    try:
        tree = ET.parse(path)
    except (ET.ParseError, OSError) as e:
        print("読めない XML: %s (%s)" % (path, e), file=sys.stderr)
        sys.exit(2)
    out = []
    for el in tree.getroot().iter("string"):
        name = el.get("name")
        if not name:
            continue
        text = "".join(el.itertext()).replace("\\'", "'").replace('\\"', '"').strip()
        out.append((name, text))
    return out


def android_files(base):
    found = []
    for dp, dn, fn in os.walk(base):
        dn[:] = sorted(d for d in dn if d not in ("build", ".gradle", ".git"))
        if os.path.basename(dp) != "values":
            continue
        found += [os.path.join(dp, f) for f in sorted(fn) if f.startswith("strings") and f.endswith(".xml")]
    return found


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--desktop", default=os.path.join(ROOT, DESKTOP))
    ap.add_argument("--android", default=os.path.join(ROOT, "android"))
    a = ap.parse_args()
    if not os.path.isfile(a.desktop):
        print("ない XML: %s" % a.desktop, file=sys.stderr)
        return 2
    if not os.path.isdir(a.android):
        print("ない場所: %s" % a.android, file=sys.stderr)
        return 2

    by_value = {}
    for f in android_files(a.android):
        for name, text in load(f):
            if text and not name.startswith("desktop_"):
                by_value.setdefault(text, []).append(name)

    hits = 0
    for name, text in load(a.desktop):
        if name.startswith("desktop_") and text in by_value:
            print("%s\t%s\t%s" % (name, ",".join(sorted(set(by_value[text]))), text))
            hits += 1
    print("Android と同じ文言の desktop_ キー: %d 件" % hits)
    return 0


if __name__ == "__main__":
    sys.exit(main())
