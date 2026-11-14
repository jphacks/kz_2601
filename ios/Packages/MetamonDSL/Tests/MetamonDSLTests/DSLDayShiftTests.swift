import XCTest
@testable import MetamonDSL

/// 日付のずらし(`DSLClock.shifted`・`dayOffset`・`debugNow`)と押した日の履歴の鍵(`DailyCheck.historyKey`)。
/// 端末の TZ に依らず通る(TZ=America/Los_Angeles でも回す)。
final class DSLDayShiftTests: XCTestCase {
    private let cal: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = .current
        return c
    }()

    private func at(_ y: Int, _ m: Int, _ d: Int, _ h: Int = 12, _ min: Int = 0, _ s: Int = 0) -> Date {
        cal.date(from: DateComponents(year: y, month: m, day: d, hour: h, minute: min, second: s))!
    }

    private func day(_ d: Date) -> String { FieldCoding.dateFormatter.string(from: d) }

    override func tearDown() {
        DSLClock.dayOffset = 0
        super.tearDown()
    }

    // MARK: - shifted

    func testShiftedMovesDaysAndKeepsTime() {
        let base = at(2026, 10, 30, 21, 45, 10)
        let cases: [(days: Int, day: String)] = [
            (0, "2026-10-30"),
            (1, "2026-10-31"),
            (2, "2026-11-01"), // 月をまたぐ(米国は夏時間の終わり)
            (3, "2026-11-02"),
            (-1, "2026-10-29"),
            (-30, "2026-09-30"),
            (365, "2027-10-30"),
        ]
        for c in cases {
            let d = DSLClock.shifted(base, days: c.days, calendar: cal)
            XCTAssertEqual(day(d), c.day, "days=\(c.days)")
            let t = cal.dateComponents([.hour, .minute, .second], from: d)
            XCTAssertEqual([t.hour, t.minute, t.second], [21, 45, 10], "時刻はそのまま days=\(c.days)")
        }
    }

    func testShiftedZeroIsTheSameInstant() {
        let base = at(2026, 3, 8, 2, 30)
        XCTAssertEqual(DSLClock.shifted(base, days: 0, calendar: cal), base)
    }

    func testShiftedAcrossLeapDay() {
        XCTAssertEqual(day(DSLClock.shifted(at(2028, 2, 28), days: 1, calendar: cal)), "2028-02-29")
        XCTAssertEqual(day(DSLClock.shifted(at(2028, 3, 1), days: -1, calendar: cal)), "2028-02-29")
        XCTAssertEqual(day(DSLClock.shifted(at(2027, 2, 28), days: 1, calendar: cal)), "2027-03-01")
    }

    // MARK: - dayOffset

    func testDayOffsetShiftsNowAndZeroRestores() {
        DSLClock.dayOffset = 3
        XCTAssertEqual(DSLClock.dayOffset, 3)
        XCTAssertEqual(DSLDayShift.shared.days, 3, "入れ物にも入る")
        XCTAssertEqual(day(DSLClock.now()), day(DSLClock.shifted(DSLClock.baseNow(), days: 3)))

        DSLClock.dayOffset = -2
        XCTAssertEqual(day(DSLClock.now()), day(DSLClock.shifted(DSLClock.baseNow(), days: -2)), "負の数は過去へ")

        DSLClock.dayOffset = 0
        XCTAssertEqual(day(DSLClock.now()), day(DSLClock.baseNow()))
    }

    func testFixedWinsOverDayOffset() {
        let fixed = at(2026, 10, 24, 9)
        DSLClock.fixed = fixed
        defer { DSLClock.fixed = nil }
        DSLClock.dayOffset = 5
        XCTAssertEqual(DSLClock.now(), fixed, "適合テストの固定の時刻はずらさない")
    }

    // MARK: - debugNow(METAMON_DEBUG_TODAY)

    func testDebugNowUsesEnvDayWithRealTime() {
        let real = at(2026, 10, 24, 23, 59, 30)
        let d = DSLClock.debugNow(real, env: ["METAMON_DEBUG_TODAY": "2026-12-31"], calendar: cal)
        XCTAssertEqual(d.map(day), "2026-12-31")
        let t = d.map { cal.dateComponents([.hour, .minute, .second], from: $0) }
        XCTAssertEqual([t?.hour, t?.minute, t?.second], [23, 59, 30], "時:分:秒は実時刻")

        let past = DSLClock.debugNow(at(2026, 10, 24, 0, 0, 1), env: ["METAMON_DEBUG_TODAY": "2025-01-01"], calendar: cal)
        XCTAssertEqual(past.map(day), "2025-01-01", "過去の日も入れられる")
    }

    func testDebugNowBrokenValuesAreNil() {
        let real = at(2026, 10, 24)
        for raw in ["", " ", "tomorrow", "2026-13-01", "2026-10-25T10:00:00Z", "-3"] {
            XCTAssertNil(DSLClock.debugNow(real, env: ["METAMON_DEBUG_TODAY": raw], calendar: cal), "raw=\(raw)")
        }
        XCTAssertNil(DSLClock.debugNow(real, env: ["SIMCTL_CHILD_METAMON_DEBUG_TODAY": "2026-10-25"], calendar: cal),
                     "接頭辞付きは simctl が外す。パッケージは素の名前だけ読む")
    }

    // MARK: - historyKey

    func testHistoryKeyAppendsSuffix() {
        XCTAssertEqual(DailyCheck.historySuffix, "__dates")
        XCTAssertEqual(DailyCheck.historyKey("taken_on"), "taken_on__dates")
        XCTAssertEqual(DailyCheck.historyKey("done"), "done__dates")
        XCTAssertEqual(DailyCheck.historyKey(""), "__dates")
        XCTAssertNotEqual(DailyCheck.historyKey("a"), DailyCheck.historyKey("a__dates"), "二重に付けない")
    }

    func testHistoryKeyIsWhatPatchAndDatesUse() {
        let today = at(2026, 10, 24)
        let patch = DailyCheck.patch(Record(id: "r", data: [:]), field: "taken_on", on: true, today: today)
        XCTAssertEqual(Set(patch.keys), ["taken_on", DailyCheck.historyKey("taken_on")])
        let record = Record(id: "r", data: patch)
        XCTAssertEqual(DailyCheck.dates(record, field: "taken_on"), [day(today)])
    }
}
