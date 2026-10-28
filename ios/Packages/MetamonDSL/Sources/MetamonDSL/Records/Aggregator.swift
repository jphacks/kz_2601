import Foundation

/// stat / chart / list の filter・sort・集計。v1 は **iOS クライアント集計**(件数が増えたらサーバー側へ)。
/// 期間 filter(`today` / `thisWeek` / `thisMonth` / `thisYear`)は端末ローカル時刻基準。
public enum Aggregator {
    /// iOS が解釈する `Filter.op`(schema の enum と一致させる。テストで突き合わせる)。
    public static let filterOps: Set<String> = ["eq", "neq", "gt", "gte", "lt", "lte", "today", "thisWeek", "thisMonth", "thisYear", "contains"]

    /// `thisWeek` 用の暦: 週の始まりを**月曜に固定**する。
    /// 日本の暦・学校・職場の「今週」は月〜日で数えるのが一般的で、ロケール既定(ja_JP は日曜始まり)のままだと
    /// 日曜の記録が翌週側に入り、週末に振り返る用途(家事当番・週の目標)で直感とずれるため。
    public static func weekCalendar(_ base: Calendar = .current) -> Calendar {
        var cal = base
        cal.firstWeekday = 2
        return cal
    }

    public static func apply(_ filter: Filter?, entity: Entity?, to records: [Record]) -> [Record] {
        guard let filter else { return records }
        let field = entity?.field(filter.field)
        return records.filter { matches(filter, record: $0, field: field) }
    }

    public static func matches(_ f: Filter, record: Record, field: Field?, now: Date = DSLClock.now(), calendar: Calendar = .current) -> Bool {
        if let mid = RecordValue.markId(f.field) {
            // 自分が付けたか(#255)。value は me(`resolvingMe` 後は uid 文字列)。サーバーの mine をそのまま使う
            guard f.value != nil, f.value != .null else { return false }
            let mine = record.markState(mid).mine
            return f.op == "neq" ? !mine : mine
        }
        if f.field == RecordValue.author {
            // 投稿者の uid と比べる(value は `resolvingMe` で uid にしたもの・#251)。作者の無い行は作者 ""。uid に置き換わっていない me はどれにも一致しない
            guard let uid = f.value?.stringValue else { return false }
            let same = (record.author ?? "") == uid
            return f.op == "neq" ? !same : same
        }
        let v = record[f.field]
        switch f.op {
        case "thisMonth":
            guard let d = FieldCoding.date(from: v) else { return false }
            return calendar.isDate(d, equalTo: now, toGranularity: .month)
        case "today":
            guard let d = FieldCoding.date(from: v) else { return false }
            return calendar.isDate(d, inSameDayAs: now)
        case "thisWeek":
            // 月曜 0:00 ≤ d < 次の月曜 0:00(年をまたぐ週もそのまま 1 週として扱う)
            guard let d = FieldCoding.date(from: v), let week = weekCalendar(calendar).dateInterval(of: .weekOfYear, for: now) else { return false }
            return d >= week.start && d < week.end
        case "thisYear":
            guard let d = FieldCoding.date(from: v) else { return false }
            return calendar.isDate(d, equalTo: now, toGranularity: .year)
        case "contains":
            // type refs(#231): 選んだ id に value を含む。未選択(空・無し)= 全員なので常に一致
            let ids = refIds(v)
            return ids.isEmpty || ids.contains(f.value?.stringValue ?? "")
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

    /// type refs の保存値 → id の配列(文字列 1 つは 1 件として読む)。
    public static func refIds(_ v: JSONValue) -> [String] {
        switch v {
        case .array(let a): return a.compactMap { $0.stringValue }.filter { !$0.isEmpty }
        case .string(let s): return s.isEmpty ? [] : [s]
        default: return []
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
            let s = FieldCoding.plainNumber(value)
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
    /// `label` は日付以外の値 → 表示名(ref の groupBy はレコード id で束ねるので、参照先の見出しを渡す・#235)。
    public static func buckets(_ c: Component, entity: Entity?, records: [Record], label relabel: ((String) -> String)? = nil) -> [Bucket] {
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
                label = relabel?(key) ?? key
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
