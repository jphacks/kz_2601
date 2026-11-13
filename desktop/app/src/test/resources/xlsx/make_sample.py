# XlsxIoTest の sample.xlsx を作る(Excel が保存する形を手で書く。式は計算済みの値つき)。
# 使い方: python3 make_sample.py(同じフォルダに sample.xlsx を書く)
import os
import zipfile

HERE = os.path.dirname(os.path.abspath(__file__))

CONTENT_TYPES = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/worksheets/sheet3.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>"""

ROOT_RELS = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>"""

# シートの並び(家計・メモ・空)と rels の番号はわざとずらす
WORKBOOK = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<workbookPr/>
<sheets>
<sheet name="家計" sheetId="1" r:id="rId3"/>
<sheet name="メモ" sheetId="2" r:id="rId1"/>
<sheet name="空" sheetId="3" r:id="rId2"/>
</sheets>
</workbook>"""

WORKBOOK_RELS = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="/xl/worksheets/sheet3.xml"/>
<Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>
<Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>"""

SHARED = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="8" uniqueCount="8">
<si><t>日付</t></si>
<si><t>品目</t></si>
<si><t>金額</t></si>
<si><t>メモ</t></si>
<si><t>済</t></si>
<si><t>ラーメン</t></si>
<si><r><rPr><b/></rPr><t>特盛</t></r><r><t xml:space="preserve"> 餃子</t></r><rPh sb="0" eb="2"><t>トクモリ</t></rPh></si>
<si><t>two</t></si>
</sst>"""

# 0: 標準 / 1: 組み込みの日付(14) / 2: 自前の日時 / 3: 桁区切りの数 / 4: 組み込みの時刻(20) / 5: 色つきの数(日付ではない)
STYLES = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<numFmts count="3">
<numFmt numFmtId="164" formatCode="yyyy/mm/dd\\ hh:mm"/>
<numFmt numFmtId="165" formatCode="#,##0.00"/>
<numFmt numFmtId="166" formatCode="[Red]0.0&quot;days&quot;"/>
</numFmts>
<cellStyleXfs count="1"><xf numFmtId="0"/></cellStyleXfs>
<cellXfs count="6">
<xf numFmtId="0" xfId="0"/>
<xf numFmtId="14" xfId="0" applyNumberFormat="1"/>
<xf numFmtId="164" xfId="0" applyNumberFormat="1"/>
<xf numFmtId="165" xfId="0" applyNumberFormat="1"/>
<xf numFmtId="20" xfId="0" applyNumberFormat="1"/>
<xf numFmtId="166" xfId="0" applyNumberFormat="1"/>
</cellXfs>
</styleSheet>"""

SHEET1 = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c><c r="D1" t="s"><v>3</v></c><c r="E1" t="s"><v>4</v></c></row>
<row r="2"><c r="A2" s="1"><v>45589</v></c><c r="B2" t="s"><v>5</v></c><c r="C2"><v>980</v></c><c r="E2" t="b"><v>1</v></c></row>
<row r="3"><c r="A3" s="2"><v>45590.604166666664</v></c><c r="B3" t="inlineStr"><is><t>コーヒー</t></is></c><c r="C3"><f>0.1+0.2</f><v>0.30000000000000004</v></c><c r="E3" t="b"><v>0</v></c></row>
<row r="4"/>
<row r="5"><c r="A5"/><c r="B5" t="s"><v>6</v></c><c r="C5"><f>SUM(C2:C3)</f><v>980.3</v></c><c r="E5" t="e"><f>1/0</f><v>#DIV/0!</v></c></row>
<row r="6"><c r="A6" s="3"><v>1234567.5</v></c><c r="B6" s="4"><v>0.5</v></c><c r="C6" t="str"><f>"ab"&amp;"c"</f><v>abc</v></c><c r="D6" s="5"><v>2.5</v></c><c r="E6"><v>1E-3</v></c></row>
<row r="7"><c r="C7"><f>C99</f></c></row>
</sheetData>
</worksheet>"""

SHEET2 = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheetData><row r="1"><c r="B1" t="s"><v>7</v></c></row></sheetData>
</worksheet>"""

SHEET3 = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/></worksheet>"""

FILES = [
    ("[Content_Types].xml", CONTENT_TYPES),
    ("_rels/.rels", ROOT_RELS),
    ("xl/workbook.xml", WORKBOOK),
    ("xl/_rels/workbook.xml.rels", WORKBOOK_RELS),
    ("xl/sharedStrings.xml", SHARED),
    ("xl/styles.xml", STYLES),
    ("xl/worksheets/sheet1.xml", SHEET1),
    ("xl/worksheets/sheet2.xml", SHEET2),
    ("xl/worksheets/sheet3.xml", SHEET3),
]

with zipfile.ZipFile(os.path.join(HERE, "sample.xlsx"), "w", zipfile.ZIP_DEFLATED) as z:
    for name, text in FILES:
        info = zipfile.ZipInfo(name, date_time=(2026, 1, 1, 0, 0, 0))
        info.compress_type = zipfile.ZIP_DEFLATED
        z.writestr(info, text.encode("utf-8"))
