import Foundation

/// `table.groupBy`(#261): 同じ値のレコードを 1 行にまとめ、集計列(count|sum|avg|min|max)を出す。
/// 例: 店ごとの平均点・件数のランキング。chart.groupBy と同じくクライアント集計。
public enum TableGrouping {
    public static let aggregates: Set<String> = ["count", "sum", "avg", "min", "max"]

    public struct Row: Identifiable, Sendable {
        public var id: String { key }
        /// グループの値(文字列化・前後の空白は無視)
        public var key: String
        /// グループ先頭のレコード(グループ名の表示に使う)
        public var sample: Record
        /// columns と同じ並び。グループ名の列・値の無い avg/min/max は nil
        public var values: [Double?]
    }

    /// groupBy があり集計列が 1 つ以上ある時だけグループ表示(それ以外は従来のレコード表)。
    public static func isGrouped(_ c: Component) -> Bool {
        guard let g = c.groupBy, !g.isEmpty else { return false }
        return (c.columns ?? []).contains { aggregates.contains($0.aggregate ?? "") }
    }

    /// グループ化 → 集計 → sort → limit。値が空のレコードはどのグループにも入れない。
    public static func rows(_ c: Component, records: [Record]) -> [Row] {
        guard let groupBy = c.groupBy, !groupBy.isEmpty else { return [] }
        let columns = c.columns ?? []
        var groups: [String: [Record]] = [:]
        var order: [String] = []
        for r in records {
            let key = r[groupBy].displayString.trimmingCharacters(in: .whitespacesAndNewlines)
            if key.isEmpty { continue }
            if groups[key] == nil { order.append(key) }
            groups[key, default: []].append(r)
        }
        var rows = order.map { key -> Row in
            let members = groups[key] ?? []
            let values = columns.map { col -> Double? in
                guard let op = col.aggregate, aggregates.contains(op) else { return nil }
                return aggregate(op, field: col.field, over: members)
            }
            return Row(key: key, sample: members[0], values: values)
        }
        let (index, ascending) = sortKey(c)
        rows.sort { a, b in
            if let i = index {
                switch (a.values[i], b.values[i]) {
                case let (x?, y?) where x != y: return ascending ? x < y : x > y
                case (.some, .none): return true   // 値なしは最後
                case (.none, .some): return false
                default: break
                }
                return nameLess(a.key, b.key)
            }
            return ascending ? nameLess(a.key, b.key) : nameLess(b.key, a.key)
        }
        if let limit = c.limit, limit > 0, rows.count > limit { rows = Array(rows.prefix(limit)) }
        return rows
    }

    /// 並べる列: sort.field と同じ field の集計列 → 無ければ groupBy の field ならグループ名。
    /// sort 省略 = 最初の集計列の降順。戻り値の index が nil ならグループ名で並べる。
    static func sortKey(_ c: Component) -> (index: Int?, ascending: Bool) {
        let columns = c.columns ?? []
        let firstAgg = columns.firstIndex { aggregates.contains($0.aggregate ?? "") }
        guard let sort = c.sort else { return (firstAgg, false) }
        let ascending = sort.direction == "asc"
        if let i = columns.firstIndex(where: { $0.field == sort.field && aggregates.contains($0.aggregate ?? "") }) {
            return (i, ascending)
        }
        if sort.field == c.groupBy { return (nil, ascending) }
        return (firstAgg, ascending)
    }

    /// avg / min / max は数値が 1 つも無ければ nil(0 と区別して「—」)。
    static func aggregate(_ op: String, field: String, over records: [Record]) -> Double? {
        if op == "avg" || op == "min" || op == "max", !records.contains(where: { $0[field].doubleValue != nil }) { return nil }
        return Aggregator.aggregate(op, field: field, over: records)
    }

    private static func nameLess(_ a: String, _ b: String) -> Bool {
        if let x = Double(a), let y = Double(b) { return x < y }
        return a.localizedStandardCompare(b) == .orderedAscending
    }
}
