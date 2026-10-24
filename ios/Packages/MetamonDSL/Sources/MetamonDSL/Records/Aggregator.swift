import Foundation

/// stat / chart / list の filter・sort・集計。v1 は **iOS クライアント集計**(件数が増えたらサーバー側へ)。
/// `thisMonth` / `today` は端末ローカル時刻基準。
public enum Aggregator {
    public static func apply(_ filter: Filter?, entity: Entity?, to records: [Record]) -> [Record] {
        guard let filter else { return records }
        let field = entity?.field(filter.field)
        return records.filter { matches(filter, record: $0, field: field) }
    }

    public static func matches(_ f: Filter, record: Record, field: Field?) -> Bool {
        let v = record[f.field]
        switch f.op {
        case "thisMonth":
            guard let d = FieldCoding.date(from: v) else { return false }
            return Calendar.current.isDate(d, equalTo: Date(), toGranularity: .month)
        case "today":
            guard let d = FieldCoding.date(from: v) else { return false }
            return Calendar.current.isDateInToday(d)
        case "eq": return v == f.value || v.displayString == (f.value ?? .null).displayString
        case "neq": return !(v == f.value || v.displayString == (f.value ?? .null).displayString)
        case "gt", "gte", "lt", "lte":
            guard let a = v.doubleValue, let b = f.value?.doubleValue else { return false }
            switch f.op {
            case "gt": return a > b
            case "gte": return a >= b
            case "lt": return a < b
            default: return a <= b
            }
        default: return true
        }
    }

    public static func sorted(_ records: [Record], by sort: Sort?, entity: Entity?) -> [Record] {
        guard let sort else {
            return records.sorted { ($0.createdAt ?? "") > ($1.createdAt ?? "") }
        }
        let asc = sort.direction != "desc"
        let field = entity?.field(sort.field)
        return records.sorted { a, b in
            let x = a[sort.field], y = b[sort.field]
            let less: Bool
            if let f = field, f.type.isNumeric, let p = x.doubleValue, let q = y.doubleValue {
                less = p < q
            } else {
                less = x.displayString < y.displayString
            }
            return asc ? less : !less
        }
    }

    /// count|sum|avg|min|max
    public static func aggregate(_ op: String?, field: String?, over records: [Record]) -> Double {
        let nums = field.map { f in records.compactMap { $0[f].doubleValue } } ?? []
        switch op ?? "count" {
        case "sum": return nums.reduce(0, +)
        case "avg": return nums.isEmpty ? 0 : nums.reduce(0, +) / Double(nums.count)
        case "min": return nums.min() ?? 0
        case "max": return nums.max() ?? 0
        default: return Double(records.count)
        }
    }

    public static func format(_ value: Double, as format: String?, unit: String? = nil) -> String {
        switch format {
        case "currency": return FieldCoding.currency(value)
        case "percent": return String(format: "%.0f%%", value)
        default:
            let s = value == value.rounded() ? String(Int(value)) : String(format: "%.1f", value)
            return unit.map { s + " " + $0 } ?? s
        }
    }

    public struct Bucket: Identifiable, Sendable {
        public var id: String { key }
        public var key: String
        public var label: String
        public var value: Double
    }

    /// chart: groupBy(enum or 日付 field)でグループ化して集計。日付は `bucket`(day|week|month)で丸める。
    public static func buckets(_ c: Component, entity: Entity?, records: [Record]) -> [Bucket] {
        guard let groupBy = c.groupBy else { return [] }
        let field = entity?.field(groupBy)
        var groups: [String: (label: String, rows: [Record])] = [:]
        var order: [String] = []
        for r in records {
            let v = r[groupBy]
            let key: String
            let label: String
            if let f = field, f.type.isDate, let d = FieldCoding.date(from: v) {
                (key, label) = dateBucket(d, c.bucket ?? "day")
            } else {
                key = v.displayString
                label = key
            }
            if groups[key] == nil { order.append(key) }
            groups[key, default: (label, [])].rows.append(r)
        }
        if let f = field, f.type == .enum, let options = f.options {
            order = options.filter { groups[$0] != nil } + order.filter { !options.contains($0) }
        } else if field?.type.isDate == true {
            order.sort()
        }
        return order.compactMap { key in
            groups[key].map { Bucket(key: key, label: $0.label, value: aggregate(c.aggregate, field: c.field, over: $0.rows)) }
        }
    }

    private static func dateBucket(_ d: Date, _ bucket: String) -> (String, String) {
        let cal = Calendar.current
        switch bucket {
        case "month":
            let comps = cal.dateComponents([.year, .month], from: d)
            return (String(format: "%04d-%02d", comps.year ?? 0, comps.month ?? 0), "\(comps.month ?? 0)月")
        case "week":
            let start = cal.dateInterval(of: .weekOfYear, for: d)?.start ?? d
            let key = FieldCoding.dateFormatter.string(from: start)
            let comps = cal.dateComponents([.month, .day], from: start)
            return (key, "\(comps.month ?? 0)/\(comps.day ?? 0)週")
        default:
            let key = FieldCoding.dateFormatter.string(from: d)
            let comps = cal.dateComponents([.month, .day], from: d)
            return (key, "\(comps.month ?? 0)/\(comps.day ?? 0)")
        }
    }
}
