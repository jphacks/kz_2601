import EventKit
import Foundation

/// 端末のカレンダーの予定・リマインダーを読む口(#1987・親 #1686)とリマインダーを足す口(#2222)。
/// 部品・アクションはまだ無い(決定待ち)。
/// `HealthDataSource` と同じ形: protocol + 実体(`LiveDeviceCalendar`)+ 偽物(`FakeDeviceCalendar`)。
/// 予定の題名・メモ・場所はログ・クラッシュ報告・サーバーに出さない(`description` も伏せる)。
public protocol DeviceCalendarSource: Sendable {
    /// 許可の状態。`requesting: true` で未決定なら OS の許可ダイアログを出す(iOS 17 のフルアクセス)。
    func authorization(for entity: DeviceCalendarEntity, requesting: Bool) async -> DeviceCalendarAccess
    /// `from..<to` に重なる予定(始まりの早い順)。期間は `DeviceCalendarLimits` で切り、件数も上限で切る。
    /// `calendarIds` が nil なら全カレンダー。フルアクセスが無ければ `DeviceCalendarError.notAuthorized`。
    func events(from: Date, to: Date, calendarIds: Set<String>?) async throws -> [DeviceCalendarEvent]
    /// リマインダー(期限の早い順・期限なしは後ろ)。`dueBefore` があれば期限がそれより前のものだけ(期限なしは除く)。
    /// 済みは直近 `DeviceCalendarLimits.daysBack` 日に済ませたものだけ。
    func reminders(includeCompleted: Bool, dueBefore: Date?, listIds: Set<String>?) async throws -> [DeviceReminder]
    /// 読む先を選ぶ画面のため: 予定のカレンダー / リマインダーのリスト(名前順)。
    func calendars() async throws -> [DeviceCalendarInfo]
    func reminderLists() async throws -> [DeviceCalendarInfo]
    /// リマインダーを 1 件足して、足したものの id を返す(#2222)。済みにする・消す口はまだ無い。
    /// `listId` が nil なら既定のリスト。`due` があるときだけ期日を入れる(`dueHasTime: false` なら日付だけ)。
    /// フルアクセスが無ければ `DeviceCalendarError.notAuthorized`、書けるリストが無ければ `.noReminderList`。
    func addReminder(title: String, due: Date?, dueHasTime: Bool, notes: String?, listId: String?) async throws -> String
    /// 端末のカレンダー・リマインダーが変わった(`EKEventStoreChanged`)ことを知らせる。中身は渡さない。
    func changes() -> AsyncStream<Void>
}

public enum DeviceCalendarEntity: String, Sendable, CaseIterable {
    case events
    case reminders
}

/// EventKit の許可状態を自前の値にしたもの(iOS 17 以降の分類)。
public enum DeviceCalendarAccess: String, Sendable, Equatable {
    case notDetermined
    case denied
    case restricted
    /// 予定の書き込みだけ(`requestWriteOnlyAccessToEvents`)。読めない
    case writeOnly
    case fullAccess

    public var canRead: Bool { self == .fullAccess }

    /// リマインダーを足せるか。リマインダーには予定の `.writeOnly` にあたる「追加だけ」が無いのでフルアクセスが要る。
    public var canAddReminder: Bool { self == .fullAccess }

    /// iOS 16 以前の `.authorized` はフルアクセスとして扱う。将来増えた値は「制限」(読まない)。
    public init(_ status: EKAuthorizationStatus) {
        switch status {
        case .notDetermined: self = .notDetermined
        case .denied: self = .denied
        case .restricted: self = .restricted
        case .writeOnly: self = .writeOnly
        case .fullAccess: self = .fullAccess
        @unknown default: self = .restricted
        }
    }
}

public enum DeviceCalendarError: LocalizedError, Equatable {
    /// フルアクセスが無い(未決定・拒否・制限・書き込みだけ)
    case notAuthorized(DeviceCalendarAccess)
    /// 書けるリマインダーのリストが 1 つも無い(既定のリストも取れない)
    case noReminderList
    /// 許可もリストもあるのに保存できなかった(EventKit の失敗)
    case writeFailed

