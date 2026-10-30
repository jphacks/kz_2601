import XCTest

/// #254: 共有シートの「参加している人」→ 参加者が自分で抜ける。
/// モック(`METAMON_MOCK_SHARING=1` + `METAMON_MOCK_MEMBERS=member`)で、参加者 3 人の一覧を端末内で出す。
final class SharedMembersUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchEnvironment = [
            "METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted",
            "METAMON_INITIAL_EXAMPLE": "kakeibo", "METAMON_MOCK_SHARING": "1", "METAMON_MOCK_MEMBERS": "member",
        ]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    func testMemberSeesListAndLeaves() {
        let fab = app.buttons["voice.fab"]
        XCTAssertTrue(fab.waitForExistence(timeout: 20), "マイク FAB が出ない")
        fab.press(forDuration: 1.0)
        XCTAssertTrue(app.buttons["switcher.close"].waitForExistence(timeout: 15), "長押しで変身一覧が開かない")
        let menu = app.buttons["家計簿のメニュー"]
        XCTAssertTrue(menu.waitForExistence(timeout: 15), "家計簿カードのメニューが無い")
        menu.tap()
        let share = app.buttons.matching(NSPredicate(format: "identifier == 'switcher.share' OR label == 'シェア・公開'")).firstMatch
        XCTAssertTrue(share.waitForExistence(timeout: 15), "メニューに「シェア・公開」が無い")
        share.tap()
        XCTAssertTrue(app.buttons["share.close"].waitForExistence(timeout: 15), "シェアシートが開かない")

        // 参加している人(作成者・父(あなた)・たろう)
        let rows = app.staticTexts.matching(identifier: "members.row")
        let leave = app.buttons["members.leave"]
        scrollUntilHittable(leave)
        XCTAssertTrue(waitUntil(timeout: 15) { rows.count == 3 }, "参加者は 3 人(\(rows.count) 人)")
        XCTAssertTrue(app.staticTexts["父"].exists && app.staticTexts["たろう"].exists && app.staticTexts["作成者"].exists)
        XCTAssertFalse(app.buttons["members.remove"].exists, "参加者には「外す」を出さない")
        attach("members-1-list")

        // 抜ける → 確認 → 「抜けました」
        leave.tap()
        let alert = app.alerts.firstMatch
        XCTAssertTrue(alert.waitForExistence(timeout: 10), "抜ける確認が出ない")
        attach("members-2-confirm")
        alert.buttons.matching(NSPredicate(format: "label == %@", "抜ける")).firstMatch.tap()
        XCTAssertTrue(app.descendants(matching: .any)["members.left"].waitForExistence(timeout: 10), "抜けた表示にならない")
        XCTAssertFalse(leave.exists, "抜けた後は「抜ける」を出さない")
        attach("members-3-left")
    }

    private func scrollUntilHittable(_ element: XCUIElement) {
        for _ in 0..<6 where !(element.exists && element.isHittable) {
            app.swipeUp()
        }
        XCTAssertTrue(element.waitForExistence(timeout: 10), "\(element) が無い")
    }

    private func waitUntil(timeout: TimeInterval, _ condition: @escaping () -> Bool) -> Bool {
        let exp = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in condition() }, object: nil)
        return XCTWaiter().wait(for: [exp], timeout: timeout) == .completed
    }

    private func attach(_ name: String) {
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }
}
