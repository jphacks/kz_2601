import XCTest

/// #415 頼み方のコツ: 音声画面のリンク / 設定から開き、例をタップすると入力欄に入る(送らない)。
final class PromptTipsUITests: XCTestCase {
    private var app: XCUIApplication!
    private let example = "下に「履歴」のタブを足して、記録を新しい順に全部見られるようにして"

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    private var field: XCUIElement { app.descendants(matching: .any)["composer.field"].firstMatch }

    func testTipFromVoiceScreenGoesIntoInput() {
        let link = app.buttons["voice.tips"]
        XCTAssertTrue(link.waitForExistence(timeout: 15), "音声画面に「頼み方のコツ」が無い")
        attach("tips-0-entry")
        link.tap()

        XCTAssertTrue(app.descendants(matching: .any)["tips.category.tabs"].waitForExistence(timeout: 5), "コツの画面が開かない")
        attach("tips-1-sheet")
        app.buttons["tips.example.tab_add"].tap()

        XCTAssertTrue(waitGone(app.buttons["tips.example.tab_add"]), "例をタップしてもシートが閉じない")
        XCTAssertTrue(waitValue(field, contains: example), "入力欄に例が入らない: \(String(describing: field.value))")
        XCTAssertFalse(app.descendants(matching: .any)["chat.restart"].exists, "送信してはいけない")
        attach("tips-2-inserted")
    }

    func testTipFromSettingsGoesIntoInput() {
        let gear = app.buttons["voice.settings"]
        XCTAssertTrue(gear.waitForExistence(timeout: 15))
        gear.tap()

        let row = app.buttons["settings.tips"]
        XCTAssertTrue(app.navigationBars.firstMatch.waitForExistence(timeout: 5))
        for _ in 0..<8 where !(row.exists && row.isHittable) {
            app.swipeUp()
        }
        XCTAssertTrue(row.isHittable, "設定に「頼み方のコツ」が無い")
        row.tap()

        let tip = app.buttons["tips.example.tab_rename"]
        XCTAssertTrue(tip.waitForExistence(timeout: 5), "設定からコツの画面が開かない")
        attach("tips-settings")
        tip.tap()

        XCTAssertTrue(waitGone(tip), "例をタップしても設定が閉じない")
        XCTAssertTrue(waitValue(field, contains: "「集計」のタブの名前を「グラフ」に変えて"),
                      "入力欄に例が入らない: \(String(describing: field.value))")
    }

    private func waitValue(_ element: XCUIElement, contains text: String, timeout: TimeInterval = 5) -> Bool {
        let exp = XCTNSPredicateExpectation(predicate: NSPredicate(format: "value CONTAINS %@", text), object: element)
        return XCTWaiter().wait(for: [exp], timeout: timeout) == .completed
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
