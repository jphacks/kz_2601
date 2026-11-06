#!/usr/bin/env python3
"""アプリに同梱するライセンスの一覧(ThirdPartyLicenses.json)を作る・検査する(#306)。

材料:
  - リポジトリの LICENSE(このアプリ自身の MIT の表示。一覧の先頭に入れる)
  - ios/Packages/MetamonServices/Package.resolved(SPM の依存。推移的な依存を含む)
  - 各パッケージのチェックアウト(Xcode の SourcePackages/checkouts など手元にあるもの)の
    LICENSE* / NOTICE* と、EXTRA_NOTICES に書いた追加の表示

使い方:
  python3 ios/scripts/third-party-licenses.py            # 生成して書き込む
  python3 ios/scripts/third-party-licenses.py --check    # 書き換えずに検査(古ければ exit 1)
  オプション: --source-packages <dir>(SourcePackages か checkouts。複数可。環境変数 METAMON_SOURCE_PACKAGES でも可)。
  無ければ ~/Library/Developer/Xcode/DerivedData/Metamon-*・ios/SourcePackages などを探す

--check は常に「Package.resolved の依存(identity・版・revision)と一覧が一致するか」と「LICENSE と先頭の表示が
一致するか」を見る。チェックアウトが見つかった時だけ、本文まで作り直して比べる(CI の ubuntu には無いので依存の照合だけ)。

ライセンスの種類が判定できない・本文が見つからない依存があれば、推測で埋めずに exit 1 で止まる。
標準ライブラリだけで動く(Python 3.9+)。
"""
import argparse
import glob
import json
import os
import re
import subprocess
import sys

IOS_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
REPO_DIR = os.path.dirname(IOS_DIR)
RESOLVED = os.path.join(IOS_DIR, "Packages", "MetamonServices", "Package.resolved")
OUTPUT = os.path.join(IOS_DIR, "Metamon", "Resources", "ThirdPartyLicenses.json")
APP_LICENSE = os.path.join(REPO_DIR, "LICENSE")

APP_ENTRY = {
    "id": "tailor-it",
    "name": "Tailor It",
    "url": "https://github.com/jphacks/kz_2601",
}

# パッケージの直下以外にある、アプリにリンクされる部分の表示(identity → チェックアウト内の相対パス)。
# Crashlytics/LICENSE は libunwind 由来の dwarf.h の著作権表示(HP)を含む。
EXTRA_NOTICES = {
    "firebase-ios-sdk": ["Crashlytics/LICENSE"],
}

LICENSE_FILE_RE = re.compile(r"^(LICEN[CS]E|COPYING)(\.(txt|md))?$", re.IGNORECASE)
NOTICE_FILE_RE = re.compile(r"^NOTICES?(\.(txt|md))?$", re.IGNORECASE)


def read_text(path):
    with open(path, encoding="utf-8") as f:
        lines = f.read().replace("\r\n", "\n").split("\n")
    return "\n".join(line.rstrip() for line in lines).strip("\n") + "\n"


def detect_license(text):
    """本文から SPDX の識別子を判定する。判定できなければ None(推測しない)。"""
    t = " ".join(text.split())
    if "Apache License" in t and "Version 2.0" in t:
        if "Runtime Library Exception" in t:
            return "Apache-2.0 WITH Swift-exception"
        return "Apache-2.0"
    if "Permission is hereby granted, free of charge" in t and "THE SOFTWARE IS PROVIDED \"AS IS\"" in t:
        return "MIT"
    if "Redistribution and use in source and binary forms" in t:
        if "Neither the name" in t:
            return "BSD-3-Clause"
        return "BSD-2-Clause"
    if "provided 'as-is'" in t and "altered source versions must be plainly marked" in t.lower():
        return "Zlib"
    return None


COPYRIGHT_RE = re.compile(r"^(Copyright\s*(\([cC]\)|©|\d)|©)")


