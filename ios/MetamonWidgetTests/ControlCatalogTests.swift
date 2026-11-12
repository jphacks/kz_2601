import XCTest

/// コントロール(#1993)の設定で選べる一覧の受け渡し(本体 → App Group → ウィジェット)。
final class ControlCatalogTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite: String!

    override func setUp() {
        suite = "control-catalog-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
    }

    private let sample = ControlCatalog(
        apps: [.init(id: "a1", name: "家計簿"), .init(id: "a2", name: "Training Log")],
        recordTargets: [
            .init(appId: "a1", entityId: "expense", appName: "家計簿", entityName: "支出", showsEntity: true),
            .init(appId: "a2", entityId: "set", appName: "Training Log", entityName: "セット", showsEntity: false),
        ]
    )

    func testEmptyWhenNothingSaved() {
        XCTAssertEqual(ControlCatalog.load(from: defaults), ControlCatalog())
    }

    func testRoundTripAndSkipUnchanged() {
        XCTAssertTrue(sample.save(to: defaults))
        XCTAssertEqual(ControlCatalog.load(from: defaults), sample)
        XCTAssertFalse(sample.save(to: defaults)) // 同じなら書かない(コントロールを読み直さない)
        var next = sample
        next.apps.removeFirst() // アプリを消した
        XCTAssertTrue(next.save(to: defaults))
        XCTAssertNil(ControlCatalog.load(from: defaults).app(id: "a1"))
    }

    /// id と見出しは本体の RecordTargetEntity と同じ決まり
    func testRecordTargetIdAndTitle() {
        XCTAssertEqual(sample.recordTargets[0].id, "a1/expense")
        XCTAssertEqual(sample.recordTargets[0].title, "家計簿 › 支出")
        XCTAssertEqual(sample.recordTargets[1].title, "Training Log")
        XCTAssertEqual(sample.recordTarget(id: "a2/set")?.entityId, "set")
    }

    func testSearch() {
        XCTAssertTrue(ControlCatalog.matches("Training Log", "training"))
        XCTAssertTrue(ControlCatalog.matches("家計簿", " 家計 "))
        XCTAssertTrue(ControlCatalog.matches("家計簿", ""))
        XCTAssertFalse(ControlCatalog.matches("家計簿", "日記"))
    }
}