    public var errorDescription: String? {
        switch self {
        case .notAuthorized(.writeOnly): L("カレンダーの読み取りが許可されていません(追加だけ許可されています)")
        case .notAuthorized: L("カレンダー・リマインダーへのアクセスが許可されていません")
        case .noReminderList: L("書き込めるリマインダーのリストがありません")
        case .writeFailed: L("リマインダーを保存できませんでした")
        }
    }
}

/// 読む期間・件数の上限。超えた分は切る(エラーにしない)。
public enum DeviceCalendarLimits {
    /// 今日から過去に読む日数(予定・済みのリマインダー)
    public static let daysBack = 31
    /// 今日から先に読む日数(予定)
    public static let daysAhead = 31
    /// 1 回で返す件数
    public static let maxItems = 200

    /// `from..<to` を「今日 0:00 - daysBack 日」〜「今日 0:00 + daysAhead + 1 日」に収める。空なら nil。
    public static func clamp(from: Date, to: Date, now: Date = Date(), calendar: Calendar = .current) -> DateInterval? {
        let today = calendar.startOfDay(for: now)
        guard let lower = calendar.date(byAdding: .day, value: -daysBack, to: today),
              let upper = calendar.date(byAdding: .day, value: daysAhead + 1, to: today) else { return nil }
        let start = max(from, lower)
        let end = min(to, upper)
        guard start < end else { return nil }
        return DateInterval(start: start, end: end)
    }
}

/// 予定のカレンダー / リマインダーのリスト。色は `#RRGGBB`(sRGB)。
public struct DeviceCalendarInfo: Sendable, Equatable, Identifiable {
    public var id: String
    public var name: String
    public var colorHex: String?
    public init(id: String, name: String, colorHex: String?) {
        self.id = id
        self.name = name
        self.colorHex = colorHex
    }
}

public struct DeviceCalendarEvent: Sendable, Equatable, Identifiable, CustomStringConvertible, CustomDebugStringConvertible {
    public var id: String
    public var title: String
    public var start: Date
    /// 終わり(含まない)。終日の予定は最終日の翌日 0:00 に揃える(EventKit は 23:59:59 を返す)。
    public var end: Date
    public var isAllDay: Bool
    public var calendar: DeviceCalendarInfo
    public var location: String?

    public init(id: String, title: String, start: Date, end: Date, isAllDay: Bool, calendar: DeviceCalendarInfo, location: String?) {
        self.id = id
        self.title = title
        self.start = start
        self.end = end
        self.isAllDay = isAllDay
        self.calendar = calendar
        self.location = location
    }

    /// 題名・場所は出さない(誤ってログに入れても漏れない)
    public var description: String { "DeviceCalendarEvent(id: \(id), allDay: \(isAllDay))" }
    public var debugDescription: String { description }
}

public struct DeviceReminder: Sendable, Equatable, Identifiable, CustomStringConvertible, CustomDebugStringConvertible {
    public var id: String
    public var title: String
    /// 期限。時刻なし(日付だけ)の期限はその日の 0:00
    public var due: Date?
    public var dueHasTime: Bool
    public var isCompleted: Bool
    public var completedAt: Date?
    public var list: DeviceCalendarInfo

    public init(id: String, title: String, due: Date?, dueHasTime: Bool, isCompleted: Bool, completedAt: Date?, list: DeviceCalendarInfo) {
        self.id = id
        self.title = title
        self.due = due
        self.dueHasTime = dueHasTime
        self.isCompleted = isCompleted
        self.completedAt = completedAt
        self.list = list
    }

    public var description: String { "DeviceReminder(id: \(id), completed: \(isCompleted))" }
    public var debugDescription: String { description }
}

