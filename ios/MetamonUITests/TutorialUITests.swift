import XCTest

/// #274: 初回は 同意 → チュートリアル(3 枚)→ 初期画面(カテゴリ別の例)→ 最初のアプリの直後に「こう話すと直せます」。
/// `METAMON_AI_CONSENT=reset` / `METAMON_TUTORIAL=reset` で初回の状態で起動する(DEBUG フック)。
final class TutorialUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "reset",
                                 "METAMON_TUTORIAL": "reset", "METAMON_CLARIFY": "0"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    func testFirstRunConsentThenTutorialThenEditHint() {
        // 1. 同意が先
        let agree = app.buttons["consent.agree"]
        XCTAssertTrue(agree.waitForExistence(timeout: 15), "初回起動で同意の画面が出ない")
        XCTAssertFalse(app.buttons["tutorial.next"].exists, "同意より先にチュートリアルが出た")
        agree.tap()

        // 2. チュートリアル 3 枚 → はじめる
        let next = app.buttons["tutorial.next"]
        XCTAssertTrue(next.waitForExistence(timeout: 10), "同意のあとにチュートリアルが出ない")
        XCTAssertTrue(app.staticTexts["話すと、アプリができる"].exists)
        attach("tutorial-1-create")
        next.tap()
        XCTAssertTrue(app.staticTexts["話すと、直せる・戻せる"].waitForExistence(timeout: 5))
        attach("tutorial-2-edit")
        next.tap()
        let start = app.buttons["tutorial.start"]
        XCTAssertTrue(start.waitForExistence(timeout: 5), "3 枚目に「はじめる」が無い")
        attach("tutorial-3-share")
        start.tap()
        XCTAssertTrue(waitGone(start), "「はじめる」で閉じない")

        // 3. 初期画面: カテゴリ別の例(家族・友達と に共有系)
        let together = app.buttons["examples.category.together"]
        XCTAssertTrue(together.waitForExistence(timeout: 10), "例のカテゴリが無い")
        attach("home-examples-record")
        together.tap()
        XCTAssertTrue(app.buttons["友達と割り勘"].waitForExistence(timeout: 5), "家族・友達と の例が出ない")
        attach("home-examples-together")

        // 4. 最初のアプリを作る → 直し方の例が 1 回だけ出る
        let field = app.descendants(matching: .any)["composer.field"].firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 10))
        field.tap()
        field.typeText("家計簿アプリを作って")
        app.buttons["送信"].firstMatch.tap()
        XCTAssertTrue(app.buttons["voice.fab"].waitForExistence(timeout: 30), "アプリができない")
        let hint = app.buttons["editHint.close"]
        XCTAssertTrue(hint.waitForExistence(timeout: 15), "最初のアプリの直後に直し方の例が出ない")
        attach("first-edit-hint")
        hint.tap()
        XCTAssertTrue(waitGone(hint), "直し方の例が閉じない")
    }

    private func waitGone(_ element: XCUIElement, timeout: TimeInterval = 10) -> Bool {
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
