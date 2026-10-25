import Foundation

// Logic v3: agg latest/earliest(#133)・毎日リセットのチェック(#132)。lookup(#134)は ExprEvaluator.fieldValue。

/// 「今日」の基準時刻。期間 filter・`today`/`now`・毎日リセットのチェックが使う。
/// DEBUG ビルドだけ環境変数 `METAMON_DEBUG_TODAY=YYYY-MM-DD`(simctl は `SIMCTL_CHILD_` 接頭辞)で日付を差し替えられる
/// (時刻は実時刻のまま。日付境界の確認用)。
public enum DSLClock {
    public static func now() -> Date {
        #if DEBUG
        if let override = debugNow(Date()) { return override }
        #endif
        return Date()
    }

    /// `METAMON_DEBUG_TODAY` の日付 + 実時刻の時:分:秒。未設定・形式外は nil。
    static func debugNow(_ real: Date, env: [String: String] = ProcessInfo.processInfo.environment, calendar: Calendar = .current) -> Date? {
        guard let s = env["METAMON_DEBUG_TODAY"], let day = FieldCoding.dateFormatter.date(from: s) else { return nil }
        let t = calendar.dateComponents([.hour, .minute, .second], from: real)
        return calendar.date(bySettingHour: t.hour ?? 0, minute: t.minute ?? 0, second: t.second ?? 0, of: day)
    }
}

/// `list.item.resets: "daily"`(#132): toggle は date field。値が今日(端末ローカル)なら済み。
public enum DailyCheck {
    /// 保存値が `today` と同じ日か。"YYYY-MM-DD" も RFC3339 も読む。空・形式外は未チェック。
    public static func isDone(_ value: JSONValue, today: Date, calendar: Calendar = .current) -> Bool {
        guard let d = FieldCoding.date(from: value) else { return false }
        if let s = value.stringValue, !s.contains("T") {
            // "YYYY-MM-DD" は暦日として比べる(dateFormatter は端末 TZ の 0:00 で読む)
            return s == FieldCoding.dateFormatter.string(from: today)
        }
        return calendar.isDate(d, inSameDayAs: today)
    }

    /// チェックを付けた/外した時に保存する値(付けたら今日の "YYYY-MM-DD"、外したら null)。
    /// 履歴(#172)も残すなら `patch(_:field:on:today:)` を使う。
    public static func stored(_ on: Bool, today: Date) -> JSONValue {
        on ? .string(FieldCoding.dateFormatter.string(from: today)) : .null
    }

    // MARK: - 押した日の履歴(#172)

    /// 履歴の保存キー(toggle field id + "__dates")。値は "YYYY-MM-DD" の昇順配列。DSL には宣言しない(appconfig が date field の隣として受ける)。
    public static func historyKey(_ field: String) -> String { field + historySuffix }
    public static let historySuffix = "__dates"
    /// 履歴の上限(古い方から捨てる。1 年強)。appconfig の上限と揃える。
    public static let maxHistory = 400

    /// 押した日("YYYY-MM-DD")の昇順・重複なし。履歴の無い旧レコード(#132 の lastDoneDate だけ)は toggle の値 1 件として読む。
    public static func dates(_ record: Record, field: String) -> [String] {
        var set = Set<String>()
        if case .array(let xs) = record[historyKey(field)] { for x in xs { if let d = day(x) { set.insert(d) } } }
        if let d = day(record[field]) { set.insert(d) }
        return set.sorted()
    }

    /// チェックを付けた/外した時に保存する差分(toggle field と履歴)。付けたら今日を足し、外したら今日を消す。
    /// toggle field は後方互換で「最後にやった日」のまま: 付けたら今日、外したら残った最新日(無ければ null)。
    public static func patch(_ record: Record, field: String, on: Bool, today: Date) -> [String: JSONValue] {
        let t = FieldCoding.dateFormatter.string(from: today)
        var ds = dates(record, field: field).filter { $0 != t }
        if on { ds.append(t) }
        ds = Array(ds.sorted().suffix(maxHistory))
        let last: JSONValue = on ? .string(t) : (ds.last.map { .string($0) } ?? .null)
        return [field: last, historyKey(field): ds.isEmpty ? .null : .array(ds.map { .string($0) })]
    }

