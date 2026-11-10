#!/usr/bin/env python3
"""Android に同梱するライセンスの一覧(third_party_licenses.json)を作る・検査する(#898・iOS の ios/scripts/third-party-licenses.py と同じ形)。

材料:
  - リポジトリの LICENSE(このアプリ自身の MIT の表示。一覧の先頭に入れる)
  - 配布版(prodRelease)の実行時の依存: `./gradlew :app:dependencies --configuration prodReleaseRuntimeClasspath` の出力
  - Gradle のキャッシュ(~/.gradle/caches/modules-2)にある各依存の POM の <licenses>(無ければ親 POM)
  - Google の AAR が同梱する third_party_licenses.json / .txt(Play services・Firebase の中で使っている部品の表示)
  - android/scripts/license-texts/ の本文(POM には本文が無いので、種類ごとの正本をここに置く)

1 件 = groupId とライセンスの組(androidx.compose.ui の 10 個の artifact は 1 件)。版が揃っていれば版も出す。

使い方(Gradle を回すので、手元ではロックを付ける):
  /usr/bin/lockf -k ~/Desktop/JPHACKS2026/.locks/gradle.lock python3 android/scripts/third-party-licenses.py
  python3 android/scripts/third-party-licenses.py --check            # 書き換えずに検査(古ければ exit 1)
  python3 android/scripts/third-party-licenses.py --deps deps.txt    # 依存の一覧を既に出してある時(Gradle を回さない)

ライセンスの種類が判定できない・本文の正本が無い依存があれば、推測で埋めずに exit 1 で止まる。
Apache-2.0 以外の OSS ライセンス(MIT・BSD など)は、著作権者の表示が要るので TEXT_FILES に正本を足す。
Google の規約(Android SDK License・ML Kit・Play Core・Play Integrity)は OSS ではないので、本文の代わりに規約の URL を出す。
標準ライブラリだけで動く(Python 3.9+)。
"""
import argparse
import glob
import json
import os
import re
import subprocess
import sys
import xml.etree.ElementTree as ET
import zipfile

ANDROID_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
REPO_DIR = os.path.dirname(ANDROID_DIR)
OUTPUT = os.path.join(ANDROID_DIR, "app", "src", "main", "res", "raw", "third_party_licenses.json")
APP_LICENSE = os.path.join(REPO_DIR, "LICENSE")
TEXTS_DIR = os.path.join(ANDROID_DIR, "scripts", "license-texts")
CONFIGURATION = "prodReleaseRuntimeClasspath"
GRADLE_CACHE = os.path.join(os.path.expanduser("~"), ".gradle", "caches", "modules-2", "files-2.1")

APP_ENTRY = {
    "id": "tailor-it",
    "name": "Tailor It",
    "url": "https://github.com/jphacks/kz_2601",
}

# POM の <license><name> → SPDX(OSS)か、規約の名前(OSS ではない)
SPDX_BY_NAME = {
    "the apache software license, version 2.0": "Apache-2.0",
    "the apache license, version 2.0": "Apache-2.0",
    "apache license, version 2.0": "Apache-2.0",
    "apache-2.0": "Apache-2.0",
    "apache 2.0": "Apache-2.0",
    "bsd-3-clause": "BSD-3-Clause",
    "mit license": "MIT",
    "mit": "MIT",
}
TERMS = {
    "android software development kit license",
    "ml kit terms of service",
    "play core software development kit terms of service",
    "play integrity api terms of service",
}

# 本文の正本。(SPDX, groupId) → ファイル。groupId が None ならその種類の既定
TEXT_FILES = {
    ("Apache-2.0", None): "Apache-2.0.txt",
    # datastore-preferences-external-protobuf は protobuf-java を中に取り込んだもの(本文は protobuf の LICENSE)
    ("BSD-3-Clause", "androidx.datastore"): "protobuf-BSD-3-Clause.txt",
    # camera-core は libyuv を中に取り込んでいる(POM の 2 つ目のライセンス)
    ("BSD-3-Clause", "androidx.camera"): "libyuv-BSD-3-Clause.txt",
    # checker-qual(guava から来る)。本文は jar の META-INF/LICENSE.txt
    ("MIT", "org.checkerframework"): "checker-qual-MIT.txt",
}

