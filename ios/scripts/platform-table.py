#!/usr/bin/env python3
"""部品・アクション・機能の「OS ごとに使えるか」の表を、Mac の申告のコードから作る(#2278)。

材料(手で写さない):
  - ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/Component.swift の ComponentType(全部品)
  - ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/Action.swift の ActionType(全アクション)
  - ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/ClientCapabilities.swift の
    macUnavailableComponents / macDisabledFeatures / reviewExcludedComponents / reviewDisabledFeatures / supportedFeatures
  - ios/MetamonMac/App/Runtime/MacAppRuntime.swift の perform の switch(Mac が一言だけ出すアクション)
  - desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop/ の DesktopSupport.kt(DesktopLeaves の containers・leaves)と
    PlayLeaves.kt・InputLeaves.kt・DisplayLeaves.kt・DataLeaves.kt・io/DesktopIo.kt の部品の集まり(デスクトップが描ける部品)
  - android/dsl/.../generated/Component.kt の ComponentType(Kotlin の名前 → type の文字列)
  - docs/desktop/support-matrix.md の 描ける / 代わり / 使えません / 未定(デスクトップの列の中身。部品とアクション)

使い方:
  python3 ios/scripts/platform-table.py           # docs/dsl/platforms.md の印の間を書き換える
  python3 ios/scripts/platform-table.py --check   # 書き換えずに比べる(古ければ exit 1)
  python3 ios/scripts/platform-table.py --print   # 表だけを標準出力に出す

Android の列は空けておく(申告ができたら足す)。デスクトップ(Windows・Linux)の列は、コードの「描ける部品」と
support-matrix.md の状態が食い違っていたら、表を作らずに止める(どちらかを直す)。
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
DESKTOP = ROOT / "desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop"
KT_COMPONENT = ROOT / "android/dsl/src/main/kotlin/jp/kz2601/metamon/dsl/generated/Component.kt"
DESKTOP_MATRIX = ROOT / "docs/desktop/support-matrix.md"
BEGIN = "<!-- platform-table:begin -->"
END = "<!-- platform-table:end -->"

OK = "使える"
ALT = "代わりの表示"
NO = "使えない"
UNDECIDED = "未定"
# support-matrix.md の状態 → 表の言葉
DESKTOP_STATE = {"描ける": OK, "代わり": ALT, "使えません": NO, "未定": UNDECIDED}


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


def desktop_rendered_types() -> set[str]:
    """デスクトップのコードが描く部品の type(文字列)。DesktopLeaves の containers・leaves と各ファイルの集まりを合わせる。"""
    kt = KT_COMPONENT.read_text()
    wire = {n: w for n, w in re.findall(r'^\s*([A-Z0-9_]+)\("([A-Za-z0-9]+)"', kt, re.M)}
    containers = {n for n, w in re.findall(r'^\s*([A-Z0-9_]+)\("([A-Za-z0-9]+)",\s*true\)', kt, re.M)}
    # (ファイル, 集まりの書き出し) どれも「書き出しから、行頭の ) まで」に ComponentType.X が並ぶ
    sets = [
        ("DesktopSupport.kt", r"val leaves: Set<ComponentType> = setOf\("),
        ("PlayLeaves.kt", r"val types: Set<ComponentType> = setOf\("),
        ("InputLeaves.kt", r"val types: Set<ComponentType> = setOf\("),
        ("DisplayLeaves.kt", r"val DISPLAY_LEAVES: Set<ComponentType> = setOf\("),
        ("DataLeaves.kt", r"val DATA_LEAVES: Set<ComponentType> = setOf\("),
        ("io/DesktopIo.kt", r"val types: Set<ComponentType> = setOf\("),
    ]
    names = set(containers)
    for fname, head in sets:
        text = (DESKTOP / fname).read_text()
        m = re.search(head + r"(.*?)\n\s*\)", text, re.S)
        if not m:
            sys.exit(f"{fname}: 描ける部品の集まり({head})が見つからない")
        names.update(re.findall(r"ComponentType\.([A-Z0-9_]+)", m.group(1)))
    unknown = names - set(wire)
    if unknown:
        sys.exit(f"{fname}: Component.kt に無い ComponentType: {sorted(unknown)}")
    return {wire[n] for n in names}


def desktop_matrix() -> tuple[dict[str, str], dict[str, str]]:
    """support-matrix.md の (部品の type → 状態, アクションの type → 状態)。"""
    text = DESKTOP_MATRIX.read_text()
    row = re.compile(r"^\| (\w+) \| (描ける|代わり|使えません|未定) \|", re.M)
    parts = re.split(r"^## ", text, flags=re.M)
    comps: dict[str, str] = {}
    acts: dict[str, str] = {}
    for part in parts:
        if part.startswith(("Container", "Leaf")):
            comps.update(row.findall(part))
        elif part.startswith("アクション"):
            acts.update(row.findall(part))
    if not comps or not acts:
        sys.exit(f"{DESKTOP_MATRIX}: 部品・アクションの表が読めない")
    return comps, acts


def desktop_columns(components: list[str], actions: list[str]) -> tuple[dict[str, str], dict[str, str]]:
    """デスクトップの列(部品・アクション)。コードと表が食い違っていたら止める。"""
    rendered = desktop_rendered_types()
    comp_state, act_state = desktop_matrix()
    errors: list[str] = []
    for c in components:
        st = comp_state.get(c)
        if st is None:
            errors.append(f"{c}: support-matrix.md に行が無い")
        elif (st in ("描ける", "代わり")) != (c in rendered):
            errors.append(f"{c}: コードは{'描く' if c in rendered else '描かない'}のに表は「{st}」")
    for c in comp_state:
        if c not in components:
            errors.append(f"{c}: support-matrix.md にあるが ComponentType に無い")
    for a in actions:
        if a not in act_state:
            errors.append(f"{a}: support-matrix.md のアクションの表に行が無い")
    if errors:
        sys.exit("デスクトップの列を作れない(コードと docs/desktop/support-matrix.md が食い違う):\n  " + "\n  ".join(errors))
    return ({c: DESKTOP_STATE[comp_state[c]] for c in components}, {a: DESKTOP_STATE[act_state[a]] for a in actions})


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

    desk_comp, desk_act = desktop_columns(components, actions)

    lines = ["### 部品", "", "| 部品 | iOS | Mac | Android | デスクトップ | 審査用の版 |", "|---|---|---|---|---|---|"]
    for c in components:
        mac = f"{NO}(枠)" if c in mac_unavailable else OK
        review = "外す" if c in review_excluded else ""
        lines.append(f"| `{c}` | {OK} | {mac} |  | {desk_comp[c]} | {review} |")

    notes = mac_action_notes(actions)
    lines += ["", "### アクション", "", "| アクション | iOS | Mac | Android | デスクトップ |", "|---|---|---|---|---|"]
    for a in actions:
        lines.append(f"| `{a}` | {OK} | {notes.get(a, OK)} |  | {desk_act[a]} |")

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
