import XCTest

/// #274・H1: 初回は 同意 → チュートリアル(1 枚)→ 初期画面(カテゴリ別の例)→ 最初のアプリの直後に「こう話すと直せます」。
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
        XCTAssertFalse(app.buttons["tutorial.start"].exists, "同意より先にチュートリアルが出た")
        agree.tap()

        // 2. チュートリアル 1 枚(H1)→ はじめる
        let start = app.buttons["tutorial.start"]
        XCTAssertTrue(start.waitForExistence(timeout: 10), "同意のあとにチュートリアルが出ない")
        XCTAssertTrue(app.staticTexts["話すと、アプリができる"].exists)
        XCTAssertFalse(app.buttons["tutorial.next"].exists, "チュートリアルは 1 枚")
        attach("tutorial-1-create")
        start.tap()
        XCTAssertTrue(waitGone(start), "「はじめる」で閉じない")
        XCTAssertFalse(app.alerts.firstMatch.waitForExistence(timeout: 2), "音声モデルの確認を自動で出さない(M14)")

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