DEP_RE = re.compile(r"--- ([^\s:]+):([^\s:]+)(?::(\S+))?(?: -> (\S+))?")


def read_text(path):
    with open(path, encoding="utf-8") as f:
        lines = f.read().replace("\r\n", "\n").split("\n")
    return "\n".join(line.rstrip() for line in lines).strip("\n") + "\n"


def dependency_report(deps_file):
    if deps_file:
        with open(deps_file, encoding="utf-8") as f:
            return f.read()
    out = subprocess.run(
        ["./gradlew", "-q", ":app:dependencies", "--configuration", CONFIGURATION],
        cwd=ANDROID_DIR, check=True, capture_output=True, text=True,
    )
    return out.stdout


def modules(report):
    """{(group, artifact): version}。制約((c))とプロジェクト(project :dsl など)は除く。版は解決後(-> の右)。"""
    found = {}
    for line in report.splitlines():
        if line.rstrip().endswith("(c)") or "project :" in line:
            continue
        m = DEP_RE.search(line)
        if not m:
            continue
        group, artifact, version, resolved = m.groups()
        version = resolved or version
        if version:
            found[(group, artifact)] = version
    return found


def module_dir(group, artifact, version):
    return os.path.join(GRADLE_CACHE, group, artifact, version)


def parse_pom(group, artifact, version):
    paths = glob.glob(os.path.join(module_dir(group, artifact, version), "*", "*.pom"))
    if not paths:
        return None
    with open(paths[0], encoding="utf-8") as f:
        text = re.sub(r'\sxmlns="[^"]+"', "", f.read(), count=1)
    return ET.fromstring(text.encode("utf-8"))


def pom_licenses(group, artifact, version, depth=0):
    """POM の <licenses>。無ければ親を辿る。見つからなければ空。"""
    root = parse_pom(group, artifact, version)
    if root is None:
        raise SystemExit(f"POM がキャッシュにありません: {group}:{artifact}:{version}(先に assembleProdRelease などで解決してください)")
    found = [((l.findtext("name") or "").strip(), (l.findtext("url") or "").strip()) for l in root.findall("licenses/license")]
    if found or depth >= 5:
        return found
    parent = root.find("parent")
    if parent is None:
        return []
    return pom_licenses(parent.findtext("groupId"), parent.findtext("artifactId"), parent.findtext("version"), depth + 1)


def pom_url(group, artifact, version):
    root = parse_pom(group, artifact, version)
    url = (root.findtext("scm/url") or root.findtext("url") or "").strip() if root is not None else ""
    return url or None


def normalize(name):
    """表記の揺れ(大文字小文字・空白・先頭の "The ")だけ吸収する。種類は広げない。"""
    return re.sub(r"^the ", "", re.sub(r"\s+", " ", name).strip().lower())


def classify(name):
    key = normalize(name)
    spdx = {normalize(k): v for k, v in SPDX_BY_NAME.items()}
    if key in spdx:
        return spdx[key], True
    if key in {normalize(t) for t in TERMS}:
        return name, False
    return None, False


def archive_notices(group, artifact, version):
    """Google の AAR / JAR の third_party_licenses.json(名前 → txt のバイト位置)。"""
    notices = []
    for path in sorted(glob.glob(os.path.join(module_dir(group, artifact, version), "*", "*"))):
        if not path.endswith((".aar", ".jar")) or path.endswith("-sources.jar"):
            continue
        with zipfile.ZipFile(path) as z:
            names = set(z.namelist())
            if "third_party_licenses.json" not in names or "third_party_licenses.txt" not in names:
                continue
            index = json.loads(z.read("third_party_licenses.json"))
            body = z.read("third_party_licenses.txt")
        for title, at in index.items():
            raw = body[at["start"]:at["start"] + at["length"]].decode("utf-8", errors="replace")
            text = "\n".join(line.rstrip() for line in raw.replace("\r\n", "\n").split("\n")).strip("\n") + "\n"
            notices.append({"title": title, "text": text})
    return notices


