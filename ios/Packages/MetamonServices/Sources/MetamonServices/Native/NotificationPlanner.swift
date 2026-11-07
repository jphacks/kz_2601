import Foundation
import MetamonDSL

/// 予約する通知 1 件(#228)。アプリごとに永続化し、全アプリ分から近い順に iOS の上限内で予約し直す。
public struct PlannedNotification: Codable, Sendable, Equatable {
    public enum Trigger: Codable, Sendable, Equatable {
        case at(Date)
        case daily(hour: Int, minute: Int)
        case weekly(weekday: Int, hour: Int, minute: Int)
        /// 毎月その日(#266)。日が毎月同じ(28 日以下で、ずらしても月をまたがない)時だけ使う
        case monthly(day: Int, hour: Int, minute: Int)
        /// 毎年その月日(#266)。2/29 や、ずらすと 2 月末をまたぐ時は使わない
        case yearly(month: Int, day: Int, hour: Int, minute: Int)
    }

    public var identifier: String
    public var title: String
    public var body: String
    public var trigger: Trigger
    /// 止めるまで鳴るアラームで届ける(`NotificationSpec.alarm`)。nil = 普通の通知(保存済みの古い予約も nil で読める)。
    public var alarm: Bool?

    public var isAlarm: Bool { alarm == true }

    public init(identifier: String, title: String, body: String, trigger: Trigger, alarm: Bool? = nil) {
        self.identifier = identifier
        self.title = title
        self.body = body
        self.trigger = trigger
        self.alarm = alarm
    }

    /// `now` より後の次の発火時刻。過ぎた 1 回きりは nil。
    public func nextFire(after now: Date, calendar: Calendar = .current) -> Date? {
        switch trigger {
        case .at(let d):
            return d > now ? d : nil
        case .daily(let h, let m):
            return calendar.nextDate(after: now, matching: DateComponents(hour: h, minute: m, second: 0), matchingPolicy: .nextTime)
        case .weekly(let w, let h, let m):
            return calendar.nextDate(after: now, matching: DateComponents(hour: h, minute: m, second: 0, weekday: w), matchingPolicy: .nextTime)
        case .monthly(let d, let h, let m):
            return calendar.nextDate(after: now, matching: DateComponents(day: d, hour: h, minute: m, second: 0), matchingPolicy: .strict)
        case .yearly(let mo, let d, let h, let m):
            return calendar.nextDate(after: now, matching: DateComponents(month: mo, day: d, hour: h, minute: m, second: 0), matchingPolicy: .strict)
        }
    }
}

/// DSL の通知定義 + レコード → 予約する通知の一覧(純粋関数・ユニットテスト対象)。
public enum NotificationPlanner {
    public static let prefix = "dsl."
    /// iOS は 1 アプリ 64 件まで(超えた分は黙って捨てられる)。タイマー等の分を残して 60。
    public static let limit = 60
    /// 毎月 / 毎年をくり返しの予約で書けない時(月末に寄せる回がある)に、先まで具体的な日付で予約する回数(#266)。
    /// 残りはアプリを開いた時(DSL の読み込み・レコードの変化)の予約し直しで先へ延ばす。上限の 60 件は select が近い順に守る。
    public static let monthlyAhead = 3
    public static let yearlyAhead = 2

    /// "dsl.<appId>.<specId>[.<recordId>]"。アプリ間で id が重なっても消し合わない。
    public static func identifier(appId: String, specId: String, recordId: String? = nil) -> String {
        prefix + appId + "." + specId + (recordId.map { "." + $0 } ?? "")
    }

    /// scheduleNotification ボタンで予約した分(DSL の再同期で消さないよう別の名前空間)。
    public static func actionIdentifier(appId: String, specId: String) -> String {
        prefix + appId + ".action." + specId
    }

    public static func plan(
        appId: String, specs: [NotificationSpec], records: [String: [Record]],
        now: Date = Date(), calendar: Calendar = .current
    ) -> [PlannedNotification] {
        specs.flatMap { spec -> [PlannedNotification] in
            planSpec(appId: appId, spec: spec, records: records, now: now, calendar: calendar).map { $0.marked(alarm: spec.alarm) }
        }
    }

