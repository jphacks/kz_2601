#!/usr/bin/env python3
"""docs/dsl/icon-map.json から Android のアイコン表を作る(#842)。

material-icons-extended(35MB の AAR)は足さず、表で使う絵だけをソースから取り込む。
material-icons-core に有る絵はそのまま core を参照する。

使い方(android/ で):
  python3 scripts/gen-symbol-icons.py <extended の sources を展開した dir> <core の sources を展開した dir>
  sources jar は dl.google.com/android/maven2/androidx/compose/material/
  material-icons-{extended,core}-android/<版>/...-sources.jar(版は libs.versions.toml の materialIconsCore)。
"""
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
MAP = os.path.join(ROOT, "..", "docs", "dsl", "icon-map.json")
OUT = os.path.join(ROOT, "renderer", "src", "main", "kotlin", "jp", "kz2601", "metamon", "renderer", "theme", "icons")
PKG = "jp.kz2601.metamon.renderer.theme.icons"
ICONS = "commonMain/androidx/compose/material/icons"


def pascal(name: str) -> str:
    return "".join(w[:1].upper() + w[1:] for w in name.split("_"))


def locate(base: str, p: str):
    for sub, ref in (("automirrored/filled", "Icons.AutoMirrored.Filled"), ("filled", "Icons.Filled")):
        path = os.path.join(base, ICONS, sub, p + ".kt")
        if os.path.exists(path):
            return sub, ref, path
    return None


def main(ext_dir: str, core_dir: str) -> None:
    data = json.load(open(MAP, encoding="utf-8"))
    symbols: dict = data["symbols"]
    materials = sorted(set(symbols.values()))

    core_imports, vendored, refs, missing = [], [], {}, []
    for m in materials:
        p = pascal(m)
        hit = locate(core_dir, p)
        if hit:
            sub, ref, _ = hit
            core_imports.append(f"androidx.compose.material.icons.{sub.replace('/', '.')}.{p}")
            refs[m] = f"{ref}.{p}"
            continue
        hit = locate(ext_dir, p)
        if not hit:
            missing.append(m)
            continue
        sub, ref, path = hit
        vendored.append(path)
        refs[m] = f"{ref}.{p}"
    if missing:
        sys.exit(f"Material に無い名前: {missing}")

    os.makedirs(OUT, exist_ok=True)

    # 取り込んだ絵(Apache-2.0)を 1 ファイルにまとめる。
    imports, bodies = set(), []
    for path in vendored:
        src = open(path, encoding="utf-8").read()
        body = re.sub(r"(?s)^.*?\*/\s*", "", src, count=1)
        lines = body.splitlines()
        rest = []
        for line in lines:
            if line.startswith("package "):
                continue
            if line.startswith("import "):
                imports.add(line)
                continue
            rest.append(line)
        text = "\n".join(rest).strip("\n")
        text = text.replace("public val ", "internal val ")
        bodies.append(text)
    header = (
        "/*\n * Copyright The Android Open Source Project\n *\n"
        " * Licensed under the Apache License, Version 2.0 (the \"License\");\n"
        " * you may not use this file except in compliance with the License.\n"
        " * You may obtain a copy of the License at\n *\n"
        " *      http://www.apache.org/licenses/LICENSE-2.0\n *\n"
        " * Unless required by applicable law or agreed to in writing, software\n"
        " * distributed under the License is distributed on an \"AS IS\" BASIS,\n"
        " * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.\n"
        " * See the License for the specific language governing permissions and\n"
        " * limitations under the License.\n */\n\n"
        "// 生成物(android/scripts/gen-symbol-icons.py)。手で直さない。\n"
        "// androidx.compose.material:material-icons-extended の Filled の絵から、icon-map.json で使う分だけを取り込んだもの。\n\n"
    )
    with open(os.path.join(OUT, "VendoredIcons.kt"), "w", encoding="utf-8") as f:
        f.write(header + f"package {PKG}\n\n" + "\n".join(sorted(imports)) + "\n\n" + "\n\n".join(bodies) + "\n")

    # SF Symbols → Material の名前、Material の名前 → 絵。
    lines = [
        "// 生成物(android/scripts/gen-symbol-icons.py が docs/dsl/icon-map.json から作る)。手で直さない。",
        f"package {PKG}",
        "",
        "import androidx.compose.material.icons.Icons",
        *[f"import {i}" for i in sorted(core_imports)],
        "import androidx.compose.ui.graphics.vector.ImageVector",
        "",
        "/** SF Symbols の名前 → Material の名前(docs/dsl/icon-map.json の symbols)。 */",
        "internal val symbolMaterialNames: Map<String, String> = mapOf(",
        *[f'    "{k}" to "{v}",' for k, v in sorted(symbols.items())],
        ")",
        "",
        "/** Material の名前 → 絵。表に出る名前だけ。 */",
        "internal fun materialVector(name: String): ImageVector? = when (name) {",
        *[f'    "{m}" -> {refs[m]}' for m in materials],
        "    else -> null",
        "}",
        "",
        "/** [materialVector] が引ける名前(テスト用)。 */",
        "internal val materialVectorNames: Set<String> = setOf(",
        *[f'    "{m}",' for m in materials],
        ")",
        "",
    ]
    with open(os.path.join(OUT, "SymbolIconTable.kt"), "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    print(f"symbols={len(symbols)} materials={len(materials)} core={len(core_imports)} vendored={len(vendored)}")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2])