/// リマインダーを足した結果。この 4 つだけ(単体テストの対象・`PhotoSaveResult` と同じ形)。
public enum ReminderAddResult: Sendable, Equatable {
    /// 「リマインダー」アプリに入った。id はあとで参照するため
    case added(id: String)
    /// フルアクセスが無い(未決定のまま拒否・拒否・制限・書き込みだけ)。設定へ案内できる
    case notAuthorized(DeviceCalendarAccess)
    /// 書けるリストが 1 つも無い(既定のリストも取れない)
    case noList
    /// 許可もリストもあるのに保存できなかった
    case failed

    public var addedId: String? {
        if case let .added(id) = self { return id }
        return nil
    }
}

@MainActor
public enum DeviceCalendar {
    /// 読み書きする口の実体。アプリ起動時に 1 回だけ差し替える(`METAMON_MOCK=1` は `FakeDeviceCalendar.sample`)。
    public static var source: any DeviceCalendarSource = LiveDeviceCalendar()

    /// 許可を求めてからリマインダーを 1 件足す(#2222)。どの分岐でも落ちずに結果で返す。
    public static func addReminder(title: String, due: Date? = nil, dueHasTime: Bool = true,
                                   notes: String? = nil, listId: String? = nil) async -> ReminderAddResult {
        await addReminder(title: title, due: due, dueHasTime: dueHasTime, notes: notes, listId: listId, to: source)
    }

    /// 書く先を差し替えられる形(単体テストはこちらを呼ぶ)。
    public static func addReminder(title: String, due: Date?, dueHasTime: Bool, notes: String?, listId: String?,
                                   to source: any DeviceCalendarSource) async -> ReminderAddResult {
        let access = await source.authorization(for: .reminders, requesting: true)
        guard access.canAddReminder else { return .notAuthorized(access) }
        do {
            let id = try await source.addReminder(title: title, due: due, dueHasTime: dueHasTime, notes: notes, listId: listId)
            return .added(id: id)
        } catch let DeviceCalendarError.notAuthorized(state) {
            return .notAuthorized(state)
        } catch DeviceCalendarError.noReminderList {
            return .noList
        } catch {
            return .failed
        }
    }
}

// MARK: - 実体と偽物で共有する絞り込み(単体テストの対象)

enum DeviceCalendarQuery {
    /// 終日の予定の終わりを翌日 0:00 に揃える。
    static func normalizedAllDayEnd(start: Date, end: Date, calendar: Calendar = .current) -> Date {
        let lastDay = calendar.startOfDay(for: max(start, end.addingTimeInterval(-1)))
        return calendar.date(byAdding: .day, value: 1, to: lastDay) ?? end
    }

    /// 期間に重なるもの・カレンダーで絞り、始まり順(同時なら終日が先・題名順)に並べて上限で切る。
    static func events(_ all: [DeviceCalendarEvent], in range: DateInterval, calendarIds: Set<String>?) -> [DeviceCalendarEvent] {
        let hits = all.filter { e in
            e.start < range.end && e.end > range.start && (calendarIds.map { $0.contains(e.calendar.id) } ?? true)
        }
        let sorted = hits.sorted { a, b in
            if a.start != b.start { return a.start < b.start }
            if a.isAllDay != b.isAllDay { return a.isAllDay }
            return a.id < b.id
        }
        return Array(sorted.prefix(DeviceCalendarLimits.maxItems))
    }

    static func reminders(_ all: [DeviceReminder], includeCompleted: Bool, dueBefore: Date?, listIds: Set<String>?,
                          now: Date = Date(), calendar: Calendar = .current) -> [DeviceReminder] {
        let completedSince = calendar.date(byAdding: .day, value: -DeviceCalendarLimits.daysBack, to: calendar.startOfDay(for: now)) ?? now
        let hits = all.filter { r in
            if let listIds, !listIds.contains(r.list.id) { return false }
            if r.isCompleted {
                guard includeCompleted, let done = r.completedAt, done >= completedSince else { return false }
            }
            if let dueBefore {
                guard let due = r.due, due < dueBefore else { return false }
            }
            return true
        }
        let sorted = hits.sorted { a, b in
            switch (a.due, b.due) {
            case let (x?, y?) where x != y: return x < y
            case (.some, nil): return true
            case (nil, .some): return false
            default: return a.id < b.id
            }
        }
        return Array(sorted.prefix(DeviceCalendarLimits.maxItems))
    }

