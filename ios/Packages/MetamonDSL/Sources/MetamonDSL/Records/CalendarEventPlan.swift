import Foundation

/// `addToCalendar` で OS の「新規イベント」画面に埋める中身。EventKit を知らない値だけ(テストできるように)。
public struct CalendarEventDraft: Identifiable, Equatable, Sendable {
    public var id = UUID()
    public var title: String
    public var start: Date
    public var end: Date
    public var isAllDay: Bool
    public var location: String?
    public var notes: String?
    /// 始まりからの通知の位置(秒・負 = 前)。nil は通知なし
    public var alertOffset: TimeInterval?
}

/// レコードの値 → 予定。date の field は終日、datetime はその時刻から(終わりが無ければ durationMinutes・既定 60 分)。
public enum CalendarEventPlan {
    public enum Failure: Error, Equatable {
        /// 始まりの field が空・読めない(フォームで未入力など)。値はその field の名前
        case noStart(String)
    }

    public static let defaultMinutes = 60

    public static func build(_ spec: CalendarEventSpec, entity: Entity, data: [String: JSONValue]) -> Result<CalendarEventDraft, Failure> {
        let startField = entity.field(spec.startField)
        guard let start = data[spec.startField].flatMap(FieldCoding.date(from:)) else {
            return .failure(.noStart(startField?.name ?? spec.startField))
        }
        let isAllDay = startField?.type == .date
        var end = spec.endField.flatMap { data[$0] }.flatMap(FieldCoding.date(from:))
        if let e = end, e < start { end = nil } // 終わりが始まりより前は無視(入力の途中など)
        let fallback = isAllDay ? start : start.addingTimeInterval(TimeInterval((spec.durationMinutes ?? defaultMinutes) * 60))
        let titleId = spec.titleField ?? entity.displayTitleField
        let title = titleId.flatMap { text(data[$0]) } ?? entity.name
        return .success(CalendarEventDraft(
            title: title, start: start, end: end ?? fallback, isAllDay: isAllDay,
            location: spec.locationField.flatMap { text(data[$0]) },
            notes: spec.notesField.flatMap { text(data[$0]) },
            alertOffset: spec.alertMinutes.map { -TimeInterval(max($0, 0) * 60) }))
    }

    /// 文字の値(空白だけは無し)。数値は整数ならそのまま(「第 3 回」の 3 など)。
    static func text(_ value: JSONValue?) -> String? {
        switch value {
        case .string(let s):
            let t = s.trimmingCharacters(in: .whitespacesAndNewlines)
            return t.isEmpty ? nil : t
        case .number(let n):
            return n == n.rounded() ? String(Int(n)) : String(n)
        default:
            return nil
        }
    }
}