    private static func planSpec(
        appId: String, spec: NotificationSpec, records: [String: [Record]], now: Date, calendar: Calendar
    ) -> [PlannedNotification] {
        if spec.isConditional { return [] } // #265 条件の通知は予約しない(ConditionNotifications)
        if spec.isRecordBased, let entity = spec.entity {
            return (records[entity] ?? []).flatMap { planRecord(appId: appId, spec: spec, record: $0, now: now, calendar: calendar) }
        }
        guard let trigger = fixedTrigger(spec, now: now, calendar: calendar) else { return [] }
        return [PlannedNotification(identifier: identifier(appId: appId, specId: spec.id), title: spec.title, body: spec.body, trigger: trigger)]
    }

    /// ボタンで押した時の予約(固定時刻のみ)。
    public static func action(appId: String, spec: NotificationSpec, now: Date = Date(), calendar: Calendar = .current) -> PlannedNotification? {
        guard !spec.isConditional, let trigger = fixedTrigger(spec, now: now, calendar: calendar) else { return nil }
        return PlannedNotification(identifier: actionIdentifier(appId: appId, specId: spec.id), title: spec.title, body: spec.body, trigger: trigger)
            .marked(alarm: spec.alarm)
    }

    /// 近い順に上限まで。過ぎた 1 回きりは落とす。
    public static func select(_ all: [PlannedNotification], now: Date = Date(), calendar: Calendar = .current, limit: Int = limit) -> [(PlannedNotification, Date)] {
        var seen = Set<String>()
        return all
            .compactMap { p in p.nextFire(after: now, calendar: calendar).map { (p, $0) } }
            .sorted { $0.1 < $1.1 }
            .filter { seen.insert($0.0.identifier).inserted }
            .prefix(limit)
            .map { $0 }
    }

    // MARK: - 組み立て

    static func fixedTrigger(_ s: NotificationSpec, now: Date, calendar: Calendar) -> PlannedNotification.Trigger? {
        let (h, m) = parseTime(s.time) ?? (9, 0)
        switch s.repeat {
        case .daily: return .daily(hour: h, minute: m)
        case .weekly: return .weekly(weekday: weekdayNumber(s.weekday), hour: h, minute: m)
        case .once, .monthly, .yearly: // monthly / yearly はレコード基準だけ(backend が拒否)。来たら 1 回きり
            return calendar.nextDate(after: now, matching: DateComponents(hour: h, minute: m, second: 0), matchingPolicy: .nextTime).map { .at($0) }
        }
    }

    static func planRecord(appId: String, spec: NotificationSpec, record: Record, now: Date, calendar: Calendar) -> [PlannedNotification] {
        guard let field = spec.field else { return [] }
        if let ef = spec.enabledField, !record[ef].isNull, !record[ef].truthy { return [] }
        let offset = spec.offsetMinutes ?? 0
        let value = record[field]
        let trigger: PlannedNotification.Trigger
        if let (h, m) = parseTime(value.stringValue) {
            // 時刻だけ("HH:mm"・timePicker): 毎日 / 毎週 / 次のその時刻
            let total = ((h * 60 + m + offset) % 1440 + 1440) % 1440
            let (hh, mm) = (total / 60, total % 60)
            switch spec.repeat {
            case .daily: trigger = .daily(hour: hh, minute: mm)
            case .weekly: trigger = .weekly(weekday: weekdayNumber(spec.weekday), hour: hh, minute: mm)
            case .once, .monthly, .yearly: // 時刻だけでは日が無い(monthly / yearly は backend が拒否)
                guard let d = calendar.nextDate(after: now, matching: DateComponents(hour: hh, minute: mm, second: 0), matchingPolicy: .nextTime) else { return [] }
                trigger = .at(d)
            }
        } else if var date = FieldCoding.date(from: value) {
            if let s = value.stringValue, FieldCoding.dateFormatter.date(from: s) != nil {
                // 日付だけ: その日の time(既定 9:00)
                let (h, m) = parseTime(spec.time) ?? (9, 0)
                date = calendar.date(bySettingHour: h, minute: m, second: 0, of: date) ?? date
            }
            let fire = date.addingTimeInterval(TimeInterval(offset * 60))
            let c = calendar.dateComponents([.hour, .minute, .weekday], from: fire)
            switch spec.repeat {
            case .once:
                guard fire > now else { return [] }
                trigger = .at(fire)
            case .daily: trigger = .daily(hour: c.hour ?? 9, minute: c.minute ?? 0)
            case .weekly: trigger = .weekly(weekday: c.weekday ?? 2, hour: c.hour ?? 9, minute: c.minute ?? 0)
            case .monthly, .yearly:
                let id = identifier(appId: appId, specId: spec.id, recordId: record.id)
                let text = body(spec, record)
                let months = spec.repeat == .monthly ? 1 : 12
                switch repeatTriggers(anchor: date, months: months, offsetMinutes: offset, now: now, calendar: calendar) {
                case .repeating(let t):
                    return [PlannedNotification(identifier: id, title: spec.title, body: text, trigger: t)]
                case .dates(let ds):
                    // 同じレコードの n 回目: "<id>#<n>"(取消は "<spec id>." の前方一致なので一緒に消える)
                    return ds.enumerated().map { PlannedNotification(identifier: id + "#\($0.offset)", title: spec.title, body: text, trigger: .at($0.element)) }
                }
            }
        } else {
            return []
        }
        return [PlannedNotification(identifier: identifier(appId: appId, specId: spec.id, recordId: record.id), title: spec.title, body: body(spec, record), trigger: trigger)]
    }

