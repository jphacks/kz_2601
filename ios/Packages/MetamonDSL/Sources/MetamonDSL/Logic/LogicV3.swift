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
    public static func stored(_ on: Bool, today: Date) -> JSONValue {
        on ? .string(FieldCoding.dateFormatter.string(from: today)) : .null
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
