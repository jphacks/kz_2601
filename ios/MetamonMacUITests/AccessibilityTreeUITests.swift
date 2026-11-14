import XCTest

/// #1205: Mac の VoiceOver に出る名前・値・見出し(#937)を XCUIElement で確かめる。
/// 単体テストで NSHostingView を辿ると SwiftUI の AX の木が作られない(根の AXGroup だけ)ので、アプリを起動して外から読む。
/// - carousel: 「前へ」「次へ」と「ページ」(値「1 / 4」)。次へで「2 / 4」になる
/// - disclosure: 行のボタンの値が「閉じています」→ 押すと「開いています」
/// - card / section の見出しが見出し(AXHeading)として出る
///
/// scheme では skipped(CI では回らない)。手元で回す方法は KeyboardShellUITests.swift の頭と同じ(署名とアクセシビリティの許可が要る)。
final class AccessibilityTreeUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    /// 部品ギャラリー(構造)の carousel と disclosure、card の見出し
    func testCarouselAndDisclosureAccessibility() {
        launch(appId: "gallery-structure")

        // carousel(4 枚・タイトル無し)
        XCTAssertTrue(element("カルーセル").waitForExistence(timeout: 10), "carousel の名前が無い")
        let back = app.buttons.matching(NSPredicate(format: "label == %@", "前へ")).firstMatch
        let forward = app.buttons.matching(NSPredicate(format: "label == %@", "次へ")).firstMatch
        let pageDots = element("ページ")
        XCTAssertTrue(back.exists, "「前へ」が無い")
        XCTAssertTrue(forward.exists, "「次へ」が無い")
        XCTAssertTrue(pageDots.exists, "「ページ」が無い")
        XCTAssertFalse(back.isEnabled, "1 枚目で「前へ」が押せる")
        XCTAssertTrue(forward.isEnabled, "1 枚目で「次へ」が押せない")
        XCTAssertTrue(waitValue(pageDots, "1 / 4"), "ページの値が 1 / 4 でない: \(pageDots.value ?? "nil")")

        forward.click()
        XCTAssertTrue(waitValue(pageDots, "2 / 4"), "次へでページの値が 2 / 4 にならない: \(pageDots.value ?? "nil")")
        XCTAssertTrue(back.isEnabled, "2 枚目で「前へ」が押せない")

        // disclosure(閉じた状態から)
        let row = app.buttons.matching(NSPredicate(format: "label == %@", "disclosure(たたむ)")).firstMatch
        XCTAssertTrue(row.exists, "disclosure の行がボタンとして出ていない")
        XCTAssertTrue(waitValue(row, "閉じています"), "閉じた disclosure の値: \(row.value ?? "nil")")
        row.click()
        XCTAssertTrue(waitValue(row, "開いています"), "開いた disclosure の値: \(row.value ?? "nil")")
        XCTAssertTrue(element("開くと詳細が出る。注意書きや補足に").waitForExistence(timeout: 5), "開いても中身が出ない")

        // card の見出し
        assertHeading("card")
    }

    /// BMI の例の section の見出し(「履歴」)
    func testSectionHeading() {
        launch(appId: "bmi")
        assertHeading("履歴")
    }

    // MARK: - 補助

    private func launch(appId: String) {
        app = XCUIApplication()
        // KeyboardShellUITests と同じモック・同意済みの起動。選んだ同梱の例を開いた状態から始める
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_MOCK_STORE": "1", "METAMON_AI_CONSENT": "granted",
                                 "METAMON_BETA_WELCOME": "seen"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja",
                               "-metamon.local.currentAppId", appId]
        app.launch()
        XCTAssertTrue(app.windows.firstMatch.waitForExistence(timeout: 20), "ウインドウが出ない")
    }

    private func element(_ label: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", label)).firstMatch
    }

    private func waitValue(_ element: XCUIElement, _ value: String, timeout: TimeInterval = 5) -> Bool {
        let predicate = NSPredicate { object, _ in ((object as? XCUIElement)?.value as? String) == value }
        return XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: predicate, object: element)], timeout: timeout) == .completed
    }

    /// XCUIElement は役割(AXRole)を API で出さないので、要素の説明(debugDescription)に AXHeading があるかで見る
    private func assertHeading(_ label: String, file: StaticString = #filePath, line: UInt = #line) {
        let heading = element(label)
        XCTAssertTrue(heading.waitForExistence(timeout: 10), "見出し「\(label)」が無い", file: file, line: line)
        XCTAssertTrue(heading.debugDescription.localizedCaseInsensitiveContains("heading"),
                      "「\(label)」が見出しとして出ていない:\n\(heading.debugDescription)", file: file, line: line)
    }
}
