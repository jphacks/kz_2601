import Foundation

/// `export` アクション(#421)で書き出す表。見出し行 + 文字列の行(UI と同じ書式に整形済み)。
public struct ExportTable: Equatable, Sendable {
    public var title: String
    public var headers: [String]
    public var rows: [[String]]
    /// PDF の見出し下に出す補足(絞り込み・件数など)
    public var notes: [String]
    /// 列ごとに数値か(PDF で右寄せ)。空なら全部左寄せ
    public var numericColumns: [Bool]

    public init(title: String, headers: [String], rows: [[String]], notes: [String] = [], numericColumns: [Bool] = []) {
        self.title = title
        self.headers = headers
        self.rows = rows
        self.notes = notes
        self.numericColumns = numericColumns
    }
}

/// 書き出しの列選び・行の整形。表示文字列への変換(日付・円・参照先の名前)は呼び出し側が渡す。
public enum RecordExport {
    /// 書き出す列。`fields` 指定ならその順(写真・動画・存在しない id は飛ばす)、省略で entity の全項目(計算項目を含む)。
    /// hidden(#262)は `includeHidden` の時だけ。
    public static func columns(entity: Entity, fields: [String]?, includeHidden: Bool) -> [Field] {
        let picked: [Field]
        if let fields, !fields.isEmpty {
            picked = fields.compactMap { id in entity.fields.first { $0.id == id } }
        } else {
            picked = entity.fields
        }
        return picked.filter { f in
            f.type != .photo && f.type != .video && (includeHidden || !f.hidden)
        }
    }

    /// 1 レコード = 1 行。hidden の値は自分が書いたレコード(`HiddenFields.canEdit`)の行だけ埋め、他は空欄。
    public static func table(title: String, entity: Entity, records: [Record], fields: [String]?, currentUid: String?,
                             notes: [String] = [], display: (JSONValue, Field) -> String) -> ExportTable {
        let showHidden = records.contains { HiddenFields.canEdit(record: $0, currentUid: currentUid) }
        let cols = columns(entity: entity, fields: fields, includeHidden: showHidden)
        let rows = records.map { r in
            let canSeeHidden = HiddenFields.canEdit(record: r, currentUid: currentUid)
            return cols.map { f -> String in
                if f.hidden && !canSeeHidden { return "" }
                let v = r[f.id]
                if v.isNull && !f.isComputed { return "" }
                return display(v, f)
            }
        }
        return ExportTable(title: title, headers: cols.map(\.name), rows: rows, notes: notes,
                           numericColumns: cols.map(\.type.isNumeric))
    }

    /// 日付の列があれば古い順(報告は時系列)。無ければ元の順。
    public static func chronological(_ records: [Record], entity: Entity) -> [Record] {
        guard let df = entity.fields.first(where: { $0.type.isDate && !$0.hidden }) else { return records }
        return records.enumerated().sorted { a, b in
            let da = FieldCoding.date(from: a.element[df.id]), db = FieldCoding.date(from: b.element[df.id])
            switch (da, db) {
            case let (x?, y?) where x != y: return x < y
            case (nil, _?): return false
            case (_?, nil): return true
            default: return a.offset < b.offset
            }
        }.map(\.element)
    }

    /// 共有シートに出るファイル名。パス区切りや制御文字を落とし、空なら「書き出し」。
    public static func fileName(_ title: String, ext: String) -> String {
        let bad = CharacterSet(charactersIn: "/\\:?%*|\"<>").union(.controlCharacters).union(.newlines)
        var base = title.unicodeScalars.map { bad.contains($0) ? "_" : String($0) }.joined()
            .trimmingCharacters(in: .whitespaces)
        while base.hasPrefix(".") { base.removeFirst() }
        if base.isEmpty { base = "書き出し" }
        return String(base.prefix(80)) + "." + ext
    }
}

/// RFC 4180 の CSV。Excel が日本語を化かさないよう UTF-8 BOM を付け、改行は CRLF。
public enum CSVWriter {
    public static let bom = Data([0xEF, 0xBB, 0xBF])

    /// 1 セル。`,` `"` 改行を含めば `"` で囲み `"` は 2 つに。`=` `+` `@` など式に見える先頭には `'` を付ける(CSV インジェクション対策)。
    public static func escape(_ cell: String) -> String {
        var s = cell
        if let first = s.unicodeScalars.first, isFormulaLead(first, next: s.unicodeScalars.dropFirst().first) {
            s = "'" + s
        }
        // Character 単位だと "\r\n" が 1 文字になり "\n" と一致しないので scalar で見る
        guard s.unicodeScalars.contains(where: { $0 == "," || $0 == "\"" || $0 == "\n" || $0 == "\r" }) else { return s }
        return "\"" + s.replacingOccurrences(of: "\"", with: "\"\"") + "\""
    }

    /// `-500` / `-¥500` のような負の数は式扱いしない(先頭 `-` は後ろが数字・¥ の時だけ許す)。
    private static func isFormulaLead(_ c: Unicode.Scalar, next: Unicode.Scalar?) -> Bool {
        switch c {
        case "=", "+", "@", "\t", "\r": return true
        case "-":
            guard let next else { return false }
            return !(CharacterSet.decimalDigits.contains(next) || next == "¥" || next == ".")
        default: return false
        }
    }

    public static func string(_ table: ExportTable) -> String {
        ([table.headers] + table.rows)
            .map { $0.map(escape).joined(separator: ",") }
            .joined(separator: "\r\n") + "\r\n"
    }

    public static func data(_ table: ExportTable) -> Data {
        bom + Data(string(table).utf8)
    }
}
