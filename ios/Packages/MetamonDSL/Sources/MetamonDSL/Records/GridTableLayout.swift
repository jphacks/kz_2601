import Foundation

/// `gridTable`(#234)の配置計算。列・行の見出しを作り、各レコードを (列, 行) のマスに置く。
/// 描画(GridTableLeaf)から切り離してテストする。
public struct GridTableLayout: Sendable {
    /// 見出し 1 つ。`value` は columnField / rowField に保存する値(空マスのタップで初期値になる)。
    public struct Header: Hashable, Sendable {
        public let value: JSONValue
        public let label: String
    }

    public struct Cell: Hashable, Sendable {
        public let column: Int
        public let row: Int
        public init(column: Int, row: Int) {
            self.column = column
            self.row = row
        }
    }

    /// integer の rowField で rowRange が無い時の既定(1〜6 限)と、行・列の上限。
    public static let defaultRows = 1...6
    public static let maxRows = 24
    public static let maxColumns = 14

    public let columnField: String
    public let rowField: String
    public let columns: [Header]
    public let rows: [Header]
    /// マス → そのマスのレコード(元の並び順)。見出しに無い値のレコードは載らない。
    public let cells: [Cell: [Record]]

    public init(component c: Component, entity: Entity?, records: [Record]) {
        columnField = c.columnField ?? ""
        rowField = c.rowField ?? ""
        let colDef = entity?.field(columnField)
        let rowDef = entity?.field(rowField)
        columns = Array(Self.headers(field: colDef, explicit: c.columnValues, range: nil, records: records, key: columnField).prefix(Self.maxColumns))
        rows = Array(Self.headers(field: rowDef, explicit: c.rowValues, range: c.rowRange, records: records, key: rowField).prefix(Self.maxRows))

        let colIndex = Dictionary(columns.enumerated().map { (Self.matchKey($0.element.value), $0.offset) }, uniquingKeysWith: { a, _ in a })
        let rowIndex = Dictionary(rows.enumerated().map { (Self.matchKey($0.element.value), $0.offset) }, uniquingKeysWith: { a, _ in a })
        var out: [Cell: [Record]] = [:]
        for r in records {
            guard let ck = Self.matchKey(r[columnField]), let rk = Self.matchKey(r[rowField]),
                  let ci = colIndex[ck], let ri = rowIndex[rk] else { continue }
            out[Cell(column: ci, row: ri), default: []].append(r)
        }
        cells = out
    }

    public func records(at cell: Cell) -> [Record] { cells[cell] ?? [] }

    /// 空のマスをタップした時に新規フォームへ渡す初期値(列・行の値)。
    public func prefill(at cell: Cell) -> [String: JSONValue] {
        var out: [String: JSONValue] = [:]
        if columns.indices.contains(cell.column), !columnField.isEmpty { out[columnField] = columns[cell.column].value }
        if rows.indices.contains(cell.row), !rowField.isEmpty { out[rowField] = rows[cell.row].value }
        return out
    }

    // MARK: - headers

    static func headers(field: Field?, explicit: [String]?, range: GridRange?, records: [Record], key: String) -> [Header] {
        if field?.type == .integer || field?.type == .number {
            let unit = field?.unit ?? ""
            return integerRange(range, records: records, key: key).map { Header(value: .number(Double($0)), label: "\($0)\(unit)") }
        }
        let values = explicit?.filter { !$0.isEmpty } ?? field?.options ?? []
        var seen = Set<String>()
        return values.filter { seen.insert($0).inserted }.map { Header(value: .string($0), label: $0) }
    }

    /// rowRange があればそれ、無ければ 1〜6(記録がそれより外ならそこまで広げる)。最大 24 行。
    static func integerRange(_ range: GridRange?, records: [Record], key: String) -> [Int] {
        if let range, range.to >= range.from {
            return Array(range.from...min(range.to, range.from + maxRows - 1))
        }
        let present = records.compactMap { intValue($0[key]) }
        let lo = min(defaultRows.lowerBound, present.min() ?? defaultRows.lowerBound)
        let hi = max(defaultRows.upperBound, present.max() ?? defaultRows.upperBound)
        return Array(lo...min(hi, lo + maxRows - 1))
    }

    static func intValue(_ v: JSONValue) -> Int? {
        switch v {
        case .number(let n) where n.isFinite: Int(n.rounded())
        case .string(let s): Int(s.trimmingCharacters(in: .whitespaces))
        default: nil
        }
    }

    /// 値の照合キー。数値は整数に丸め、"2" と 2 を同じマスに置く。
    static func matchKey(_ v: JSONValue) -> String? {
        switch v {
        case .number(let n) where n.isFinite: "n:\(Int(n.rounded()))"
        case .string(let s):
            if let i = Int(s.trimmingCharacters(in: .whitespaces)) { "n:\(i)" } else { s.isEmpty ? nil : "s:\(s)" }
        case .bool(let b): "s:\(b)"
        default: nil
        }
    }

    // MARK: - today

    /// 列見出しが曜日なら今日の列の index(「月」「月曜」「月曜日」「Mon」「Monday」)。曜日の列が 3 つ未満なら nil(曜日でない表を誤って強調しない)。
    public static func todayColumn(labels: [String], now: Date = Date(), calendar: Calendar = .current) -> Int? {
        let weekdays = labels.map(weekdayIndex)
        guard weekdays.compactMap({ $0 }).count >= min(3, labels.count), !labels.isEmpty else { return nil }
        let today = calendar.component(.weekday, from: now) // 1 = 日曜
        return weekdays.firstIndex(of: today)
    }

    private static let jaWeekdays = ["日", "月", "火", "水", "木", "金", "土"]
    private static let enWeekdays = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"]

    /// 曜日らしい見出し → Calendar の weekday(1 = 日曜)。
    static func weekdayIndex(_ label: String) -> Int? {
        let t = label.trimmingCharacters(in: .whitespaces)
        if let i = jaWeekdays.firstIndex(where: { t == $0 || t.hasPrefix($0 + "曜") || t == "(\($0))" }) { return i + 1 }
        let l = t.lowercased()
        if l.count >= 3, let i = enWeekdays.firstIndex(where: { l.hasPrefix($0) }) { return i + 1 }
        return nil
    }
}
