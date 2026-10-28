import XCTest

/// #416: 右上の「＋」と右下の追加ボタンが同じフォームを開く DSL(モックの「予約」= 保存済みの DSL 相当)でも、
/// 追加の操作は右下の 1 つだけ出る。
final class AddButtonDedupUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    func testOnlyOneAddControl() {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted", "METAMON_CLARIFY": "0"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()

        let field = app.descendants(matching: .any)["composer.field"].firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 15), "初期画面の文字入力が無い")
        field.tap()
        field.typeText("美容室の予約アプリを作って")
        app.buttons["送信"].firstMatch.tap()

        let alert = app.alerts["個人の情報も公開されます"]
        XCTAssertTrue(alert.waitForExistence(timeout: 30))
        alert.buttons["公開のままにする"].tap()

        let add = app.buttons["予約を追加"].firstMatch
        XCTAssertTrue(add.waitForExistence(timeout: 15), "右下の追加ボタンが無い")
        let banner = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "ほかの人に見られてよいか")).firstMatch
        if banner.exists { banner.tap() } // 上の要約カードがナビバーに被らないように
        _ = XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == false"), object: banner)], timeout: 10)
        shot("416-home")

        XCTAssertFalse(app.navigationBars.buttons["追加"].exists, "右上の＋が右下の追加ボタンと重なって出ている")
        add.tap()
        XCTAssertTrue(app.staticTexts["お名前"].firstMatch.waitForExistence(timeout: 10), "追加ボタンでフォームが開かない")
    }

    private func shot(_ name: String) {
        let s = app.screenshot()
        let a = XCTAttachment(screenshot: s)
        a.name = name
        a.lifetime = .keepAlways
        add(a)
        if let dir = ProcessInfo.processInfo.environment["SHOT_DIR"], !dir.isEmpty {
            try? s.pngRepresentation.write(to: URL(fileURLWithPath: dir).appending(path: "\(name).png"))
        }
    }
}
