import EventKit
import XCTest
@testable import MetamonRenderer

/// 端末のカレンダー・リマインダーの読む口(#1987)。偽物で、許可・期間・件数・終日の扱いを確かめる。
final class DeviceCalendarTests: XCTestCase {
    private var cal: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }()
    private let now = ISO8601DateFormatter().date(from: "2026-11-12T09:30:00+09:00")!
    private let work = DeviceCalendarInfo(id: "w", name: "仕事", colorHex: "#112233")
    private let home = DeviceCalendarInfo(id: "h", name: "家", colorHex: nil)

    private func day(_ offset: Int, _ hour: Int = 0) -> Date {
        cal.date(byAdding: .hour, value: offset * 24 + hour, to: cal.startOfDay(for: now))!
    }

    private func event(_ id: String, _ start: Date, _ end: Date, allDay: Bool = false, in c: DeviceCalendarInfo? = nil) -> DeviceCalendarEvent {
        DeviceCalendarEvent(id: id, title: "秘密の予定 \(id)", start: start, end: end, isAllDay: allDay, calendar: c ?? work, location: "自宅")
    }

    private func fake(access: [DeviceCalendarEntity: DeviceCalendarAccess] = [.events: .fullAccess, .reminders: .fullAccess],
                      events: [DeviceCalendarEvent] = [], reminders: [DeviceReminder] = []) -> FakeDeviceCalendar {
        let now = self.now
        return FakeDeviceCalendar(access: access, events: events, reminders: reminders,
                                  calendars: [home, work], reminderLists: [work], now: { now })
    }

    // MARK: 許可

    func testAccessMapsEveryEventKitStatus() {
        XCTAssertEqual(DeviceCalendarAccess(.notDetermined), .notDetermined)
        XCTAssertEqual(DeviceCalendarAccess(.denied), .denied)
        XCTAssertEqual(DeviceCalendarAccess(.restricted), .restricted)
        XCTAssertEqual(DeviceCalendarAccess(.writeOnly), .writeOnly)
        XCTAssertEqual(DeviceCalendarAccess(.fullAccess), .fullAccess)
        XCTAssertTrue(DeviceCalendarAccess.fullAccess.canRead)
        XCTAssertFalse(DeviceCalendarAccess.writeOnly.canRead)
    }

    func testReadsWithoutFullAccessThrowFixedError() async {
        for state in [DeviceCalendarAccess.notDetermined, .denied, .restricted, .writeOnly] {
            let f = fake(access: [.events: state, .reminders: state], events: [event("a", day(0, 10), day(0, 11))])
            do {
                _ = try await f.events(from: day(0), to: day(1), calendarIds: nil)
                XCTFail("\(state) で読めてしまった")
            } catch {
                XCTAssertEqual(error as? DeviceCalendarError, .notAuthorized(state))
            }
            do {
                _ = try await f.reminders(includeCompleted: true, dueBefore: nil, listIds: nil)
                XCTFail("\(state) で読めてしまった")
            } catch {
                XCTAssertEqual(error as? DeviceCalendarError, .notAuthorized(state))
            }
            await XCTAssertThrowsAsync { try await f.calendars() }
            await XCTAssertThrowsAsync { try await f.reminderLists() }
        }
    }

    func testRequestingOnlyChangesNotDetermined() async {
        var f = fake(access: [.events: .notDetermined, .reminders: .denied])
        f.grantOnRequest = .fullAccess
        let notAsked = await f.authorization(for: .events, requesting: false)
        XCTAssertEqual(notAsked, .notDetermined)
        let asked = await f.authorization(for: .events, requesting: true)
        XCTAssertEqual(asked, .fullAccess)
        let denied = await f.authorization(for: .reminders, requesting: true) // 拒否済みはダイアログを出さない
        XCTAssertEqual(denied, .denied)
    }

    func testErrorMessagesExist() {
        XCTAssertFalse((DeviceCalendarError.notAuthorized(.denied).errorDescription ?? "").isEmpty)
        XCTAssertNotEqual(DeviceCalendarError.notAuthorized(.writeOnly).errorDescription,
                          DeviceCalendarError.notAuthorized(.denied).errorDescription)
    }

    // MARK: 期間・件数

    func testClampCutsRangeToLimits() {
        let r = DeviceCalendarLimits.clamp(from: day(-100), to: day(100), now: now, calendar: cal)!
        XCTAssertEqual(r.start, day(-DeviceCalendarLimits.daysBack))
        XCTAssertEqual(r.end, day(DeviceCalendarLimits.daysAhead + 1))
        let inside = DeviceCalendarLimits.clamp(from: day(0, 9), to: day(0, 12), now: now, calendar: cal)!
        XCTAssertEqual(inside, DateInterval(start: day(0, 9), end: day(0, 12)))
        XCTAssertNil(DeviceCalendarLimits.clamp(from: day(40), to: day(50), now: now, calendar: cal)) // 範囲の外だけ
        XCTAssertNil(DeviceCalendarLimits.clamp(from: day(1), to: day(0), now: now, calendar: cal)) // 逆さま
    }

    func testEventsOverlapFilterSortAndCalendarIds() async throws {
        let f = fake(events: [
            event("late", day(0, 15), day(0, 16)),
            event("early", day(0, 9), day(0, 10), in: home),
            event("spans", day(-1, 22), day(0, 1)), // 前日から続く
            event("before", day(-1, 8), day(-1, 9)),
            event("far", day(60, 9), day(60, 10)),
        ])
        let todays = try await f.events(from: day(0), to: day(1), calendarIds: nil)
        XCTAssertEqual(todays.map(\.id), ["spans", "early", "late"])
        let onlyWork = try await f.events(from: day(0), to: day(1), calendarIds: ["w"])
        XCTAssertEqual(onlyWork.map(\.id), ["spans", "late"])
        let none = try await f.events(from: day(0), to: day(1), calendarIds: [])
        XCTAssertTrue(none.isEmpty)
        let farAway = try await f.events(from: day(59), to: day(61), calendarIds: nil) // 上限の外は読まない
        XCTAssertTrue(farAway.isEmpty)
    }

    func testEventsAreCappedAtMaxItems() async throws {
        let many = (0..<(DeviceCalendarLimits.maxItems + 50)).map { i in
            event(String(format: "%04d", i), day(0, 0).addingTimeInterval(Double(i) * 60), day(0, 1))
        }
        let got = try await fake(events: many.shuffled()).events(from: day(0), to: day(1), calendarIds: nil)
        XCTAssertEqual(got.count, DeviceCalendarLimits.maxItems)
        XCTAssertEqual(got.first?.id, "0000") // 早い方を残す
    }

    // MARK: 終日

    func testAllDayEndIsNextMidnight() {
        // EventKit は 11/14 の終日を 11/14 0:00〜23:59:59 で返す
        let start = day(2)
        let end = day(3).addingTimeInterval(-1)
        XCTAssertEqual(DeviceCalendarQuery.normalizedAllDayEnd(start: start, end: end, calendar: cal), day(3))
        // すでに翌日 0:00 のものはそのまま
        XCTAssertEqual(DeviceCalendarQuery.normalizedAllDayEnd(start: start, end: day(4), calendar: cal), day(4))
        // 終わりが始まりと同じ(壊れたデータ)でも 1 日分
        XCTAssertEqual(DeviceCalendarQuery.normalizedAllDayEnd(start: start, end: start, calendar: cal), day(3))
    }

    func testAllDayComesFirstAndDoesNotLeakIntoNextDay() async throws {
        let f = fake(events: [
            event("timed", day(1), day(1, 1)),
            event("allday", day(1), day(2), allDay: true),
        ])
        let tomorrow = try await f.events(from: day(1), to: day(2), calendarIds: nil)
        XCTAssertEqual(tomorrow.map(\.id), ["allday", "timed"])
        let dayAfter = try await f.events(from: day(2), to: day(3), calendarIds: nil)
        XCTAssertTrue(dayAfter.isEmpty)
    }

    // MARK: リマインダー

    func testRemindersFilterAndSort() async throws {
        func r(_ id: String, due: Date?, done: Date? = nil) -> DeviceReminder {
            DeviceReminder(id: id, title: "秘密 \(id)", due: due, dueHasTime: false, isCompleted: done != nil, completedAt: done, list: work)
        }
        let f = fake(reminders: [
            r("nodue", due: nil),
            r("later", due: day(5)),
            r("soon", due: day(1)),
            r("doneRecent", due: day(-1), done: day(-1)),
            r("doneOld", due: day(-60), done: day(-60)),
        ])
        let open = try await f.reminders(includeCompleted: false, dueBefore: nil, listIds: nil)
        XCTAssertEqual(open.map(\.id), ["soon", "later", "nodue"])
        let withDone = try await f.reminders(includeCompleted: true, dueBefore: nil, listIds: nil)
        XCTAssertEqual(withDone.map(\.id), ["doneRecent", "soon", "later", "nodue"])
        let dueSoon = try await f.reminders(includeCompleted: false, dueBefore: day(2), listIds: nil)
        XCTAssertEqual(dueSoon.map(\.id), ["soon"])
        let otherList = try await f.reminders(includeCompleted: true, dueBefore: nil, listIds: ["h"])
        XCTAssertTrue(otherList.isEmpty)
    }

    func testListsAreSortedByName() async throws {
        let got = try await fake().calendars()
        XCTAssertEqual(got.map(\.id), DeviceCalendarQuery.sortedInfos([home, work]).map(\.id))
        XCTAssertEqual(got.count, 2)
    }

    // MARK: 漏らさない・その他

    func testDescriptionsHideTitleAndLocation() {
        let e = event("x", day(0), day(1))
        XCTAssertFalse(String(describing: e).contains("秘密"))
        XCTAssertFalse(String(reflecting: e).contains("自宅"))
        let r = DeviceReminder(id: "y", title: "秘密", due: nil, dueHasTime: false, isCompleted: false, completedAt: nil, list: work)
        XCTAssertFalse("\(r)".contains("秘密"))
    }

    func testHexColor() {
        XCTAssertEqual(DeviceCalendarQuery.hex(red: 1, green: 0, blue: 0.5), "#FF0080")
        XCTAssertEqual(DeviceCalendarQuery.hex(red: -1, green: 2, blue: 0), "#00FF00")
    }

    func testUsageKeysAreIOS17FullAccess() {
        XCTAssertEqual(LiveDeviceCalendar.usageKey(for: .events), "NSCalendarsFullAccessUsageDescription")
        XCTAssertEqual(LiveDeviceCalendar.usageKey(for: .reminders), "NSRemindersFullAccessUsageDescription")
    }

    func testUsageDescriptionCheckReadsRawInfoDictionary() {
        let info: [String: Any] = ["NSCalendarsFullAccessUsageDescription": "予定を読みます", "NSRemindersFullAccessUsageDescription": ""]
        XCTAssertTrue(LiveDeviceCalendar.hasUsageDescription(for: .events, in: info))
        XCTAssertFalse(LiveDeviceCalendar.hasUsageDescription(for: .reminders, in: info)) // 空文字は無いのと同じ
        XCTAssertFalse(LiveDeviceCalendar.hasUsageDescription(for: .events, in: [:]))
        XCTAssertFalse(LiveDeviceCalendar.hasUsageDescription(for: .events, in: nil))
    }

    func testSampleHasTodayData() async throws {
        let s = FakeDeviceCalendar.sample
        let today = Calendar.current.startOfDay(for: Date())
        let events = try await s.events(from: today, to: today.addingTimeInterval(86_400), calendarIds: nil)
        XCTAssertFalse(events.isEmpty)
        let reminders = try await s.reminders(includeCompleted: false, dueBefore: nil, listIds: nil)
        XCTAssertEqual(reminders.count, 3)
    }

    private func XCTAssertThrowsAsync(_ body: () async throws -> Void, file: StaticString = #filePath, line: UInt = #line) async {
        do {
            try await body()
            XCTFail("throw しなかった", file: file, line: line)
        } catch {}
    }
}