    /// 期日を EventKit の形にする。期日なしは nil(= `dueDateComponents` を入れない)。時刻なしは年月日だけ。
    static func dueComponents(due: Date?, hasTime: Bool, calendar: Calendar = .current) -> DateComponents? {
        guard let due else { return nil }
        let units: Set<Calendar.Component> = hasTime ? [.year, .month, .day, .hour, .minute] : [.year, .month, .day]
        return calendar.dateComponents(units, from: due)
    }

    /// 書く先のリスト: id の指定があればそれ、無ければ既定、既定も無ければ名前順の先頭。1 つも無ければ nil。
    static func writeTarget<T>(_ lists: [T], id: String?, defaultList: T?, identify: (T) -> String) -> T? {
        if let id { return lists.first { identify($0) == id } }
        return defaultList ?? lists.first
    }

    static func sortedInfos(_ infos: [DeviceCalendarInfo]) -> [DeviceCalendarInfo] {
        infos.sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    /// sRGB の 0...1 から `#RRGGBB`。
    static func hex(red: Double, green: Double, blue: Double) -> String {
        func byte(_ v: Double) -> Int { Int((min(max(v, 0), 1) * 255).rounded()) }
        return String(format: "#%02X%02X%02X", byte(red), byte(green), byte(blue))
    }
}

// MARK: - 偽物

/// `METAMON_MOCK=1`・テスト・プレビュー用。中身と許可の状態を外から決められる。
public struct FakeDeviceCalendar: DeviceCalendarSource {
    /// 偽物が受け取った書き込み(単体テストで中身を確かめる)
    public struct Written: Sendable, Equatable {
        public var reminder: DeviceReminder
        public var notes: String?
        /// 期日なしなら nil
        public var dueComponents: DateComponents?
    }

    public var access: [DeviceCalendarEntity: DeviceCalendarAccess]
    /// `requesting: true` で未決定の時に、ダイアログで選ばれたことにする状態
    public var grantOnRequest: DeviceCalendarAccess
    public var allEvents: [DeviceCalendarEvent]
    public var allReminders: [DeviceReminder]
    public var calendarList: [DeviceCalendarInfo]
    public var reminderListList: [DeviceCalendarInfo]
    public var now: @Sendable () -> Date
    /// 許可もリストもあるのに保存に失敗する場合(`.writeFailed` を試す)
    public var writeError: DeviceCalendarError?
    private let writes = WrittenBox()
    /// 足されたリマインダー(新しいものが後ろ)
    public var written: [Written] { writes.items }

    public init(access: [DeviceCalendarEntity: DeviceCalendarAccess] = [.events: .fullAccess, .reminders: .fullAccess],
                grantOnRequest: DeviceCalendarAccess = .fullAccess,
                events: [DeviceCalendarEvent] = [], reminders: [DeviceReminder] = [],
                calendars: [DeviceCalendarInfo] = [], reminderLists: [DeviceCalendarInfo] = [],
                writeError: DeviceCalendarError? = nil,
                now: @escaping @Sendable () -> Date = { Date() }) {
        self.writeError = writeError
        self.access = access
        self.grantOnRequest = grantOnRequest
        self.allEvents = events
        self.allReminders = reminders
        self.calendarList = calendars
        self.reminderListList = reminderLists
        self.now = now
    }

    public func authorization(for entity: DeviceCalendarEntity, requesting: Bool) async -> DeviceCalendarAccess {
        let current = access[entity] ?? .notDetermined
        return requesting && current == .notDetermined ? grantOnRequest : current
    }

    public func events(from: Date, to: Date, calendarIds: Set<String>?) async throws -> [DeviceCalendarEvent] {
        try check(.events)
        guard let range = DeviceCalendarLimits.clamp(from: from, to: to, now: now()) else { return [] }
        return DeviceCalendarQuery.events(allEvents, in: range, calendarIds: calendarIds)
    }

