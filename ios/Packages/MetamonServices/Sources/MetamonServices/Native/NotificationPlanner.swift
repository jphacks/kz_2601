import Foundation
import MetamonDSL

/// 予約する通知 1 件(#228)。アプリごとに永続化し、全アプリ分から近い順に iOS の上限内で予約し直す。
public struct PlannedNotification: Codable, Sendable, Equatable {
    public enum Trigger: Codable, Sendable, Equatable {
        case at(Date)
        case daily(hour: Int, minute: Int)
        case weekly(weekday: Int, hour: Int, minute: Int)
    }

    public var identifier: String
    public var title: String
    public var body: String
    public var trigger: Trigger

    public init(identifier: String, title: String, body: String, trigger: Trigger) {
        self.identifier = identifier
        self.title = title
        self.body = body
        self.trigger = trigger
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
        }
    }
}

/// DSL の通知定義 + レコード → 予約する通知の一覧(純粋関数・ユニットテスト対象)。
public enum NotificationPlanner {
    public static let prefix = "dsl."
    /// iOS は 1 アプリ 64 件まで(超えた分は黙って捨てられる)。タイマー等の分を残して 60。
    public static let limit = 60

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
            if spec.isRecordBased, let entity = spec.entity {
                return (records[entity] ?? []).compactMap { planRecord(appId: appId, spec: spec, record: $0, now: now, calendar: calendar) }
            }
            guard let trigger = fixedTrigger(spec, now: now, calendar: calendar) else { return [] }
            return [PlannedNotification(identifier: identifier(appId: appId, specId: spec.id), title: spec.title, body: spec.body, trigger: trigger)]
        }
    }

    /// ボタンで押した時の予約(固定時刻のみ)。
    public static func action(appId: String, spec: NotificationSpec, now: Date = Date(), calendar: Calendar = .current) -> PlannedNotification? {
        guard let trigger = fixedTrigger(spec, now: now, calendar: calendar) else { return nil }
        return PlannedNotification(identifier: actionIdentifier(appId: appId, specId: spec.id), title: spec.title, body: spec.body, trigger: trigger)
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
        case .once:
            return calendar.nextDate(after: now, matching: DateComponents(hour: h, minute: m, second: 0), matchingPolicy: .nextTime).map { .at($0) }
        }
    }

    static func planRecord(appId: String, spec: NotificationSpec, record: Record, now: Date, calendar: Calendar) -> PlannedNotification? {
        guard let field = spec.field else { return nil }
        if let ef = spec.enabledField, !record[ef].isNull, !record[ef].truthy { return nil }
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
            case .once:
                guard let d = calendar.nextDate(after: now, matching: DateComponents(hour: hh, minute: mm, second: 0), matchingPolicy: .nextTime) else { return nil }
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
                guard fire > now else { return nil }
                trigger = .at(fire)
            case .daily: trigger = .daily(hour: c.hour ?? 9, minute: c.minute ?? 0)
            case .weekly: trigger = .weekly(weekday: c.weekday ?? 2, hour: c.hour ?? 9, minute: c.minute ?? 0)
            }
        } else {
            return nil
        }
        let label = spec.titleField.map { record[$0] }.flatMap { $0.stringValue ?? ($0.isNull ? nil : $0.displayString) } ?? ""
        let body = label.isEmpty ? spec.body : (spec.body.isEmpty ? label : label + "\n" + spec.body)
        return PlannedNotification(identifier: identifier(appId: appId, specId: spec.id, recordId: record.id), title: spec.title, body: body, trigger: trigger)
    }

    public static func parseTime(_ t: String?) -> (Int, Int)? {
        guard let t else { return nil }
        let p = t.split(separator: ":").compactMap { Int($0) }
        guard p.count == 2, (0...23).contains(p[0]), (0...59).contains(p[1]) else { return nil }
        return (p[0], p[1])
    }

    public static func weekdayNumber(_ w: String?) -> Int {
        ["sun": 1, "mon": 2, "tue": 3, "wed": 4, "thu": 5, "fri": 6, "sat": 7][w ?? ""] ?? 2
    }
}