/// リマインダーを足す口(#2222)。期日あり / なし・許可なし・リストが無い の 4 分岐を偽物で確かめる。
final class DeviceCalendarAddReminderTests: XCTestCase {
    private var cal: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }()
    private let due = ISO8601DateFormatter().date(from: "2026-11-20T17:30:00+09:00")!
    private let todo = DeviceCalendarInfo(id: "todo", name: "リマインダー", colorHex: nil)
    private let work = DeviceCalendarInfo(id: "work", name: "仕事", colorHex: nil)

    private func fake(access: DeviceCalendarAccess = .fullAccess,
                      lists: [DeviceCalendarInfo]? = nil,
                      writeError: DeviceCalendarError? = nil) -> FakeDeviceCalendar {
        FakeDeviceCalendar(access: [.events: access, .reminders: access], grantOnRequest: access,
                           reminderLists: lists ?? [todo, work], writeError: writeError)
    }

    // MARK: 期日あり

    func testAddsReminderWithDueDateAndTime() async throws {
        let f = fake()
        let id = try await f.addReminder(title: "牛乳を買う", due: due, dueHasTime: true, notes: "低脂肪", listId: nil)
        let got = try XCTUnwrap(f.written.first)
        XCTAssertEqual(f.written.count, 1)
        XCTAssertEqual(got.reminder.id, id)
        XCTAssertEqual(got.reminder.title, "牛乳を買う")
        XCTAssertEqual(got.reminder.due, due)
        XCTAssertTrue(got.reminder.dueHasTime)
        XCTAssertEqual(got.notes, "低脂肪")
        XCTAssertNotNil(got.dueComponents?.hour)
    }

    func testDueWithoutTimeKeepsOnlyYearMonthDay() throws {
        let comps = try XCTUnwrap(DeviceCalendarQuery.dueComponents(due: due, hasTime: false, calendar: cal))
        XCTAssertEqual(comps.year, 2026)
        XCTAssertEqual(comps.month, 11)
        XCTAssertEqual(comps.day, 20)
        XCTAssertNil(comps.hour)
        let withTime = DeviceCalendarQuery.dueComponents(due: due, hasTime: true, calendar: cal)
        XCTAssertEqual(withTime?.hour, 17)
        XCTAssertEqual(withTime?.minute, 30)
    }

    // MARK: 期日なし

    func testAddsReminderWithoutDueDate() async throws {
        let f = fake()
        _ = try await f.addReminder(title: "本を返す", due: nil, dueHasTime: true, notes: nil, listId: nil)
        let got = try XCTUnwrap(f.written.first)
        XCTAssertNil(got.dueComponents) // 期日なしは dueDateComponents を入れない
        XCTAssertNil(got.reminder.due)
        XCTAssertFalse(got.reminder.dueHasTime)
        XCTAssertNil(DeviceCalendarQuery.dueComponents(due: nil, hasTime: true, calendar: cal))
    }

    // MARK: 許可なし

    func testAddWithoutFullAccessThrowsAndWritesNothing() async {
        for state in [DeviceCalendarAccess.notDetermined, .denied, .restricted, .writeOnly] {
            XCTAssertFalse(state.canAddReminder)
            let f = fake(access: state)
            do {
                _ = try await f.addReminder(title: "秘密", due: nil, dueHasTime: false, notes: nil, listId: nil)
                XCTFail("\(state) で書けてしまった")
            } catch {
                XCTAssertEqual(error as? DeviceCalendarError, .notAuthorized(state))
            }
            XCTAssertTrue(f.written.isEmpty)
        }
        XCTAssertTrue(DeviceCalendarAccess.fullAccess.canAddReminder)
    }

    func testResultIsNotAuthorizedWhenAccessDenied() async {
        let f = fake(access: .denied)
        let result = await DeviceCalendar.addReminder(title: "秘密", due: nil, dueHasTime: false, notes: nil, listId: nil, to: f)
        XCTAssertEqual(result, .notAuthorized(.denied))
        XCTAssertNil(result.addedId)
        XCTAssertTrue(f.written.isEmpty)
    }

    // MARK: リストが無い

    func testAddWithoutAnyListThrowsNoReminderList() async {
        let f = fake(lists: [])
        do {
            _ = try await f.addReminder(title: "買い物", due: nil, dueHasTime: false, notes: nil, listId: nil)
            XCTFail("リストが無いのに書けてしまった")
        } catch {
            XCTAssertEqual(error as? DeviceCalendarError, .noReminderList)
        }
        let result = await DeviceCalendar.addReminder(title: "買い物", due: nil, dueHasTime: false, notes: nil, listId: nil, to: f)
        XCTAssertEqual(result, .noList)
    }

    func testUnknownListIdIsNoList() async {
        let f = fake()
        let result = await DeviceCalendar.addReminder(title: "買い物", due: due, dueHasTime: true, notes: nil, listId: "none", to: f)
        XCTAssertEqual(result, .noList)
        XCTAssertTrue(f.written.isEmpty)
    }

    // MARK: 書く先の選び方

    func testWriteTargetPrefersGivenIdThenDefaultThenFirst() {
        let lists = [todo, work]
        XCTAssertEqual(DeviceCalendarQuery.writeTarget(lists, id: "work", defaultList: todo, identify: \.id)?.id, "work")
        XCTAssertEqual(DeviceCalendarQuery.writeTarget(lists, id: nil, defaultList: work, identify: \.id)?.id, "work")
        XCTAssertEqual(DeviceCalendarQuery.writeTarget(lists, id: nil, defaultList: nil, identify: \.id)?.id, "todo")
        XCTAssertNil(DeviceCalendarQuery.writeTarget([DeviceCalendarInfo](), id: nil, defaultList: nil, identify: \.id))
        XCTAssertNil(DeviceCalendarQuery.writeTarget(lists, id: "none", defaultList: todo, identify: \.id))
    }

    func testAddsToNamedList() async throws {
        let f = fake()
        _ = try await f.addReminder(title: "資料を作る", due: nil, dueHasTime: false, notes: nil, listId: "work")
        XCTAssertEqual(f.written.first?.reminder.list.id, "work")
    }

    // MARK: 結果の形

    func testResultAddedCarriesIdAndSaveFailureIsFailed() async {
        let f = fake()
        let ok = await DeviceCalendar.addReminder(title: "牛乳", due: due, dueHasTime: true, notes: nil, listId: nil, to: f)
        XCTAssertEqual(ok, .added(id: "fake-added-1"))
        XCTAssertEqual(ok.addedId, "fake-added-1")

        let broken = fake(writeError: .writeFailed)
        let bad = await DeviceCalendar.addReminder(title: "牛乳", due: due, dueHasTime: true, notes: nil, listId: nil, to: broken)
        XCTAssertEqual(bad, .failed)
        XCTAssertTrue(broken.written.isEmpty)
    }

    func testErrorDescriptionsHideNothingSensitive() {
        XCTAssertNotNil(DeviceCalendarError.noReminderList.errorDescription)
        XCTAssertNotNil(DeviceCalendarError.writeFailed.errorDescription)
    }
}
