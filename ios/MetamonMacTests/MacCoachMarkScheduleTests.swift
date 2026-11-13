import XCTest
@testable import Metamon

/// #2242 コーチマークを出す順の決まり(`CoachMarkSchedule.next`)だけを、保存から切り離して確かめる。
/// Mac でも iOS と同じ順番・同じ保存先で出す(共有の ios/Shared/Onboarding/CoachMarks.swift)ことを、Mac のターゲットでも確かめる。
final class MacCoachMarkScheduleTests: XCTestCase {
    private let all = Set(CoachMark.allCases)
    private var calendar: Calendar {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }
    /// 2027-01-15 09:20 JST(日付の境目から離れた時刻)
    private let day1 = Date(timeIntervalSince1970: 1_800_000_000)
    private var day1Later: Date { day1.addingTimeInterval(60 * 60) }
    private var day2: Date { day1.addingTimeInterval(86_400) }

    private func next(eligible: Set<CoachMark> = Set(CoachMark.allCases),
                      done: Set<CoachMark> = [],
                      optedOut: Bool = false,
                      replaying: Bool = false,
                      lastShown: CoachMarkSchedule.LastShown? = nil,
                      now: Date? = nil) -> CoachMark? {
        CoachMarkSchedule.next(eligible: eligible, done: done, optedOut: optedOut, replaying: replaying,
                               lastShown: lastShown, now: now ?? day1, calendar: calendar)
    }

    // MARK: - 順番どおり

    func testPicksFirstInDeclaredOrder() {
        XCTAssertEqual(next(), .fabLongPress, "大事なものから出す")
        XCTAssertEqual(next(eligible: [.switcherReorder, .fabDrag]), .fabDrag, "使える物の中でも宣言順")
        XCTAssertEqual(CoachMark.allCases, [.fabLongPress, .fabDrag, .switcherReorder], "出す順 = 宣言順")
        XCTAssertTrue(CoachMark.fabLongPress < CoachMark.fabDrag)
        XCTAssertTrue(CoachMark.fabDrag < CoachMark.switcherReorder)
    }

    func testOnlyEligibleMarksAreOffered() {
        XCTAssertEqual(next(eligible: [.switcherReorder]), .switcherReorder, "その場面で使える物だけ")
        XCTAssertNil(next(eligible: []), "使える物が無ければ出さない")
    }

    // MARK: - 済んだ物は飛ばす

    func testSkipsDoneMarks() {
        XCTAssertEqual(next(done: [.fabLongPress]), .fabDrag)
        XCTAssertEqual(next(done: [.fabLongPress, .fabDrag]), .switcherReorder)
    }

    func testAllDoneShowsNothing() {
        XCTAssertNil(next(done: all), "全部済んだら出さない")
        XCTAssertNil(next(done: all, replaying: true), "やり直しの間でも済んだ物は出さない")
        XCTAssertNil(next(done: all, lastShown: .init(mark: .fabLongPress, at: day1), now: day2),
                     "済んだ物は出し直しの対象にもしない")
    }

    // MARK: - やめた(optedOut)

    func testOptedOutShowsNothing() {
        XCTAssertNil(next(optedOut: true), "「もう出さない」なら何も出さない")
        XCTAssertNil(next(optedOut: true, replaying: true, lastShown: .init(mark: .fabDrag, at: day1)),
                     "やり直し中でも・出したままでも、やめたが最優先")
    }

    // MARK: - 前回から短い間は出さない(1 日 1 つ)

    func testSameDayBlocksANewMark() {
        let shown = CoachMarkSchedule.LastShown(mark: .fabLongPress, at: day1)
        XCTAssertNil(next(done: [.fabLongPress], lastShown: shown, now: day1Later),
                     "同じ日は次の物を出さない")
        XCTAssertEqual(next(done: [.fabLongPress], lastShown: shown, now: day2), .fabDrag,
                       "日が変われば次の物")
    }

    func testUnclosedMarkKeepsBeingShownEvenSameDay() {
        let shown = CoachMarkSchedule.LastShown(mark: .fabDrag, at: day1)
        XCTAssertEqual(next(lastShown: shown, now: day1Later), .fabDrag,
                       "閉じていない物は、同じ日でも続けて出す(一度に 1 つ)")
        XCTAssertNil(next(eligible: [.switcherReorder], lastShown: shown, now: day1Later),
                     "出したままの物が使えない場面では、同じ日に別の物を出さない")
        XCTAssertEqual(next(eligible: [.switcherReorder], lastShown: shown, now: day2), .switcherReorder)
    }

    // MARK: - やり直し(replaying)は間隔を空けない

    func testReplayingIgnoresTheOncePerDayGap() {
        let shown = CoachMarkSchedule.LastShown(mark: .fabLongPress, at: day1)
        XCTAssertEqual(next(done: [.fabLongPress], replaying: true, lastShown: shown, now: day1Later), .fabDrag,
                       "「もう一度見る」の間は同じ日でも続けて出す")
    }

    func testLastShownNilShowsFirstRegardlessOfDay() {
        XCTAssertEqual(next(now: day2), .fabLongPress, "まだ一度も出していなければ、いつでも最初の物")
    }

    // MARK: - LastShown の値

