import XCTest

/// #2001: 外の API の 402 / 409 の案内。設定の開発用の呼び出し口から、偽の provider `fakeOauth` の op を
/// `METAMON_MOCK_EXT_ERROR`(モックの `setNextError`)で断らせて、シートが出ることを確かめる。
final class ExtDenialUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    private func launch(error code: String) {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted",
                                 "METAMON_MOCK_EXT_ERROR": code]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()
    }

    private func callFakeProvider() {
        let gear = app.buttons["voice.settings"]
        XCTAssertTrue(gear.waitForExistence(timeout: 15))
        gear.tap()
        XCTAssertTrue(app.navigationBars.firstMatch.waitForExistence(timeout: 5))
        let button = app.buttons["settings.connections.debugCall"]
        for _ in 0..<8 where !(button.exists && button.isHittable) {
            app.swipeUp()
        }
        XCTAssertTrue(button.isHittable, "設定に開発用の呼び出し口が無い")
        button.tap()
    }

    /// 409 `ext_not_connected` → つなぐ案内 →「つなぐ」で今のつなぐ流れ(Pro でなければ課金画面)
    func testNotConnectedShowsConnectSheet() {
        launch(error: "ext_not_connected")
        callFakeProvider()
        let title = app.staticTexts["ext.connect.title"]
        XCTAssertTrue(title.waitForExistence(timeout: 5), "つなぐ案内が出ない")
        XCTAssertTrue(title.label.contains("試験用のサービス"), "provider の表示名が出ない: \(title.label)")
        attach("ext-not-connected")
        app.buttons["ext.connect.button"].tap()
        let paywall = app.descendants(matching: .any)["paywall.headline"].firstMatch
        let connected = waitGone(title, timeout: 5)
        XCTAssertTrue(connected || paywall.waitForExistence(timeout: 5), "「つなぐ」で今のつなぐ流れに入らない")
        attach("ext-connect-tapped")
    }

    /// 402 `pro_required` → 課金画面
    func testProRequiredShowsPaywall() {
        launch(error: "pro_required")
        callFakeProvider()
        XCTAssertTrue(app.descendants(matching: .any)["paywall.headline"].firstMatch.waitForExistence(timeout: 5), "課金画面が出ない")
        attach("ext-pro-required")
    }

    private func waitGone(_ element: XCUIElement, timeout: TimeInterval) -> Bool {
        let exp = XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == false"), object: element)
        return XCTWaiter().wait(for: [exp], timeout: timeout) == .completed
    }

    private func attach(_ name: String) {
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }
}