    public func reminders(includeCompleted: Bool, dueBefore: Date?, listIds: Set<String>?) async throws -> [DeviceReminder] {
        try check(.reminders)
        return DeviceCalendarQuery.reminders(allReminders, includeCompleted: includeCompleted, dueBefore: dueBefore, listIds: listIds, now: now())
    }

    public func calendars() async throws -> [DeviceCalendarInfo] {
        try check(.events)
        return DeviceCalendarQuery.sortedInfos(calendarList)
    }

    public func reminderLists() async throws -> [DeviceCalendarInfo] {
        try check(.reminders)
        return DeviceCalendarQuery.sortedInfos(reminderListList)
    }

    public func addReminder(title: String, due: Date?, dueHasTime: Bool, notes: String?, listId: String?) async throws -> String {
        let state = access[.reminders] ?? .notDetermined
        guard state.canAddReminder else { throw DeviceCalendarError.notAuthorized(state) }
        if let writeError { throw writeError }
        let lists = DeviceCalendarQuery.sortedInfos(reminderListList)
        guard let list = DeviceCalendarQuery.writeTarget(lists, id: listId, defaultList: nil, identify: \.id) else {
            throw DeviceCalendarError.noReminderList
        }
        let comps = DeviceCalendarQuery.dueComponents(due: due, hasTime: dueHasTime)
        let reminder = DeviceReminder(id: "fake-added-\(writes.items.count + 1)", title: title, due: due,
                                      dueHasTime: comps?.hour != nil, isCompleted: false, completedAt: nil, list: list)
        writes.append(Written(reminder: reminder, notes: notes, dueComponents: comps))
        return reminder.id
    }

    public func changes() -> AsyncStream<Void> { AsyncStream { $0.finish() } }

    private func check(_ entity: DeviceCalendarEntity) throws {
        let a = access[entity] ?? .notDetermined
        guard a.canRead else { throw DeviceCalendarError.notAuthorized(a) }
    }

    /// `METAMON_MOCK=1` の固定の中身(今日を基準に作る)。
    public static var sample: FakeDeviceCalendar {
        let cal = Calendar.current
        let today = cal.startOfDay(for: Date())
        func at(_ day: Int, _ hour: Int) -> Date { cal.date(byAdding: .hour, value: day * 24 + hour, to: today) ?? today }
        let work = DeviceCalendarInfo(id: "fake-work", name: "仕事", colorHex: "#1E88E5")
        let home = DeviceCalendarInfo(id: "fake-home", name: "家", colorHex: "#43A047")
        let todo = DeviceCalendarInfo(id: "fake-todo", name: "リマインダー", colorHex: "#FB8C00")
        return FakeDeviceCalendar(
            events: [
                DeviceCalendarEvent(id: "e1", title: "定例ミーティング", start: at(0, 10), end: at(0, 11), isAllDay: false, calendar: work, location: "会議室 A"),
                DeviceCalendarEvent(id: "e2", title: "買い出し", start: at(0, 18), end: at(0, 19), isAllDay: false, calendar: home, location: nil),
                DeviceCalendarEvent(id: "e3", title: "旅行", start: at(2, 0), end: at(4, 0), isAllDay: true, calendar: home, location: "金沢"),
            ],
            reminders: [
                DeviceReminder(id: "r1", title: "牛乳を買う", due: at(0, 17), dueHasTime: true, isCompleted: false, completedAt: nil, list: todo),
                DeviceReminder(id: "r2", title: "書類を出す", due: at(1, 0), dueHasTime: false, isCompleted: false, completedAt: nil, list: todo),
                DeviceReminder(id: "r3", title: "本を返す", due: nil, dueHasTime: false, isCompleted: false, completedAt: nil, list: todo),
            ],
            calendars: [work, home],
            reminderLists: [todo]
        )
    }
}

// MARK: - 実体

/// EventKit から読む。`EKEventStore` は 1 つだけ持つ(作るたびに重い・変更通知もこの store から来る)。
/// Info.plist に説明文が無いビルド(審査用の版など)では許可を求めずに `.restricted` を返す(求めると落ちるため)。
public final class LiveDeviceCalendar: DeviceCalendarSource, @unchecked Sendable {
    private let store = EKEventStore()

