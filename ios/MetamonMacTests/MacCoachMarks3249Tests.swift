import XCTest
@testable import Metamon

/// Mac の吹き出し(#3249)。iOS と同じ決まり・同じ保存先で、並べ替えの吹き出しを 1 回だけ出す。
@MainActor
final class MacCoachMarks3249Tests: XCTestCase {
    private var suite: String!
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        suite = "MacCoachMarks3249Tests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        super.tearDown()
    }

    private func eligible(open: Bool = true, count: Int = 2, searching: Bool = false, tutorial: Bool = true, busy: Bool = false) -> Set<CoachMark> {
        MacCoachMarks.eligible(hasOpenApp: open, reorderableCount: count, isSearching: searching, tutorialSeen: tutorial, busy: busy)
    }

    func testOnlyReorderOnMac() {
        XCTAssertEqual(eligible(), [.switcherReorder])
        XCTAssertTrue(MacCoachMarks.marks.isDisjoint(with: CoachMark.fabMarks), "Mac にマイクのボタンは無い")
    }

    func testNotEligibleWhenSceneIsWrong() {
        XCTAssertEqual(eligible(open: false), [], "中央にアプリが無い")
        XCTAssertEqual(eligible(count: 1), [], "並べ替えられない")
        XCTAssertEqual(eligible(searching: true), [], "検索中は並べ替えられない")
        XCTAssertEqual(eligible(tutorial: false), [], "チュートリアルの前")
        XCTAssertEqual(eligible(busy: true), [], "シート・お知らせの間")
    }

    func testShownOnceThenNeverAgain() {
        let store = CoachMarkStore(defaults: defaults)
        XCTAssertEqual(store.next(eligible: eligible()), .switcherReorder)
        store.markShown(.switcherReorder)
        XCTAssertEqual(CoachMarkStore(defaults: defaults).next(eligible: eligible()), .switcherReorder, "閉じずに終えたら次も同じもの")
        store.dismiss(.switcherReorder)
        XCTAssertNil(store.next(eligible: eligible()))
        XCTAssertNil(CoachMarkStore(defaults: defaults).next(eligible: eligible()), "起動し直しても出さない")
    }

    func testDraggingBeforeTheBubbleCountsAsDone() {
        let store = CoachMarkStore(defaults: defaults)
        store.markPerformed(.switcherReorder)
        XCTAssertNil(store.next(eligible: eligible()))
    }

    func testOptOutStopsIt() {
        let store = CoachMarkStore(defaults: defaults)
        store.optOut()
        XCTAssertNil(CoachMarkStore(defaults: defaults).next(eligible: eligible()))
    }

    func testSharesTheIOSStorageKeys() {
        // iOS と同じ保存のキー(`CoachMarkStore` をそのまま使う)
        let store = CoachMarkStore(defaults: defaults)
        store.dismiss(.switcherReorder)
        XCTAssertTrue(defaults.bool(forKey: CoachMarkStore.doneKey(.switcherReorder)))
    }

    func testCopyIsNotEmpty() {
        XCTAssertFalse(MacCoachMarks.title(.switcherReorder).isEmpty)
        XCTAssertFalse(MacCoachMarks.body(.switcherReorder).isEmpty)
    }
}