    private static func body(_ spec: NotificationSpec, _ record: Record) -> String {
        let label = spec.titleField.map { record[$0] }.flatMap { $0.stringValue ?? ($0.isNull ? nil : $0.displayString) } ?? ""
        return label.isEmpty ? spec.body : (spec.body.isEmpty ? label : label + "\n" + spec.body)
    }

    enum RepeatPlan: Equatable {
        case repeating(PlannedNotification.Trigger)
        case dates([Date])
    }

    /// 毎月 / 毎年の通知(#266)。回 k = 起点 + k か月(日は月末に寄せる・`RepeatDate`)、発火 = 回 + offset。
    /// - 発火の日時(毎月なら日・時・分、毎年なら月・日・時・分)が 4 年分ずっと同じで、次の発火がくり返しの次の一致と同じなら
    ///   `UNCalendarNotificationTrigger` のくり返し 1 件(アプリを開かなくても鳴り続ける)。
    /// - 月末に寄せる回がある(29〜31 日・2/29・ずらすと月をまたぐ)・起点がまだ先なら、近い順に `monthlyAhead` / `yearlyAhead` 件の具体的な日時。
    static func repeatTriggers(anchor: Date, months: Int, offsetMinutes: Int, now: Date, calendar: Calendar) -> RepeatPlan {
        func fire(_ k: Int) -> Date? {
            RepeatDate.occurrence(anchor, months: months, index: k, calendar: calendar)?.addingTimeInterval(TimeInterval(offsetMinutes * 60))
        }
        // offset は ±7 日まで(backend)なので、暦の月の差から 1 回戻れば取りこぼさない
        var k = max(RepeatDate.monthsBetween(anchor, now, calendar: calendar) / months - 1, 0)
        while let f = fire(k), f <= now { k += 1 }
        let ahead = months == 1 ? monthlyAhead : yearlyAhead
        let upcoming = (k..<(k + ahead)).compactMap(fire)
        guard let first = upcoming.first else { return .dates([]) }

        let keys: Set<Calendar.Component> = months == 1 ? [.day, .hour, .minute] : [.month, .day, .hour, .minute]
        let span = months == 1 ? 48 : 4 // 閏年の周期 4 年分
        let comps = (k..<(k + span)).compactMap(fire).map { calendar.dateComponents(keys, from: $0) }
        if let c = comps.first, comps.count == span, comps.allSatisfy({ $0 == c }),
           let day = c.day, let hour = c.hour, let minute = c.minute {
            let t: PlannedNotification.Trigger = months == 1
                ? .monthly(day: day, hour: hour, minute: minute)
                : .yearly(month: c.month ?? 1, day: day, hour: hour, minute: minute)
            if PlannedNotification(identifier: "", title: "", body: "", trigger: t).nextFire(after: now, calendar: calendar) == first {
                return .repeating(t)
            }
        }
        return .dates(upcoming)
    }

    public static func parseTime(_ t: String?) -> (Int, Int)? {
        guard let t else { return nil }
        // 部分が全部数字の時だけ(compactMap で捨てると "…T23:00:00+09:00" が 0:00 と読まれていた)
        let parts = t.split(separator: ":", omittingEmptySubsequences: false)
        guard parts.count == 2, let h = Int(parts[0]), let m = Int(parts[1]), (0...23).contains(h), (0...59).contains(m) else { return nil }
        return (h, m)
    }

    public static func weekdayNumber(_ w: String?) -> Int {
        ["sun": 1, "mon": 2, "tue": 3, "wed": 4, "thu": 5, "fri": 6, "sat": 7][w ?? ""] ?? 2
    }
}

extension PlannedNotification {
    func marked(alarm: Bool?) -> PlannedNotification {
        var p = self
        p.alarm = alarm == true ? true : nil
        return p
    }
}