    public init() {}

    static func usageKey(for entity: DeviceCalendarEntity) -> String {
        switch entity {
        case .events: "NSCalendarsFullAccessUsageDescription"
        case .reminders: "NSRemindersFullAccessUsageDescription"
        }
    }

    /// 説明文の有無は生の Info.plist で見る(`object(forInfoDictionaryKey:)` は訳を返すので、審査用ビルドで
    /// Info.plist から消しても `.lproj` に訳が残っていると「ある」と判定してしまう)。
    static func hasUsageDescription(for entity: DeviceCalendarEntity, in info: [String: Any]?) -> Bool {
        guard let text = info?[usageKey(for: entity)] as? String else { return false }
        return !text.isEmpty
    }

    private static func ekType(_ entity: DeviceCalendarEntity) -> EKEntityType {
        entity == .events ? .event : .reminder
    }

    public func authorization(for entity: DeviceCalendarEntity, requesting: Bool) async -> DeviceCalendarAccess {
        let current = DeviceCalendarAccess(EKEventStore.authorizationStatus(for: Self.ekType(entity)))
        guard requesting, current == .notDetermined else { return current }
        guard Self.hasUsageDescription(for: entity, in: Bundle.main.infoDictionary) else { return .restricted }
        do {
            _ = entity == .events ? try await store.requestFullAccessToEvents() : try await store.requestFullAccessToReminders()
        } catch {
            return .restricted
        }
        return DeviceCalendarAccess(EKEventStore.authorizationStatus(for: Self.ekType(entity)))
    }

    public func events(from: Date, to: Date, calendarIds: Set<String>?) async throws -> [DeviceCalendarEvent] {
        try check(.events)
        guard let range = DeviceCalendarLimits.clamp(from: from, to: to) else { return [] }
        let calendars = calendarIds.map { ids in store.calendars(for: .event).filter { ids.contains($0.calendarIdentifier) } }
        if let calendars, calendars.isEmpty { return [] } // 空配列を渡すと全カレンダーになるので先に返す
        let predicate = store.predicateForEvents(withStart: range.start, end: range.end, calendars: calendars)
        let mapped = store.events(matching: predicate).map(Self.event)
        return DeviceCalendarQuery.events(mapped, in: range, calendarIds: calendarIds)
    }

    public func reminders(includeCompleted: Bool, dueBefore: Date?, listIds: Set<String>?) async throws -> [DeviceReminder] {
        try check(.reminders)
        let lists = listIds.map { ids in store.calendars(for: .reminder).filter { ids.contains($0.calendarIdentifier) } }
        if let lists, lists.isEmpty { return [] }
        var predicates = [store.predicateForIncompleteReminders(withDueDateStarting: nil, ending: dueBefore, calendars: lists)]
        if includeCompleted {
            let since = Calendar.current.date(byAdding: .day, value: -DeviceCalendarLimits.daysBack, to: Calendar.current.startOfDay(for: Date()))
            predicates.append(store.predicateForCompletedReminders(withCompletionDateStarting: since, ending: Date(), calendars: lists))
        }
        var mapped: [DeviceReminder] = []
        for p in predicates { mapped += await fetch(p).map(Self.reminder) }
        return DeviceCalendarQuery.reminders(mapped, includeCompleted: includeCompleted, dueBefore: dueBefore, listIds: listIds)
    }

    public func calendars() async throws -> [DeviceCalendarInfo] {
        try check(.events)
        return DeviceCalendarQuery.sortedInfos(store.calendars(for: .event).map(Self.info))
    }

    public func reminderLists() async throws -> [DeviceCalendarInfo] {
        try check(.reminders)
        return DeviceCalendarQuery.sortedInfos(store.calendars(for: .reminder).map(Self.info))
    }

