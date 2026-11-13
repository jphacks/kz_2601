#!/usr/bin/env python3
"""部品・アクション・機能の「OS ごとに使えるか」の表を、Mac の申告のコードから作る(#2278)。

材料(手で写さない):
  - ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/Component.swift の ComponentType(全部品)
  - ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/Action.swift の ActionType(全アクション)
  - ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/ClientCapabilities.swift の
    macUnavailableComponents / macDisabledFeatures / reviewExcludedComponents / reviewDisabledFeatures / supportedFeatures
  - ios/MetamonMac/App/Runtime/MacAppRuntime.swift の perform の switch(Mac が一言だけ出すアクション)

使い方:
  python3 ios/scripts/platform-table.py           # docs/dsl/platforms.md の印の間を書き換える
  python3 ios/scripts/platform-table.py --check   # 書き換えずに比べる(古ければ exit 1)
  python3 ios/scripts/platform-table.py --print   # 表だけを標準出力に出す

Android・デスクトップ(Windows・Linux)の列は空けておく(それぞれの申告ができたら足す)。
標準ライブラリだけで動く(Python 3.9+)。
"""
import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DSL = ROOT / "ios/Packages/MetamonDSL/Sources/MetamonDSL/Model"
MAC_RUNTIME = ROOT / "ios/MetamonMac/App/Runtime/MacAppRuntime.swift"
DOC = ROOT / "docs/dsl/platforms.md"
BEGIN = "<!-- platform-table:begin -->"
END = "<!-- platform-table:end -->"

OK = "使える"
ALT = "代わりの表示"
NO = "使えない"


def enum_cases(path: Path, name: str) -> list[str]:
    text = path.read_text()
    m = re.search(r"public enum %s\b[^{]*\{(.*?)\n\}" % name, text, re.S)
    if not m:
        sys.exit(f"{path}: enum {name} が見つからない")
    out: list[str] = []
    for line in m.group(1).splitlines():
        line = line.split("//")[0].strip()
        cm = re.match(r"case\s+([A-Za-z0-9_,\s]+)$", line)
        if cm:
            out += [c.strip() for c in cm.group(1).split(",") if c.strip()]
    return [c for c in out if c != "unknown"]


def swift_list(text: str, name: str) -> list[str]:
    m = re.search(r"static let %s\s*(?::[^=]+)?=\s*\[(.*?)\]" % name, text, re.S)
    if not m:
        sys.exit(f"ClientCapabilities.swift: {name} が見つからない")
    return [a or b for a, b in re.findall(r'\.(\w+)|"([^"]+)"', m.group(1))]


def supported_features(text: str) -> tuple[list[str], list[str]]:
    m = re.search(r"static var supportedFeatures: \[String\] \{\s*#if APP_STORE_REVIEW\s*\[(.*?)\]\s*#else\s*\[(.*?)\]\s*#endif", text, re.S)
    if not m:
        sys.exit("ClientCapabilities.swift: supportedFeatures の #if が見つからない")
    """(通常の版, 審査用の版)"""
    return re.findall(r'"([^"]+)"', m.group(2)), re.findall(r'"([^"]+)"', m.group(1))


def mac_action_notes(all_actions: list[str]) -> dict[str, str]:
    """perform の switch で toast だけを出すアクション(と default)を拾う。"""
    text = MAC_RUNTIME.read_text()
    m = re.search(r"private func perform\(_ action: Action, _ ctx: ActionContext\) async \{\s*switch action\.type \{(.*?)\n        \}\n    \}", text, re.S)
    if not m:
        sys.exit("MacAppRuntime.swift: perform の switch が見つからない")
    body = m.group(1)
    notes: dict[str, str] = {}
    handled: set[str] = set()
    for cm in re.finditer(r"\n        case ((?:\.\w+(?:,\s*)?)+):(.*?)(?=\n        case |\n        default:|\Z)", body, re.S):
        names = re.findall(r"\.(\w+)", cm.group(1))
        handled.update(names)
        block = cm.group(2)
        code = "\n".join(l.split("//")[0] for l in block.splitlines()).strip()
        tm = re.search(r'toast = L10n\.tr\("([^"]+)"\)', code)
        if tm and re.fullmatch(r"toast = L10n\.tr\(\"[^\"]+\"\)", code):
            for n in names:
                notes[n] = f"{ALT}(一言「{tm.group(1)}」)"
        elif code in ("", "break"):
            for n in names:
                notes[n] = f"{OK}(何もしない)"
    for a in all_actions:
        if a not in handled:
            notes[a] = f"{NO}(一言「このボタンは Mac ではまだ使えません」)"
    return notes


def build() -> str:
    caps = (DSL / "ClientCapabilities.swift").read_text()
    components = enum_cases(DSL / "Component.swift", "ComponentType")
    actions = enum_cases(DSL / "Action.swift", "ActionType")
    mac_unavailable = set(swift_list(caps, "macUnavailableComponents"))
    review_excluded = set(swift_list(caps, "reviewExcludedComponents"))
    mac_disabled = swift_list(caps, "macDisabledFeatures")
    review_disabled = swift_list(caps, "reviewDisabledFeatures")
    features, review_features = supported_features(caps)
    for name in mac_unavailable | review_excluded:
        if name not in components:
            sys.exit(f"ClientCapabilities.swift の {name} が ComponentType に無い")

    lines = ["### 部品", "", "| 部品 | iOS | Mac | Android | デスクトップ | 審査用の版 |", "|---|---|---|---|---|---|"]
    for c in components:
        mac = f"{NO}(枠)" if c in mac_unavailable else OK
        review = "外す" if c in review_excluded else ""
        lines.append(f"| `{c}` | {OK} | {mac} |  |  | {review} |")

    notes = mac_action_notes(actions)
    lines += ["", "### アクション", "", "| アクション | iOS | Mac | Android | デスクトップ |", "|---|---|---|---|---|"]
    for a in actions:
        lines.append(f"| `{a}` | {OK} | {notes.get(a, OK)} |  |  |")

    lines += ["", "### 機能(`supportedFeatures` / `disabledFeatures`)", "",
              "| 機能 | iOS | Mac | Android | デスクトップ | 審査用の版 |", "|---|---|---|---|---|---|"]
    rows: list[str] = []
    for f in features + [f for f in mac_disabled + review_disabled if f not in features]:
        if f in rows:
            continue
        rows.append(f)
        mac = f"{NO}(`disabledFeatures`)" if f in mac_disabled else OK
        if f in review_disabled:
            review = "外す"
        elif f in features and f not in review_features:
            review = "送らない"
        else:
            review = ""
        lines.append(f"| `{f}` | {OK} | {mac} |  |  | {review} |")
    return "\n".join(lines) + "\n"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--check", action="store_true")
    ap.add_argument("--print", action="store_true")
    args = ap.parse_args()
    table = build()
    if args.print:
        sys.stdout.write(table)
        return 0
    doc = DOC.read_text()
    m = re.search(re.escape(BEGIN) + r"\n(.*?)" + re.escape(END), doc, re.S)
    if not m:
        sys.exit(f"{DOC}: {BEGIN} と {END} が見つからない")
    if m.group(1) == table:
        print("platforms.md は最新")
        return 0
    if args.check:
        print("platforms.md が古い。python3 ios/scripts/platform-table.py で作り直す", file=sys.stderr)
        return 1
    DOC.write_text(doc[: m.start(1)] + table + doc[m.end(1):])
    print("platforms.md を書き換えた")
    return 0


if __name__ == "__main__":
    sys.exit(main())