    /// 連続日数。今日やっていれば今日から、まだなら昨日から遡って続いた日数(今日未チェックでも昨日までの連続は切らない)。
    public static func streak(_ days: [String], today: Date, calendar: Calendar = .current) -> Int {
        let set = Set(days)
        var d = calendar.startOfDay(for: today)
        if !set.contains(key(d)) { d = calendar.date(byAdding: .day, value: -1, to: d) ?? d }
        var n = 0
        while set.contains(key(d)), n <= maxHistory {
            n += 1
            guard let prev = calendar.date(byAdding: .day, value: -1, to: d) else { break }
            d = prev
        }
        return n
    }

    /// 直近 n 日(今日を含む)にやった日数。n < 1 は 0。
    public static func count(_ days: [String], lastDays n: Int, today: Date, calendar: Calendar = .current) -> Int {
        guard n >= 1 else { return 0 }
        let start = calendar.startOfDay(for: today)
        let window = Set((0..<min(n, maxHistory)).compactMap { calendar.date(byAdding: .day, value: -$0, to: start).map(key) })
        return Set(days).intersection(window).count
    }

    private static func key(_ d: Date) -> String { FieldCoding.dateFormatter.string(from: d) }

    /// 保存値 → "YYYY-MM-DD"(RFC3339 は端末ローカルの日付)。空・形式外は nil。
    private static func day(_ v: JSONValue) -> String? {
        guard let s = v.stringValue, !s.isEmpty, let d = FieldCoding.date(from: v) else { return nil }
        return s.contains("T") ? key(d) : String(s.prefix(10))
    }
}

public extension Aggregator {
    /// 1 件の値を返す集計(latest / earliest)か。
    static func isPick(_ op: String?) -> Bool { op == "latest" || op == "earliest" }

    /// latest / earliest の対象レコード。`by`(date/datetime field)で並べ、省略時は entity の最初の日付 field、
    /// それも無ければ作成順。同じ日(by の値が同じ)は作成順で後の方を新しいとみなす。by が空のレコードは除く。0 件は nil。
    static func pick(_ op: String, by: String?, entity: Entity?, over records: [Record]) -> Record? {
        let byField = by ?? entity?.fields.first { $0.type.isDate }?.id
        typealias Key = (date: Date?, created: String, index: Int)
        let keyed: [(Key, Record)] = records.enumerated().compactMap { i, r in
            let d = byField.flatMap { FieldCoding.date(from: r[$0]) }
            if byField != nil, d == nil { return nil }
            return ((d, r.createdAt ?? "", i), r)
        }
        func less(_ a: Key, _ b: Key) -> Bool {
            if let x = a.date, let y = b.date, x != y { return x < y }
            if a.created != b.created { return a.created < b.created }
            return a.index < b.index
        }
        let sorted = keyed.sorted { less($0.0, $1.0) }
        return op == "earliest" ? sorted.first?.1 : sorted.last?.1
    }

    /// stat の aggregate latest / earliest の表示文字列(数値は format・unit、それ以外は field の表示形)。0 件・空は nil。
    static func pickText(_ c: Component, entity: Entity?, records: [Record]) -> String? {
        guard let op = c.aggregate, let fieldId = c.field,
              let r = pick(op, by: c.by, entity: entity, over: records) else { return nil }
        let v = r[fieldId]
        guard !v.isNull else { return nil }
        let f = entity?.field(fieldId)
        if let n = v.doubleValue, f?.type.isNumeric ?? true {
            let fmt = c.format ?? (f?.type == .currency ? "currency" : nil)
            return format(n, as: fmt, unit: f?.unit)
        }
        return FieldCoding.display(v, field: f)
    }
}

public extension AppDSL {
    /// `list.item.resets: "daily"` の toggle に使われる field id(entity id → field ids)。
    var dailyToggleFields: [String: Set<String>] {
        var out: [String: Set<String>] = [:]
        for s in screens {
            for c in s.allComponents where c.type == .list && c.item?.resets == "daily" {
                if let e = c.entity, let t = c.item?.toggle { out[e, default: []].insert(t) }
            }
        }
        return out
    }

    /// 毎日リセットの toggle field は新規作成で今日を入れない(入れると追加した瞬間に「済み」になる)。
    /// date field の既定値(未指定 = 今日)を null に固定する。フォーム・クイック記録の両方に効く。
    mutating func clearDailyToggleDefaults() {
        let daily = dailyToggleFields
        guard !daily.isEmpty else { return }
        for i in entities.indices {
            guard let ids = daily[entities[i].id] else { continue }
            for j in entities[i].fields.indices where ids.contains(entities[i].fields[j].id) && entities[i].fields[j].type.isDate {
                entities[i].fields[j].defaultValue = .null
            }
        }
    }
}