    public func addReminder(title: String, due: Date?, dueHasTime: Bool, notes: String?, listId: String?) async throws -> String {
        let state = DeviceCalendarAccess(EKEventStore.authorizationStatus(for: .reminder))
        guard state.canAddReminder else { throw DeviceCalendarError.notAuthorized(state) }
        let lists = store.calendars(for: .reminder).filter(\.allowsContentModifications)
        let fallback = store.defaultCalendarForNewReminders().flatMap { $0.allowsContentModifications ? $0 : nil }
        guard let list = DeviceCalendarQuery.writeTarget(lists, id: listId, defaultList: fallback, identify: \.calendarIdentifier) else {
            throw DeviceCalendarError.noReminderList
        }
        let reminder = EKReminder(eventStore: store)
        reminder.calendar = list
        reminder.title = title
        reminder.notes = notes
        reminder.dueDateComponents = DeviceCalendarQuery.dueComponents(due: due, hasTime: dueHasTime)
        do {
            try store.save(reminder, commit: true)
        } catch {
            throw DeviceCalendarError.writeFailed
        }
        return reminder.calendarItemIdentifier
    }

    public func changes() -> AsyncStream<Void> {
        AsyncStream { continuation in
            let token = NotificationCenter.default.addObserver(forName: .EKEventStoreChanged, object: store, queue: nil) { _ in
                continuation.yield()
            }
            let box = ObserverBox(token)
            continuation.onTermination = { _ in NotificationCenter.default.removeObserver(box.token) }
        }
    }

    private func check(_ entity: DeviceCalendarEntity) throws {
        let a = DeviceCalendarAccess(EKEventStore.authorizationStatus(for: Self.ekType(entity)))
        guard a.canRead else { throw DeviceCalendarError.notAuthorized(a) }
    }

    private func fetch(_ predicate: NSPredicate) async -> [EKReminder] {
        await withCheckedContinuation { cont in
            store.fetchReminders(matching: predicate) { cont.resume(returning: $0 ?? []) }
        }
    }

    private static func info(_ c: EKCalendar) -> DeviceCalendarInfo {
        DeviceCalendarInfo(id: c.calendarIdentifier, name: c.title, colorHex: hex(c.cgColor))
    }

    private static func hex(_ color: CGColor?) -> String? {
        guard let color, let srgb = CGColorSpace(name: CGColorSpace.sRGB),
              let c = color.converted(to: srgb, intent: .defaultIntent, options: nil)?.components, c.count >= 3 else { return nil }
        return DeviceCalendarQuery.hex(red: Double(c[0]), green: Double(c[1]), blue: Double(c[2]))
    }

    private static func event(_ e: EKEvent) -> DeviceCalendarEvent {
        let start: Date = e.startDate
        let rawEnd: Date = e.endDate ?? start
        return DeviceCalendarEvent(
            id: e.calendarItemIdentifier + "@" + String(Int(start.timeIntervalSince1970)), // 繰り返しの各回を区別する
            title: e.title ?? "",
            start: start,
            end: e.isAllDay ? DeviceCalendarQuery.normalizedAllDayEnd(start: start, end: rawEnd) : rawEnd,
            isAllDay: e.isAllDay,
            calendar: info(e.calendar),
            location: e.location
        )
    }

    private static func reminder(_ r: EKReminder) -> DeviceReminder {
        let comps = r.dueDateComponents
        let due = comps.flatMap { c in (c.calendar ?? Calendar.current).date(from: c) }
        return DeviceReminder(
            id: r.calendarItemIdentifier,
            title: r.title ?? "",
            due: due,
            dueHasTime: comps?.hour != nil,
            isCompleted: r.isCompleted,
            completedAt: r.completionDate,
            list: info(r.calendar)
        )
    }
}

/// 偽物は struct なので、書き込みの記録だけ参照で持つ。
private final class WrittenBox: @unchecked Sendable {
    private(set) var items: [FakeDeviceCalendar.Written] = []
    func append(_ item: FakeDeviceCalendar.Written) { items.append(item) }
}

private final class ObserverBox: @unchecked Sendable {
    let token: NSObjectProtocol
    init(_ token: NSObjectProtocol) { self.token = token }
}
