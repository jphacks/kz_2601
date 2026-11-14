import XCTest
@testable import MetamonDSL

/// #2082 開発用メニューの日付ずらし(`DSLClock.dayOffset`)
final class DSLClockShiftTests: XCTestCase {
    private let cal = Calendar.current

    override func tearDown() {
        DSLClock.dayOffset = 0
        DSLClock.fixed = nil
        super.tearDown()
    }

    private func dayKey(_ d: Date) -> String { FieldCoding.dateFormatter.string(from: d) }

    func testZeroOffsetIsBaseNow() {
        DSLClock.dayOffset = 0
        XCTAssertEqual(DSLClock.now().timeIntervalSince(DSLClock.baseNow()), 0, accuracy: 1)
    }

    func testOffsetShiftsDayKeepsTime() {
        let base = DSLClock.baseNow()
        DSLClock.dayOffset = 3
        let now = DSLClock.now()
        XCTAssertEqual(dayKey(now), dayKey(cal.date(byAdding: .day, value: 3, to: base)!))
        XCTAssertEqual(cal.component(.hour, from: now), cal.component(.hour, from: base), "時刻はそのまま")
        DSLClock.dayOffset = -1
        XCTAssertEqual(dayKey(DSLClock.now()), dayKey(cal.date(byAdding: .day, value: -1, to: DSLClock.baseNow())!))
    }

    func testFixedWinsOverOffset() {
        let fixed = Date(timeIntervalSince1970: 1_800_000_000)
        DSLClock.fixed = fixed
        DSLClock.dayOffset = 5
        XCTAssertEqual(DSLClock.now(), fixed)
    }

    func testResetRestoresRealTime() {
        DSLClock.dayOffset = 7
        DSLClock.dayOffset = 0
        XCTAssertEqual(DSLClock.now().timeIntervalSince(Date()), 0, accuracy: 1)
    }

    func testDailyResetAndStreakFollowShift() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"habit","name":"習慣","fields":[{"id":"done","name":"済み","type":"date"}]}
        """#.utf8))
        let today = DSLClock.baseNow()
        let days = (0..<3).map { dayKey(cal.date(byAdding: .day, value: -$0, to: today)!) }.sorted()
        let record = Record(id: "r", data: ["done": .string(days.last!), "done__dates": .array(days.map { .string($0) })])
        let streak = try JSONDecoder().decode(Expr.self, from: Data(#"{"op":"streak","args":[{"ref":"done"}]}"#.utf8))
        func evalStreak() -> Double? {
            ExprEvaluator(entities: [entity], records: { _ in [record] }).evaluate(streak, row: .init(entity: entity, record: record))?.number
        }

        XCTAssertTrue(DailyCheck.isDone(record["done"], today: DSLClock.now()))
        XCTAssertEqual(evalStreak(), 3)

        DSLClock.dayOffset = 1 // 明日: チェックは外れて見え、昨日までの連続は残る
        XCTAssertFalse(DailyCheck.isDone(record["done"], today: DSLClock.now()))
        XCTAssertEqual(evalStreak(), 3)

        DSLClock.dayOffset = 2 // 明後日: 1 日空いたので途切れる
        XCTAssertEqual(evalStreak(), 0)

        DSLClock.dayOffset = 0
        XCTAssertTrue(DailyCheck.isDone(record["done"], today: DSLClock.now()))
        XCTAssertEqual(evalStreak(), 3)
    }

    func testTodayFilterFollowsShift() throws {
        let field = try JSONDecoder().decode(Field.self, from: Data(#"{"id":"d","name":"日付","type":"date"}"#.utf8))
        let filter = Filter(field: "d", op: "today")
        let record = Record(id: "r", data: ["d": .string(dayKey(DSLClock.baseNow()))])
        XCTAssertTrue(Aggregator.matches(filter, record: record, field: field))
        DSLClock.dayOffset = 1
        XCTAssertFalse(Aggregator.matches(filter, record: record, field: field), "今日の集計から外れる")
    }
}
