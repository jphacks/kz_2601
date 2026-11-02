import XCTest

/// #313: β の版(`METAMON_BETA=1` で強制・DEBUG フック)は、チュートリアルのあとに β の案内が 1 回出る。
final class BetaWelcomeUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted",
                                 "METAMON_TUTORIAL": "seen", "METAMON_BETA": "1", "METAMON_BETA_WELCOME": "reset",
                                 "METAMON_CLARIFY": "0"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    func testBetaWelcomeShowsOnceWithFeedbackEntry() {
        let close = app.buttons["betaWelcome.close"]
        XCTAssertTrue(close.waitForExistence(timeout: 15), "β の版の初回に案内が出ない")
        XCTAssertTrue(app.staticTexts["Tailor It β へようこそ"].exists)
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = "beta-welcome"
        shot.lifetime = .keepAlways
        add(shot)
        let feedback = app.buttons["betaWelcome.feedback"]
        for _ in 0..<3 where !feedback.exists { app.swipeUp() } // List は画面外の行を作らない
        XCTAssertTrue(feedback.waitForExistence(timeout: 5), "フィードバックへの入口が無い")
        close.tap()
        let gone = XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == false"), object: close)
        XCTAssertEqual(XCTWaiter().wait(for: [gone], timeout: 10), .completed, "「はじめる」で閉じない")

        // 2 回目の起動では出ない(見た扱いを保存している)
        app.terminate()
        app.launchEnvironment["METAMON_BETA_WELCOME"] = nil
        app.launch()
        XCTAssertTrue(app.buttons["voice.fab"].waitForExistence(timeout: 15) || app.descendants(matching: .any)["composer.field"].firstMatch.waitForExistence(timeout: 5))
        XCTAssertFalse(app.buttons["betaWelcome.close"].waitForExistence(timeout: 4), "2 回目の起動でも出た")
    }
}
