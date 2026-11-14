#if METAMON_STG
import XCTest
import MetamonDSL
@testable import Metamon

/// #2082: 開発用メニューの日付ずらし。保存した日付が `DSLClock` に入り、連続記録・毎日リセットが変わり、戻すと実時刻に戻る
@MainActor
final class DebugClockSectionTests: XCTestCase {
    private var suiteName = ""
    private var defaults: UserDefaults!
    private var saved: DebugKnobStore!
    private let cal = Calendar.current

    override func setUp() {
        suiteName = "DebugClockSectionTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)
        saved = DebugKnobs.store
        DebugKnobs.store = DebugKnobStore(defaults: defaults, environment: { [:] })
    }

    override func tearDown() {
        DSLClock.dayOffset = 0
        DebugKnobs.store = saved
        defaults.removePersistentDomain(forName: suiteName)
        defaults = nil
    }

    private func key(_ d: Date) -> String { FieldCoding.dateFormatter.string(from: d) }
    private func day(_ n: Int) -> Date { cal.date(byAdding: .day, value: n, to: DSLClock.baseNow())! }

    func testApplySavesDayAndShiftsClock() {
        DebugClock.apply(day: day(1))
        XCTAssertEqual(DSLClock.dayOffset, 1)
        XCTAssertEqual(key(DSLClock.now()), key(day(1)))
        XCTAssertEqual(DebugKnobs.string(DebugClock.knob), key(day(1)), "起動し直しても残るよう保存する")
        XCTAssertTrue(DebugClock.isShifted)
    }

    func testMoveIsRelativeToShiftedDay() {
        DebugClock.move(days: 1)
        DebugClock.move(days: 1)
        XCTAssertEqual(DSLClock.dayOffset, 2)
        DebugClock.move(days: -3)
        XCTAssertEqual(DSLClock.dayOffset, -1)
        XCTAssertEqual(DebugClock.label, "\(key(day(-1)))(-1 日)")
    }

    func testLaunchReadsSavedDay() {
        DebugKnobs.set(DebugClock.knob, key(day(7)))
        DSLClock.dayOffset = 0
        DebugClock.applyLaunch()
        XCTAssertEqual(DSLClock.dayOffset, 7)
    }

    func testResetRestoresRealTime() {
        DebugClock.apply(day: day(5))
        DebugClockSection.reset()
        XCTAssertEqual(DSLClock.dayOffset, 0)
        XCTAssertNil(DebugKnobs.string(DebugClock.knob))
        XCTAssertEqual(DSLClock.now().timeIntervalSince(Date()), 0, accuracy: 1)
        XCTAssertFalse(DebugClock.isShifted)
    }

    func testStreakAndDailyResetFollowShift() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"habit","name":"習慣","fields":[{"id":"done","name":"済み","type":"date"}]}
        """#.utf8))
        let days = (0..<2).map { key(day(-$0)) }.sorted() // 昨日と今日
        let record = Record(id: "r", data: ["done": .string(days.last!), "done__dates": .array(days.map { .string($0) })])
        let streak = try JSONDecoder().decode(Expr.self, from: Data(#"{"op":"streak","args":[{"ref":"done"}]}"#.utf8))
        func evalStreak() -> Double? {
            ExprEvaluator(entities: [entity], records: { _ in [record] }).evaluate(streak, row: .init(entity: entity, record: record))?.number
        }
        XCTAssertEqual(evalStreak(), 2)
        XCTAssertTrue(DailyCheck.isDone(record["done"], today: DSLClock.now()))

        DebugClock.move(days: 1) // 明日: チェックは外れ、連続は昨日まで残る
        XCTAssertFalse(DailyCheck.isDone(record["done"], today: DSLClock.now()))
        XCTAssertEqual(evalStreak(), 2)
        DebugClock.move(days: 1) // 明後日: 途切れる
        XCTAssertEqual(evalStreak(), 0)

        DebugClockSection.reset()
        XCTAssertEqual(evalStreak(), 2)
        XCTAssertTrue(DailyCheck.isDone(record["done"], today: DSLClock.now()))
    }

    func testRegisteredFirst() {
        XCTAssertTrue(DebugMenuRegistry.discover().contains { $0 == DebugClockSection.self })
        XCTAssertEqual(DebugClockSection.order, 0)
    }
}
#endif
