import XCTest
import MetamonServices
@testable import Metamon

/// 集中モードのフィルタ(#1991): 保存・読み込み・切れた時に消す・一覧の絞り込み。
final class FocusFilterStateTests: XCTestCase {
    private var suite: String!
    private var defaults: UserDefaults!

    override func setUp() {
        suite = "FocusFilterStateTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
    }

    private func app(_ id: String) -> AppSummary {
        AppSummary(id: id, name: id, currentVersion: 1)
    }

    func testSaveAndLoad() {
        let state = FocusFilterState(appIds: ["a", "c"], silencesNotifications: true)
        FocusFilterState.apply(state, to: defaults)
        XCTAssertEqual(FocusFilterState.load(from: defaults), state)
    }

    func testNotSetLoadsNil() {
        XCTAssertNil(FocusFilterState.load(from: defaults))
    }

    func testFocusEndedClears() {
        FocusFilterState.apply(FocusFilterState(appIds: ["a"], silencesNotifications: false), to: defaults)
        // 集中モードが切れると既定値(空)で perform される
        FocusFilterState.apply(FocusFilterState(appIds: [], silencesNotifications: false), to: defaults)
        XCTAssertNil(FocusFilterState.load(from: defaults))

        FocusFilterState.apply(FocusFilterState(appIds: ["a"], silencesNotifications: false), to: defaults)
        FocusFilterState.apply(nil, to: defaults)
        XCTAssertNil(FocusFilterState.load(from: defaults))
    }

    func testSilenceOnlyIsKept() {
        let state = FocusFilterState(appIds: [], silencesNotifications: true)
        FocusFilterState.apply(state, to: defaults)
        XCTAssertEqual(FocusFilterState.load(from: defaults), state)
    }

    func testBrokenDataLoadsNil() {
        defaults.set(Data("x".utf8), forKey: FocusFilterState.storageKey)
        XCTAssertNil(FocusFilterState.load(from: defaults))
    }

    func testFilterKeepsPickedInListOrder() {
        let apps = ["a", "b", "c"].map(app)
        let state = FocusFilterState(appIds: ["c", "a"], silencesNotifications: false)
        XCTAssertEqual(state.filter(apps).map(\.id), ["a", "c"])
    }

    func testFilterWithoutPickedAppsShowsAll() {
        let apps = ["a", "b"].map(app)
        XCTAssertEqual(FocusFilterState(appIds: [], silencesNotifications: true).filter(apps).map(\.id), ["a", "b"])
        // 選んだアプリが消えていたら空にせず全部出す
        XCTAssertEqual(FocusFilterState(appIds: ["gone"], silencesNotifications: false).filter(apps).map(\.id), ["a", "b"])
    }

    func testIntentStateFromParameters() {
        var intent = TailorFocusFilter()
        intent.apps = [TailorAppEntity(app("a"))]
        intent.silencesNotifications = true
        XCTAssertEqual(intent.state, FocusFilterState(appIds: ["a"], silencesNotifications: true))
    }

    // MARK: - 通知とウィジェット(#1992)

    func testNotificationAllowedWithoutSilence() {
        let state = FocusFilterState(appIds: ["a"], silencesNotifications: false)
        XCTAssertTrue(state.allowsNotification(appId: "a"))
        XCTAssertTrue(state.allowsNotification(appId: "b"), "通知を止めない時は絞らない")
        XCTAssertNil(state.notificationFilterPredicate)
    }

    func testSilenceKeepsOnlyPickedApps() {
        let state = FocusFilterState(appIds: ["a", "c"], silencesNotifications: true)
        XCTAssertTrue(state.allowsNotification(appId: "a"))
        XCTAssertTrue(state.allowsNotification(appId: "c"))
        XCTAssertFalse(state.allowsNotification(appId: "b"))
    }

    func testSilenceWithoutPickedAppsStopsAll() {
        let state = FocusFilterState(appIds: [], silencesNotifications: true)
        XCTAssertFalse(state.allowsNotification(appId: "a"))
    }

    /// OS に渡す絞り込みは `allowsNotification` と同じ答え(通知の filterCriteria = 生成アプリの id)。
    func testPredicateMatchesDecision() throws {
        for state in [FocusFilterState(appIds: ["a", "c.d"], silencesNotifications: true),
                      FocusFilterState(appIds: [], silencesNotifications: true)] {
            let predicate = try XCTUnwrap(state.notificationFilterPredicate)
            for id in ["a", "b", "c.d", "c", ""] {
                XCTAssertEqual(predicate.evaluate(with: id), state.allowsNotification(appId: id), "\(state.appIds) \(id)")
            }
        }
    }

    func testIntentHandsPredicateToOS() {
        var intent = TailorFocusFilter()
        intent.apps = [TailorAppEntity(app("a"))]
        intent.silencesNotifications = true
        XCTAssertEqual(intent.appContext.notificationFilterPredicate?.evaluate(with: "a"), true)
        XCTAssertEqual(intent.appContext.notificationFilterPredicate?.evaluate(with: "b"), false)
        intent.silencesNotifications = false
        XCTAssertNil(intent.appContext.notificationFilterPredicate)
    }

    func testWidgetShowsOnlyPickedApps() {
        let narrowed = FocusFilterState(appIds: ["a"], silencesNotifications: false)
        XCTAssertTrue(narrowed.allowsWidget(appId: "a"))
        XCTAssertFalse(narrowed.allowsWidget(appId: "b"))
        XCTAssertFalse(narrowed.allowsWidget(appId: nil), "どのアプリか分からない古いデータは出さない")
        // 一覧を絞らない(通知だけ止める)時はウィジェットはそのまま
        XCTAssertTrue(FocusFilterState(appIds: [], silencesNotifications: true).allowsWidget(appId: "b"))
    }

    func testWidgetPauseUnchangedWithoutFilterOrSnapshot() {
        var snapshot = WidgetSnapshot.sample
        snapshot.appId = "b"
        let narrowed = FocusFilterState(appIds: ["a"], silencesNotifications: false)
        XCTAssertFalse(FocusFilterState.pausesWidget(snapshot, focus: nil), "フィルタが無い人は今まで通り")
        XCTAssertFalse(FocusFilterState.pausesWidget(nil, focus: narrowed), "まだアプリが無い時の案内はそのまま")
        XCTAssertTrue(FocusFilterState.pausesWidget(snapshot, focus: narrowed))
        snapshot.appId = "a"
        XCTAssertFalse(FocusFilterState.pausesWidget(snapshot, focus: narrowed))
    }

    func testCopyHasEnglish() throws {
        let path = try XCTUnwrap(Bundle(for: AppRuntime.self).path(forResource: "en", ofType: "lproj"))
        let en = try XCTUnwrap(Bundle(path: path))
        for key in [TailorFocusFilter.title.key, "出すアプリ", "通知を止める", "絞らない", "%@だけ", "すべて表示",
                    "集中モード中: 選んだアプリだけを表示しています", "集中モード中: すべて表示しています"] {
            let value = en.localizedString(forKey: key, value: "__missing__", table: nil)
            XCTAssertNotEqual(value, "__missing__", key)
            XCTAssertNotEqual(value, key, key)
        }
    }
}