def copyright_lines(text):
    """「Copyright (c) 年 名前」の形の行だけ。本文中の "copyright notice" などは拾わない。"""
    out = []
    for line in text.splitlines():
        s = line.strip()
        if COPYRIGHT_RE.match(s) and "[yyyy]" not in s:
            out.append(s)
    return out


def load_pins():
    with open(RESOLVED, encoding="utf-8") as f:
        data = json.load(f)
    pins = []
    for p in data["pins"]:
        state = p.get("state", {})
        loc = p["location"]
        name = loc.rstrip("/").rsplit("/", 1)[-1]
        if name.endswith(".git"):
            name = name[:-4]
        pins.append({
            "id": p["identity"],
            "name": name,
            "version": state.get("version"),
            "revision": state.get("revision"),
            "url": re.sub(r"\.git$", "", loc),
        })
    return sorted(pins, key=lambda p: p["name"].lower())


def candidate_checkout_dirs(extra):
    dirs = []
    env = os.environ.get("METAMON_SOURCE_PACKAGES")
    for d in list(extra) + ([env] if env else []):
        d = os.path.expanduser(d)
        dirs.append(os.path.join(d, "checkouts") if os.path.isdir(os.path.join(d, "checkouts")) else d)
    found = glob.glob(os.path.expanduser("~/Library/Developer/Xcode/DerivedData/Metamon-*/SourcePackages/checkouts"))
    found += [
        os.path.join(IOS_DIR, "SourcePackages", "checkouts"),  # CI(-clonedSourcePackagesDirPath)
        os.path.join(IOS_DIR, "DerivedData", "SourcePackages", "checkouts"),
        os.path.join(IOS_DIR, "Packages", "MetamonServices", ".build", "checkouts"),
    ]
    found = [d for d in found if os.path.isdir(d)]
    found.sort(key=os.path.getmtime, reverse=True)
    return dirs + found


def git_head(path):
    try:
        r = subprocess.run(["git", "-C", path, "rev-parse", "HEAD"], capture_output=True, text=True, check=True)
        return r.stdout.strip()
    except (OSError, subprocess.CalledProcessError):
        return None


def find_checkout(pin, dirs):
    """revision が一致するチェックアウトを返す。(path, 理由)"""
    reasons = []
    for d in dirs:
        try:
            names = os.listdir(d)
        except OSError:
            continue
        match = [n for n in names if n.lower() in (pin["name"].lower(), pin["id"].lower())]
        for n in match:
            path = os.path.join(d, n)
            head = git_head(path)
            if head == pin["revision"]:
                return path, None
            reasons.append(f"{path} は {head or '不明'}")
    return None, "; ".join(reasons) or "チェックアウトが見つからない"


def build_entry(pin, path):
    files = sorted(os.listdir(path))
    lic = [f for f in files if LICENSE_FILE_RE.match(f) and os.path.isfile(os.path.join(path, f))]
    if not lic:
        raise LookupError(f"{pin['id']}: {path} に LICENSE が無い")
    text = read_text(os.path.join(path, lic[0]))
    spdx = detect_license(text)
    if spdx is None:
        raise LookupError(f"{pin['id']}: {lic[0]} のライセンスの種類を判定できない")
    notices = []
    rels = [f for f in files if NOTICE_FILE_RE.match(f)] + EXTRA_NOTICES.get(pin["id"], [])
    for rel in rels:
        p = os.path.join(path, rel)
        if not os.path.isfile(p):
            raise LookupError(f"{pin['id']}: 追加の表示 {rel} が無い")
        notices.append({"title": rel, "text": read_text(p)})
    copyrights = copyright_lines(text)
    for n in notices:
        copyrights += [c for c in copyright_lines(n["text"]) if c not in copyrights]
    return {
        "id": pin["id"],
        "name": pin["name"],
        "version": pin["version"],
        "revision": pin["revision"],
        "url": pin["url"],
        "license": spdx,
        "copyright": copyrights,
        "text": text,
        "notices": notices,
    }