    func testLastShownEquatable() {
        XCTAssertEqual(CoachMarkSchedule.LastShown(mark: .fabDrag, at: day1),
                       CoachMarkSchedule.LastShown(mark: .fabDrag, at: day1))
        XCTAssertNotEqual(CoachMarkSchedule.LastShown(mark: .fabDrag, at: day1),
                          CoachMarkSchedule.LastShown(mark: .fabDrag, at: day2))
        XCTAssertNotEqual(CoachMarkSchedule.LastShown(mark: .fabDrag, at: day1),
                          CoachMarkSchedule.LastShown(mark: .fabLongPress, at: day1))
    }
}

/// #2242 `CoachMarkStore` が UserDefaults に何を書くか(保存先を差し替えて鍵ごと確かめる)。
@MainActor
final class MacCoachMarkStorePersistenceTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""
    private let all = Set(CoachMark.allCases)
    private var calendar: Calendar {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }
    private let day1 = Date(timeIntervalSince1970: 1_800_000_000)
    private var day2: Date { day1.addingTimeInterval(86_400) }

    override func setUp() {
        suite = "macCoachMarkSchedule2242-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    private func makeStore() -> CoachMarkStore { CoachMarkStore(defaults: defaults, calendar: calendar) }

    func testMarkShownWritesMarkAndTime() {
        let store = makeStore()
        store.markShown(.fabDrag, now: day1)
        XCTAssertEqual(defaults.string(forKey: CoachMarkStore.lastShownKey), CoachMark.fabDrag.rawValue)
        XCTAssertEqual(defaults.object(forKey: CoachMarkStore.lastShownAtKey) as? Date, day1)
        XCTAssertEqual(makeStore().lastShown, .init(mark: .fabDrag, at: day1), "作り直しても読み戻せる")
    }

    func testMarkShownAgainKeepsTheFirstTime() {
        let store = makeStore()
        store.markShown(.fabDrag, now: day1)
        store.markShown(.fabDrag, now: day2)
        XCTAssertEqual(defaults.object(forKey: CoachMarkStore.lastShownAtKey) as? Date, day1,
                       "同じ物を出し直しても 1 日 1 つの数え方を崩さない")
        store.markShown(.switcherReorder, now: day2)
        XCTAssertEqual(defaults.object(forKey: CoachMarkStore.lastShownAtKey) as? Date, day2, "別の物なら進む")
    }

    func testDismissAndMarkPerformedWriteDonePerMark() {
        let store = makeStore()
        store.dismiss(.fabLongPress)
        store.markPerformed(.switcherReorder)
        XCTAssertTrue(defaults.bool(forKey: CoachMarkStore.doneKey(.fabLongPress)))
        XCTAssertTrue(defaults.bool(forKey: CoachMarkStore.doneKey(.switcherReorder)))
        XCTAssertFalse(defaults.bool(forKey: CoachMarkStore.doneKey(.fabDrag)), "触っていない物は済みにしない")
        XCTAssertEqual(makeStore().done, [.fabLongPress, .switcherReorder])
        XCTAssertEqual(CoachMarkStore.doneKey(.fabDrag), CoachMarkStore.doneKeyPrefix + "fabDrag")
    }

    func testOptOutIsSaved() {
        makeStore().optOut()
        XCTAssertTrue(defaults.bool(forKey: CoachMarkStore.optOutKey))
        XCTAssertTrue(makeStore().optedOut)
    }

    func testReplayClearsEverySavedKey() {
        let store = makeStore()
        store.markShown(.fabLongPress, now: day1)
        for mark in CoachMark.allCases { store.dismiss(mark) }
        store.optOut()

        store.replay()

        for mark in CoachMark.allCases {
            XCTAssertNil(defaults.object(forKey: CoachMarkStore.doneKey(mark)))
        }
        XCTAssertNil(defaults.object(forKey: CoachMarkStore.optOutKey))
        XCTAssertNil(defaults.object(forKey: CoachMarkStore.lastShownKey))
        XCTAssertNil(defaults.object(forKey: CoachMarkStore.lastShownAtKey))
        XCTAssertEqual(store.done, [])
        XCTAssertFalse(store.optedOut)
        XCTAssertNil(store.lastShown)
        XCTAssertTrue(store.replaying, "やり直しの印は保存しない(その場だけ)")
        XCTAssertFalse(makeStore().replaying)
    }

    func testReloadPicksUpExternalWipe() {
        let store = makeStore()
        store.dismiss(.fabLongPress)
        store.optOut()
        defaults.removePersistentDomain(forName: suite)
        store.reload()
        XCTAssertEqual(store.done, [])
        XCTAssertFalse(store.optedOut)
        XCTAssertNil(store.lastShown)
        XCTAssertEqual(store.next(eligible: all, now: day1), .fabLongPress)
    }

    func testBrokenSavedLastShownIsIgnored() {
        defaults.set("notAMark", forKey: CoachMarkStore.lastShownKey)
        defaults.set(day1, forKey: CoachMarkStore.lastShownAtKey)
        XCTAssertNil(makeStore().lastShown, "知らない名前は無視する")
        defaults.set(CoachMark.fabDrag.rawValue, forKey: CoachMarkStore.lastShownKey)
        defaults.removeObject(forKey: CoachMarkStore.lastShownAtKey)
        XCTAssertNil(makeStore().lastShown, "時刻が無ければ無視する")
    }
}
