import Foundation
import MetamonDSL

/// Mac の `Table`(列の並べ替え・幅の変更・行の選択・右クリック)で table / gridTable を描く試作(#2326・#944 の判断の材料)。
/// 既定 off。DEBUG ビルドで `defaults write <bundle id> metamon.debug.macTable -bool YES`
/// (または起動引数 `-metamon.debug.macTable YES`)の時だけ on。リリースビルドと iOS は常に off(今の描き方)。
public enum MacTableExperiment {
    public static let defaultsKey = "metamon.debug.macTable"

    public static func isEnabled(_ defaults: UserDefaults = .standard) -> Bool {
        #if os(macOS) && DEBUG
        defaults.bool(forKey: defaultsKey)
        #else
        false
        #endif
    }
}

/// 列の見出しで並べ替える時の値。数は数として、文字は Finder と同じ順(`localizedStandardCompare`)で比べ、値の無い行は最後
public enum TableSortKey: Equatable, Sendable {
    case number(Double)
    case text(String)
    case none

    static func of(_ value: JSONValue?, display: String) -> TableSortKey {
        if case .number(let n) = value { return .number(n) }
        return display.isEmpty ? .none : .text(display)
    }

    /// 昇順の比較。値の無い行は並びの向きに関わらず後ろ(呼び出し側で反転しない)
    static func compare(_ a: TableSortKey, _ b: TableSortKey) -> ComparisonResult {
        switch (a, b) {
        case let (.number(x), .number(y)): x < y ? .orderedAscending : x > y ? .orderedDescending : .orderedSame
        case let (.text(x), .text(y)): x.localizedStandardCompare(y)
        case (.number, .text): .orderedAscending
        case (.text, .number): .orderedDescending
        case (.none, .none): .orderedSame
        case (.none, _): .orderedDescending
        case (_, .none): .orderedAscending
        }
    }
}

/// `Table` の列(並べ替え・幅・並びの保存に使う id は位置 + field。同じ field の列が 2 つあっても別の列)
struct MacTableColumn: Identifiable, Hashable {
    var index: Int
    var title: String
    var numeric: Bool
    var id: String

    init(index: Int, title: String, field: String, numeric: Bool) {
        self.index = index
        self.title = title
        self.numeric = numeric
        id = "\(index)-\(field)"
    }
}

/// 列の見出しを押した時の並べ替え(`Table` の sortOrder)
struct MacTableSort: SortComparator, Hashable {
    var column: Int
    var order: SortOrder = .forward

    func compare(_ lhs: TableLeaf.CellRow, _ rhs: TableLeaf.CellRow) -> ComparisonResult {
        let a = column < lhs.keys.count ? lhs.keys[column] : .none
        let b = column < rhs.keys.count ? rhs.keys[column] : .none
        let result = TableSortKey.compare(a, b)
        if a == .none || b == .none || order == .forward { return result }
        return result == .orderedAscending ? .orderedDescending : result == .orderedDescending ? .orderedAscending : .orderedSame
    }

    /// 並べ替えた行(並べ替えていなければそのまま)
    static func sorted(_ rows: [TableLeaf.CellRow], by order: [MacTableSort]) -> [TableLeaf.CellRow] {
        order.isEmpty ? rows : rows.sorted(using: order)
    }
}

/// 選んだ行をタブ区切りでコピーする文字(見出し行 + 行。表計算に貼れる)
enum MacTableCopy {
    static func text(columns: [MacTableColumn], rows: [TableLeaf.CellRow]) -> String {
        let header = columns.map(\.title).joined(separator: "\t")
        let lines = rows.map { row in columns.map { $0.index < row.texts.count ? row.texts[$0.index] : "" }.joined(separator: "\t") }
        return ([header] + lines).joined(separator: "\n")
    }
}
