import XCTest

/// #1140: Mac のキーボード操作(#934)をキーだけで通す。
/// ⌘1 / ⌘2 のフォーカス移動・Return で中央へ・Esc でシートを閉じる・⌘[ で戻る・⌘Z(削除が無い時は入力欄の取り消し)。
///
/// scheme では skipped(CI では回らない)。手元で回す:
///   cd ios && xcodebuild test -scheme MetamonMac -destination 'platform=macOS' -only-testing:MetamonMacUITests
/// 初回は「システム設定 > プライバシーとセキュリティ > アクセシビリティ / オートメーション」で Xcode(またはターミナル)の許可が要る。
final class KeyboardShellUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    func testKeyboardOnlyShell() {
        app = XCUIApplication()
        // モック・同意済み(案内とコーチマークも出ない)・β の案内は見た扱い
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_MOCK_STORE": "1", "METAMON_AI_CONSENT": "granted",
                                 "METAMON_BETA_WELCOME": "seen"]
        // 画面遷移の例(a → b → c)から始める
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja",
                               "-metamon.local.currentAppId", "navigation"]
        app.launch()

        let window = app.windows.firstMatch
        XCTAssertTrue(window.waitForExistence(timeout: 20), "ウインドウが出ない")
        XCTAssertTrue(app.descendants(matching: .any)["shell.sidebar"].firstMatch.waitForExistence(timeout: 10), "サイドバーが無い")
        XCTAssertTrue(waitTitle(of: window, timeout: 10) { !$0.isEmpty }, "アプリが開いていない")

        // ⌘1: サイドバーに入る → ↑↓ で選んだアプリが変わる
        app.typeKey("1", modifierFlags: .command)
        let start = window.title
        var forward = XCUIKeyboardKey.downArrow, backward = XCUIKeyboardKey.upArrow
        app.typeKey(forward, modifierFlags: [])
        if !waitTitle(of: window, timeout: 3, { $0 != start }) {
            swap(&forward, &backward) // 一覧の端だった
            app.typeKey(forward, modifierFlags: [])
        }
        XCTAssertTrue(waitTitle(of: window, timeout: 5) { $0 != start }, "⌘1 の後の矢印で選択が動かない")

        // Return: 中央へ移る → 矢印ではもう選択が動かない
        app.typeKey(.return, modifierFlags: [])
        assertTitleStays(window, after: forward, "Return の後も矢印がサイドバーに届いている")

        // ⌘1 → ⌘2: 中央へ移る
        app.typeKey("1", modifierFlags: .command)
        app.typeKey("2", modifierFlags: .command)
        assertTitleStays(window, after: forward, "⌘2 の後も矢印がサイドバーに届いている")

        // ⌘1 + 逆の矢印で画面遷移の例に戻し、⌘2 で中央へ
        app.typeKey("1", modifierFlags: .command)
        app.typeKey(backward, modifierFlags: [])
        XCTAssertTrue(waitTitle(of: window, timeout: 5) { $0 == start }, "画面遷移の例に戻れない")
        app.typeKey("2", modifierFlags: .command)

        // Esc: ⌘K の会話のシートを閉じる
        app.typeKey("k", modifierFlags: .command)
        XCTAssertTrue(app.sheets.firstMatch.waitForExistence(timeout: 10), "⌘K で会話のシートが開かない")
        app.typeKey(.escape, modifierFlags: [])
        XCTAssertTrue(waitGone(app.sheets.firstMatch, timeout: 5), "Esc でシートが閉じない")

        // ⌘[: 画面マップ(⇧⌘E)を矢印と Return で動かして B へ進み、⌘[ で戻る
        let back = app.menuBarItems["移動"].menuItems["戻る"]
        XCTAssertFalse(back.isEnabled, "最初の画面で「戻る」が押せる")
        app.typeKey("e", modifierFlags: [.command, .shift])
        XCTAssertTrue(app.sheets.firstMatch.waitForExistence(timeout: 10), "⇧⌘E で画面マップが開かない")
        app.typeKey(.downArrow, modifierFlags: [])
        app.typeKey(.return, modifierFlags: [])
        XCTAssertTrue(waitGone(app.sheets.firstMatch, timeout: 5), "画面マップの Return で画面が開かない")
        XCTAssertTrue(waitEnabled(back, true, timeout: 5), "次の画面へ進んだのに「戻る」が押せない")
        app.typeKey("[", modifierFlags: .command)
        XCTAssertTrue(waitEnabled(back, false, timeout: 5), "⌘[ で最初の画面に戻らない")

        // ⌘Z: 記録を消していない時は入力欄の取り消し(「元に戻す」)。押しても落ちない
        XCTAssertTrue(app.menuBarItems["編集"].menuItems["元に戻す"].exists, "編集メニューに「元に戻す」が無い")
        app.typeKey("z", modifierFlags: .command)
        XCTAssertEqual(app.state, .runningForeground)
        XCTAssertEqual(window.title, start)
    }

    // MARK: - 待ち合わせ

    private func waitTitle(of window: XCUIElement, timeout: TimeInterval, _ check: @escaping (String) -> Bool) -> Bool {
        let predicate = NSPredicate { object, _ in check((object as? XCUIElement)?.title ?? "") }
        return XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: predicate, object: window)], timeout: timeout) == .completed
    }

    private func waitGone(_ element: XCUIElement, timeout: TimeInterval) -> Bool {
        let expectation = XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == false"), object: element)
        return XCTWaiter().wait(for: [expectation], timeout: timeout) == .completed
    }

    private func waitEnabled(_ element: XCUIElement, _ enabled: Bool, timeout: TimeInterval) -> Bool {
        let expectation = XCTNSPredicateExpectation(predicate: NSPredicate(format: "isEnabled == %@", NSNumber(value: enabled)), object: element)
        return XCTWaiter().wait(for: [expectation], timeout: timeout) == .completed
    }

    /// キーを押しても選んだアプリ(ウインドウのタイトル)が変わらない
    private func assertTitleStays(_ window: XCUIElement, after key: XCUIKeyboardKey, _ message: String,
                                  file: StaticString = #filePath, line: UInt = #line) {
        let before = window.title
        app.typeKey(key, modifierFlags: [])
        XCTAssertFalse(waitTitle(of: window, timeout: 1.5) { $0 != before }, message, file: file, line: line)
    }
}