def build(report):
    mods = modules(report)
    if not mods:
        raise SystemExit("依存が 1 つも読めませんでした(Gradle の出力を確かめてください)")
    groups = {}
    errors = []
    for (group, artifact), version in sorted(mods.items()):
        if artifact.endswith("-bom"):
            continue  # 版を揃えるだけ(中身は無い)
        licenses = pom_licenses(group, artifact, version)
        if not licenses:
            errors.append(f"{group}:{artifact}:{version} のライセンスが POM にありません")
            continue
        # 複数あれば全部出す(camera-core は Apache-2.0 と、取り込んだ libyuv の BSD-3-Clause)
        notices = archive_notices(group, artifact, version)
        for name, url in licenses:
            license_id, oss = classify(name)
            if license_id is None:
                errors.append(f"{group}:{artifact}:{version} のライセンス「{name}」を判定できません(SPDX_BY_NAME / TERMS に足す)")
                continue
            entry = groups.setdefault((group, license_id), {
                "artifacts": {}, "oss": oss, "terms_url": None if oss else url, "notices": [], "url": None,
            })
            entry["artifacts"][artifact] = version
            entry["url"] = entry["url"] or pom_url(group, artifact, version)
            entry["notices"] += notices
            notices = []  # 同じ AAR の表示は 1 回だけ

    multi = {}
    for group, license_id in groups:
        multi[group] = multi.get(group, 0) + 1

    entries = []
    seen_notices = set()
    for (group, license_id), g in sorted(groups.items()):
        if g["oss"]:
            text_file = TEXT_FILES.get((license_id, group)) or TEXT_FILES.get((license_id, None))
            if not text_file:
                errors.append(f"{group}({license_id})の本文の正本がありません(license-texts と TEXT_FILES に足す)")
                continue
            text = read_text(os.path.join(TEXTS_DIR, text_file))
        else:
            text = f"{license_id}\n{g['terms_url']}\n"
        versions = sorted(set(g["artifacts"].values()))
        notices = []
        for n in g["notices"]:  # 同じ部品の表示は一覧全体で 1 回だけ(Kotlin・Guava などは多くの AAR が同梱する)
            if n["title"] in seen_notices:
                continue
            seen_notices.add(n["title"])
            notices.append(n)
        slug = re.sub(r"[^a-z0-9]+", "-", group.lower()).strip("-")
        if multi[group] > 1:
            slug += "-" + re.sub(r"[^a-z0-9]+", "-", license_id.lower()).strip("-")
        entries.append({
            "kind": "dependency",
            "id": slug,
            "name": group,
            "version": versions[0] if len(versions) == 1 else None,
            "url": g["url"],
            "license": license_id,
            "copyright": [],
            "artifacts": [f"{a}:{v}" for a, v in sorted(g["artifacts"].items())],
            "text": text,
            "notices": sorted(notices, key=lambda n: n["title"].lower()),
        })
    if errors:
        raise SystemExit("\n".join(errors))

    app_text = read_text(APP_LICENSE)
    app = dict(APP_ENTRY, kind="app", version=None, license="MIT",
               copyright=[l for l in app_text.split("\n") if l.startswith("Copyright")],
               artifacts=[], text=app_text, notices=[])
    return {"source": f"android :app {CONFIGURATION}", "entries": [app] + entries}


def dump(data):
    return json.dumps(data, ensure_ascii=False, indent=1) + "\n"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true", help="書き換えずに検査する(古ければ exit 1)")
    parser.add_argument("--deps", help=f"`:app:dependencies --configuration {CONFIGURATION}` の出力(無ければ Gradle を回す)")
    args = parser.parse_args()
    text = dump(build(dependency_report(args.deps)))
    if args.check:
        current = open(OUTPUT, encoding="utf-8").read() if os.path.exists(OUTPUT) else ""
        if current != text:
            print(f"{os.path.relpath(OUTPUT, REPO_DIR)} が古いです。python3 android/scripts/third-party-licenses.py で作り直してください", file=sys.stderr)
            sys.exit(1)
        print("ok")
        return
    os.makedirs(os.path.dirname(OUTPUT), exist_ok=True)
    with open(OUTPUT, "w", encoding="utf-8") as f:
        f.write(text)
    print(f"wrote {os.path.relpath(OUTPUT, REPO_DIR)}")


if __name__ == "__main__":
    main()