def app_entry():
    text = read_text(APP_LICENSE)
    spdx = detect_license(text)
    if spdx is None:
        raise LookupError("LICENSE の種類を判定できない")
    return dict(APP_ENTRY, kind="app", license=spdx, copyright=copyright_lines(text), text=text, notices=[])


def generate(dirs):
    entries = [app_entry()]
    errors = []
    for pin in load_pins():
        path, why = find_checkout(pin, dirs)
        if path is None:
            errors.append(f"{pin['id']} {pin['version']}({pin['revision'][:7]}): {why}")
            continue
        try:
            entries.append(dict(kind="dependency", **build_entry(pin, path)))
        except LookupError as e:
            errors.append(str(e))
    return {"source": "ios/Packages/MetamonServices/Package.resolved", "entries": entries}, errors


def dump(doc):
    return json.dumps(doc, ensure_ascii=False, indent=2) + "\n"


def check_pins(current):
    """依存の集合と LICENSE だけを照合する(チェックアウト不要)。"""
    problems = []
    want = {p["id"]: p for p in load_pins()}
    have = {e["id"]: e for e in current.get("entries", []) if e.get("kind") == "dependency"}
    for pid in sorted(set(want) - set(have)):
        problems.append(f"一覧に無い依存: {pid} {want[pid]['version']}")
    for pid in sorted(set(have) - set(want)):
        problems.append(f"Package.resolved に無い依存が一覧にある: {pid}")
    for pid in sorted(set(want) & set(have)):
        w, h = want[pid], have[pid]
        if (w["version"], w["revision"]) != (h.get("version"), h.get("revision")):
            problems.append(f"版が違う: {pid} 一覧 {h.get('version')} / Package.resolved {w['version']}")
    apps = [e for e in current.get("entries", []) if e.get("kind") == "app"]
    if not apps or current["entries"][0].get("kind") != "app":
        problems.append("先頭にこのアプリの表示が無い")
    elif apps[0].get("text") != read_text(APP_LICENSE):
        problems.append("LICENSE と先頭の表示が違う")
    return problems


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--check", action="store_true", help="書き換えずに検査する")
    ap.add_argument("--source-packages", action="append", default=[], help="SourcePackages または checkouts のディレクトリ")
    args = ap.parse_args()
    rel_out = os.path.relpath(OUTPUT, REPO_DIR)
    regen = f"python3 ios/scripts/{os.path.basename(__file__)}"

    if args.check:
        try:
            with open(OUTPUT, encoding="utf-8") as f:
                raw = f.read()
            current = json.loads(raw)
        except (OSError, ValueError) as e:
            print(f"{rel_out} を読めません: {e}", file=sys.stderr)
            return 1
        problems = check_pins(current)
        if problems:
            print(f"{rel_out} が古い。`{regen}` で作り直してください:", file=sys.stderr)
            for p in problems:
                print(f"  - {p}", file=sys.stderr)
            return 1
        doc, errors = generate(candidate_checkout_dirs(args.source_packages))
        if errors:
            print(f"依存は一致(チェックアウトが揃わないので本文は比べていない: {len(errors)} 件)")
            return 0
        if dump(doc) != raw:
            print(f"{rel_out} の本文がチェックアウトと違う。`{regen}` で作り直してください", file=sys.stderr)
            return 1
        print(f"{rel_out} は最新({len(doc['entries'])} 件)")
        return 0

    doc, errors = generate(candidate_checkout_dirs(args.source_packages))
    if errors:
        print("一覧を作れません(推測で埋めないので止まります):", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        print("Xcode で一度ビルドして SourcePackages を作るか、--source-packages で場所を指定してください", file=sys.stderr)
        return 1
    with open(OUTPUT, "w", encoding="utf-8") as f:
        f.write(dump(doc))
    print(f"{rel_out} に {len(doc['entries'])} 件を書きました")
    return 0


if __name__ == "__main__":
    sys.exit(main())
