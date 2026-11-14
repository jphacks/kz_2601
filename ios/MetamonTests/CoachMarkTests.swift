import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #1441 気づきにくい操作の吹き出し: 1 回だけ・閉じた / 操作したら出さない・「もう出さない」・1 日 1 つ・もう一度見る。
@MainActor
final class CoachMarkTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""
    private let all = Set(CoachMark.allCases)
    private var calendar: Calendar {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }
    private let day1 = Date(timeIntervalSince1970: 1_800_000_000)
    private var day1Later: Date { day1.addingTimeInterval(60) }
    private var day2: Date { day1.addingTimeInterval(86_400) }
    private var day3: Date { day1.addingTimeInterval(86_400 * 2) }

    override func setUp() {
        suite = "coachMarks1441-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    private func makeStore() -> CoachMarkStore { CoachMarkStore(defaults: defaults, calendar: calendar) }

    // MARK: - 順番・1 回だけ

    func testShowsInOrderOnlyWhereUsable() {
        let store = makeStore()
        XCTAssertEqual(store.next(eligible: all, now: day1), .fabLongPress, "大事なもの(長押しで変身一覧)から")
        XCTAssertEqual(store.next(eligible: [.switcherReorder], now: day1), .switcherReorder, "その場面で使えるものだけ")
        XCTAssertNil(store.next(eligible: [], now: day1))
    }

    func testShownMarkStaysUntilClosedButOnlyOnePerDay() {
        let store = makeStore()
        store.markShown(.fabLongPress, now: day1)
        XCTAssertEqual(store.next(eligible: all, now: day1Later), .fabLongPress, "閉じるまで同じものを出し続ける(一度に 1 つ)")
        store.dismiss(.fabLongPress)
        XCTAssertNil(store.next(eligible: all, now: day1Later), "初日に全部を出さない(1 日 1 つ)")
        XCTAssertEqual(store.next(eligible: all, now: day2), .fabDrag, "次の日に次のもの")
    }

    func testDismissedNeverShownAgain() {
        let store = makeStore()
        store.markShown(.fabLongPress, now: day1)
        store.dismiss(.fabLongPress)
        XCTAssertTrue(store.isDone(.fabLongPress))
        XCTAssertNil(store.next(eligible: [.fabLongPress], now: day3))
        XCTAssertNil(makeStore().next(eligible: [.fabLongPress], now: day3), "ヒントごとに保存される")
    }

    func testPerformedBeforeShownIsSkipped() {
        let store = makeStore()
        store.markPerformed(.fabLongPress)
        XCTAssertEqual(store.next(eligible: CoachMark.fabMarks, now: day1), .fabDrag, "自分で長押しした人には長押しを教えない")
        store.markShown(.fabDrag, now: day1)
        store.markPerformed(.fabDrag)
        XCTAssertNil(store.next(eligible: CoachMark.fabMarks, now: day1Later), "出ている吹き出しも操作したら消える")
    }

    func testOptOutStopsEverything() {
        let store = makeStore()
        store.markShown(.fabLongPress, now: day1)
        store.optOut()
        XCTAssertNil(store.next(eligible: all, now: day3))
        XCTAssertNil(makeStore().next(eligible: all, now: day3), "「もう出さない」は保存される")
    }

    func testUnclosedMarkComesBackAfterRelaunch() {
        makeStore().markShown(.fabDrag, now: day1)
        let reloaded = makeStore()
        XCTAssertEqual(reloaded.next(eligible: all, now: day1Later), .fabDrag, "閉じずにアプリを閉じたら、次も同じものを出す")
        XCTAssertNil(reloaded.next(eligible: [.switcherReorder], now: day1Later), "同じ日は別の場面でも新しいものを出さない")
        XCTAssertEqual(reloaded.next(eligible: [.switcherReorder], now: day2), .switcherReorder)
    }

    func testMarkShownTwiceKeepsFirstTime() {
        let store = makeStore()
        store.markShown(.fabLongPress, now: day1)
        store.markShown(.fabLongPress, now: day2)
        XCTAssertEqual(store.lastShown?.at, day1, "出し直しで日付を進めない")
    }

    // MARK: - もう一度見る

    func testReplayResetsAndShowsBackToBack() {
        let store = makeStore()
        for mark in CoachMark.allCases { store.markPerformed(mark) }
        store.optOut()
        store.replay()
        XCTAssertFalse(store.optedOut)
        XCTAssertEqual(store.next(eligible: all, now: day1), .fabLongPress)
        store.markShown(.fabLongPress, now: day1)
        store.dismiss(.fabLongPress)
        XCTAssertEqual(store.next(eligible: all, now: day1Later), .fabDrag, "もう一度見る間は同じ日でも続けて出す")
        store.dismiss(.fabDrag)
        store.dismiss(.switcherReorder)
        XCTAssertFalse(store.replaying, "全部見たら普段の間隔に戻る")
    }

    // MARK: - 自動テストの起動

    func testLaunchOverride() {
        let store = makeStore()
        store.applyLaunchOverride(environment: ["METAMON_AI_CONSENT": "granted"])
        XCTAssertNil(store.next(eligible: all, now: day1), "既存の自動テストは吹き出しで画面を塞がない")
        store.applyLaunchOverride(environment: ["METAMON_COACH_MARKS": "reset"])
        XCTAssertEqual(store.next(eligible: all, now: day1), .fabLongPress)
        store.applyLaunchOverride(environment: ["METAMON_COACH_MARKS": "off"])
        XCTAssertNil(store.next(eligible: all, now: day1))
    }

    // MARK: - 最初の案内・お知らせの後

    private func makeRuntime() throws -> AppRuntime {
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0),
                            services: BuildConfig.make(environment: .mock))
        rt.aiConsent = AIConsentStore(defaults: defaults)
        rt.tutorial = FirstRunTutorialStore(defaults: defaults)
        rt.betaWelcome = BetaWelcomeStore(defaults: defaults, isBeta: false)
        rt.coachMarks = makeStore()
        return rt
    }

    func testWaitsForTutorialFirstAppAndNotices() throws {
        let rt = try makeRuntime()
        XCTAssertNil(rt.fabCoachMark, "最初の案内(#274)の前は出さない")
        rt.tutorial?.markTutorialSeen()
        XCTAssertNil(rt.fabCoachMark, "最初のアプリの前は出さない")
        rt.tutorial?.markEditHintShown()
        rt.showFirstEditHint = true
        XCTAssertNil(rt.fabCoachMark, "完成の後のお知らせ(直し方の例)が閉じてから")
        rt.showFirstEditHint = false
        XCTAssertEqual(rt.fabCoachMark, .fabLongPress)
        rt.showVoiceSheet = true
        XCTAssertNil(rt.fabCoachMark, "シートの間は出さない")
        rt.showVoiceSheet = false
        rt.showAppSwitcher = true
        XCTAssertNil(rt.fabCoachMark, "変身一覧の下の FAB には出さない")
        XCTAssertEqual(rt.coachMark(eligible: [.switcherReorder]), .switcherReorder)
    }

    func testRuntimeEventsAndReplay() throws {
        let rt = try makeRuntime()
        rt.tutorial?.markTutorialSeen()
        rt.tutorial?.markEditHintShown()
        rt.handleCoachMark(.fabLongPress, .shown)
        rt.noteGesturePerformed(.fabLongPress)
        XCTAssertTrue(rt.coachMarks?.isDone(.fabLongPress) ?? false)
        rt.handleCoachMark(.fabDrag, .neverAgain)
        XCTAssertNil(rt.coachMark(eligible: Set(CoachMark.allCases)))
        rt.showAppSwitcher = true
        rt.replayCoachMarks()
        XCTAssertFalse(rt.showAppSwitcher, "もう一度見る時は変身一覧を閉じて FAB を見せる")
        XCTAssertEqual(rt.fabCoachMark, .fabLongPress)
    }
}
