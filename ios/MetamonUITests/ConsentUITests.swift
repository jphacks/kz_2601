import XCTest

/// #246: 初回起動の「AI へのデータ送信」の同意 → 同意前は生成できない → 同意すると生成できる。
/// `METAMON_AI_CONSENT=reset` で保存済みの同意を消して初回の状態で起動する(DEBUG フック)。
final class ConsentUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "reset", "METAMON_CLARIFY": "0"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    func testFirstLaunchConsentGatesGeneration() {
        let agree = app.buttons["consent.agree"]
        let later = app.buttons["consent.later"]
        XCTAssertTrue(agree.waitForExistence(timeout: 15), "初回起動で同意の画面が出ない")
        attach("consent-first-launch")

        // 「あとで」: 閲覧はできるが、生成しようとすると同意の画面がもう一度出る(AI には送らない)
        later.tap()
        XCTAssertTrue(waitGone(agree), "「あとで」で閉じない")
        let field = app.descendants(matching: .any)["composer.field"].firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 15), "初期画面の文字入力が無い")
        field.tap()
        field.typeText("家計簿アプリを作って")
        app.buttons["送信"].firstMatch.tap()
        XCTAssertTrue(agree.waitForExistence(timeout: 10), "同意前に生成しようとしても同意の画面が出ない")
        XCTAssertFalse(app.buttons["voice.fab"].exists, "同意前に生成された")

        // 同意 → 生成できる(生成したアプリのマイク FAB に切り替わる)
        agree.tap()
        XCTAssertTrue(waitGone(agree), "同意しても閉じない")
        XCTAssertTrue(field.waitForExistence(timeout: 10))
        field.tap()
        field.typeText("家計簿アプリを作って")
        app.buttons["送信"].firstMatch.tap()
        XCTAssertTrue(app.buttons["voice.fab"].waitForExistence(timeout: 30), "同意後に生成できない")
        attach("consent-after-generate")
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
