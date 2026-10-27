import XCTest

/// #277 変身一覧: カードの「…」から名前を変える → 複製 → 左へ移動(声なしで整理できる)。
final class AppListManageUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
    }

    private func openSwitcher() {
        let app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted",
                                 "METAMON_INITIAL_EXAMPLE": "kakeibo", "METAMON_MOCK_SHARING": "1"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()
        self.app = app
        let fab = app.buttons["voice.fab"]
        XCTAssertTrue(fab.waitForExistence(timeout: 15), "マイク FAB が出ない")
        Thread.sleep(forTimeInterval: 1.0) // 入場アニメ
        fab.press(forDuration: 1.0)
        XCTAssertTrue(app.buttons["switcher.close"].waitForExistence(timeout: 15), "長押しで変身一覧が開かない")
    }

    private func menuItem(_ id: String, _ label: String) -> XCUIElement {
        app.buttons.matching(NSPredicate(format: "identifier == %@ OR label == %@", id, label)).firstMatch
    }

    private func shot(_ name: String) {
        let a = XCTAttachment(screenshot: app.screenshot())
        a.name = name
        a.lifetime = .keepAlways
        add(a)
    }

    private func cardMenus() -> [String] {
        app.buttons.matching(identifier: "switcher.cardMenu").allElementsBoundByIndex.map(\.label)
    }

    func testRenameDuplicateAndMove() {
        openSwitcher()
        let kakeibo = app.buttons["家計簿のメニュー"]
        XCTAssertTrue(kakeibo.waitForExistence(timeout: 15), "家計簿カードのメニューが無い")

        // 名前を変える
        kakeibo.tap()
        let rename = menuItem("switcher.rename", "名前を変える")
        XCTAssertTrue(rename.waitForExistence(timeout: 10), "メニューに「名前を変える」が無い")
        shot("applist-1-menu")
        rename.tap()
        let field = app.alerts.textFields.firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 10), "名前の入力欄が出ない")
        field.tap()
        field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 6)) // 入っている「家計簿」を消す
        field.typeText("仕事の家計簿")
        shot("applist-2-rename")
        app.alerts.buttons["変更"].firstMatch.tap()
        let renamed = app.buttons["仕事の家計簿のメニュー"]
        XCTAssertTrue(renamed.waitForExistence(timeout: 10), "名前が変わらない: \(cardMenus())")

        // 複製(記録は空・切り替えない)
        renamed.tap()
        let duplicate = menuItem("switcher.duplicate", "複製")
        XCTAssertTrue(duplicate.waitForExistence(timeout: 10), "メニューに「複製」が無い")
        duplicate.tap()
        let copy = app.buttons["仕事の家計簿のコピーのメニュー"]
        XCTAssertTrue(copy.waitForExistence(timeout: 10), "複製が一覧に出ない: \(cardMenus())")
        shot("applist-3-duplicated")

        // 右へ移動: 先頭のカードを 1 つ右へ(先頭は画面内にある)
        let before = cardMenus()
        XCTAssertGreaterThanOrEqual(before.count, 3)
        let first = before[0]
        app.buttons[first].tap()
        let right = menuItem("switcher.moveRight", "右へ移動")
        XCTAssertTrue(right.waitForExistence(timeout: 10), "メニューに「右へ移動」が無い")
        right.tap()
        XCTAssertTrue(waitUntil(timeout: 10) { [self] in
            let now = cardMenus()
            return now.count == before.count && now[1] == first && now[0] == before[1]
        }, "並びが変わらない: \(before) → \(cardMenus())")
        shot("applist-4-moved")
    }

    private func waitUntil(timeout: TimeInterval, _ cond: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(timeout)
        while Date() < end {
            if cond() { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        }
        return cond()
    }
}
