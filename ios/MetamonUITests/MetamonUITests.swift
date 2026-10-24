import XCTest

/// 指の操作(タップ・ドラッグ・長押し)の UI テスト(#165)。simctl では再現できない操作を XCUITest で実際に行う。
/// すべてモック(`METAMON_MOCK=1`)で起動し、DEBUG フックで初期アプリ・FAB 位置を固定する。
/// 手元での実行手順は docs/ios/architecture.md「UI テスト」。
final class MetamonUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    // MARK: - 起動

    /// `METAMON_FAB_POSITION=auto` で前のテストが保存した FAB の位置(@AppStorage)を毎回初期値に戻す。
    @discardableResult
    private func launch(example: String?, _ extra: [String: String] = [:]) -> XCUIApplication {
        let app = XCUIApplication()
        var env = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto"]
        if let example { env["METAMON_INITIAL_EXAMPLE"] = example }
        env.merge(extra) { _, new in new }
        app.launchEnvironment = env
        // 文言で探す要素があるので日本語に固定(シミュレータの既定は英語)。アプリ内の言語設定も上書き
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()
        self.app = app
        return app
    }

    private var fab: XCUIElement { app.buttons["voice.fab"] }
    private var fabHandle: XCUIElement { app.descendants(matching: .any)["voice.fabHandle"] }
    /// 音声シート(生成後の FAB から開く)だけにある設定ボタン
    private var voiceSheet: XCUIElement { app.buttons["voice.settings"] }
    private var switcher: XCUIElement { app.buttons["switcher.close"] }

    private func waitForFab(file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertTrue(fab.waitForExistence(timeout: 10), "マイク FAB が出ない", file: file, line: line)
        // 起動直後の FAB_POSITION 適用・入場アニメを待つ
        Thread.sleep(forTimeInterval: 0.8)
    }

    private func element(labelBeginsWith prefix: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH %@", prefix)).firstMatch
    }

    private func waitFor(_ element: XCUIElement, _ predicate: String, timeout: TimeInterval) -> Bool {
        let exp = XCTNSPredicateExpectation(predicate: NSPredicate(format: predicate), object: element)
        return XCTWaiter().wait(for: [exp], timeout: timeout) == .completed
    }

    private func attachScreenshot(_ name: String) {
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }

    // MARK: - マイク FAB(#110)

    func testFabTapOpensVoiceSheet() {
        launch(example: "kakeibo")
        waitForFab()
        fab.tap()
        XCTAssertTrue(voiceSheet.waitForExistence(timeout: 5), "タップで音声シートが開かない")
        XCTAssertFalse(switcher.exists, "タップで変身一覧まで開いた")
        attachScreenshot("fab-tap-voice-sheet")
    }

    func testFabLongPressOpensOnlySwitcher() {
        launch(example: "kakeibo")
        waitForFab()
        fab.press(forDuration: 1.0)
        XCTAssertTrue(switcher.waitForExistence(timeout: 5), "長押しで変身一覧が開かない")
        // 長押しを離した瞬間の Button タップで音声シートが開いてしまわないこと
        Thread.sleep(forTimeInterval: 1.0)
        XCTAssertFalse(voiceSheet.exists, "長押しで音声シートも開いた")
        attachScreenshot("fab-longpress-switcher")
    }

    func testFabDragRightSnapsToTrailingWithoutOpeningSheet() {
        launch(example: "kakeibo")
        waitForFab()
        let window = app.windows.firstMatch.frame
        let start = fab.frame
        XCTAssertLessThan(start.midX, window.midX, "初期位置が左下でない")

        let from = fab.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
        // 画面の 6 割の位置までゆっくり運んで止めてから離す(弾かない = 隠れない)
        let to = app.windows.firstMatch.coordinate(withNormalizedOffset: CGVector(dx: 0.62, dy: start.midY / window.height))
        from.press(forDuration: 0.1, thenDragTo: to, withVelocity: XCUIGestureVelocity(400), thenHoldForDuration: 0.4)

        Thread.sleep(forTimeInterval: 1.2) // 吸着アニメ + 離した瞬間のタップ抑止(0.3s)を待つ
        XCTAssertTrue(fab.exists, "ドラッグ後に FAB が消えた(隠れた?)")
        XCTAssertGreaterThan(fab.frame.midX, window.midX, "右に吸着していない")
        XCTAssertGreaterThan(fab.frame.maxX, window.width - 40, "右端に寄っていない")
        XCTAssertFalse(voiceSheet.exists, "ドラッグを離した後に音声シートが開いた")
        XCTAssertFalse(switcher.exists, "ドラッグで変身一覧が開いた")
        attachScreenshot("fab-drag-right")
    }

    func testFabFlickOffEdgeHidesAndHandleRestores() {
        launch(example: "kakeibo")
        waitForFab()
        let window = app.windows.firstMatch.frame
        let start = fab.frame
        let from = fab.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
        // 右端の外へ速く弾く
        let to = app.windows.firstMatch.coordinate(withNormalizedOffset: CGVector(dx: 0.99, dy: start.midY / window.height))
        from.press(forDuration: 0.1, thenDragTo: to, withVelocity: XCUIGestureVelocity(4000), thenHoldForDuration: 0)

        XCTAssertTrue(fabHandle.waitForExistence(timeout: 3), "端の外へ弾いても取っ手が出ない")
        XCTAssertFalse(fab.isHittable, "隠れた後も FAB が押せる")
        XCTAssertGreaterThan(fabHandle.frame.midX, window.midX, "取っ手が右端に無い")
        XCTAssertFalse(voiceSheet.exists, "弾いた後に音声シートが開いた")
        attachScreenshot("fab-hidden-handle")

        fabHandle.tap()
        XCTAssertTrue(waitFor(fabHandle, "exists == false", timeout: 3), "取っ手のタップで FAB が戻らない(取っ手が残っている)")
        Thread.sleep(forTimeInterval: 1.0) // 戻りのバネのアニメを待つ
        // accessibilityHidden の解除がアクセシビリティツリーに反映されるまで少し遅れることがある
        XCTAssertTrue(waitFor(fab, "exists == true", timeout: 5), "取っ手のタップで FAB が戻らない")
        XCTAssertGreaterThan(fab.frame.minX, window.midX, "FAB が右端の画面内に戻っていない")
        XCTAssertLessThan(fab.frame.maxX, window.width, "FAB が画面外に残っている")
        XCTAssertFalse(voiceSheet.exists, "取っ手のタップで音声シートまで開いた")
        // 戻った FAB を実際に押せること(isHittable は SwiftUI の .position 配置で偽陰性が出るので使わない)
        fab.tap()
        XCTAssertTrue(voiceSheet.waitForExistence(timeout: 5), "戻った FAB を押しても音声シートが開かない")
    }

    // MARK: - 対話のチップ(#105)

    func testClarifyChipAdvancesChat() {
        // 短い 1 発目 → モック AI が 1 問だけ聞く。AUTO_PAUSE を長くして自動返答させず、チップを指で押す
        launch(example: nil, ["METAMON_CLARIFY": "1", "METAMON_AUTO_TRANSCRIPT": "日記", "METAMON_AUTO_PAUSE": "600"])
        let chip = app.buttons.matching(identifier: "chat.chip").firstMatch
        XCTAssertTrue(chip.waitForExistence(timeout: 15), "質問の選択肢チップが出ない")
        attachScreenshot("chat-question-chips")
        let label = chip.label
        chip.tap()
        XCTAssertTrue(element(labelBeginsWith: "あなた: \(label)").waitForExistence(timeout: 5), "チップの答えが吹き出しに出ない")
        XCTAssertTrue(app.buttons["chat.build"].waitForExistence(timeout: 10), "チップで次(この内容で作る)に進まない")
        attachScreenshot("chat-ready")
    }

    // MARK: - 一覧のチェック丸(#127)

    func testListCheckCircleMarksDone() {
        launch(example: "shopping")
        let row = element(labelBeginsWith: "牛乳")
        XCTAssertTrue(row.waitForExistence(timeout: 10), "買い物リストの行が出ない")
        XCTAssertEqual(row.value as? String, "未チェック")
        // 丸は行頭(行の左端から 16pt + 半径 14pt)。行の中央を押すとフォームが開くので丸だけを狙う
        row.coordinate(withNormalizedOffset: CGVector(dx: 0, dy: 0.5)).withOffset(CGVector(dx: 30, dy: 0)).tap()

        let checked = app.descendants(matching: .any)
            .matching(NSPredicate(format: "label BEGINSWITH %@ AND value == %@", "牛乳", "チェック済み")).firstMatch
        XCTAssertTrue(checked.waitForExistence(timeout: 5), "丸のタップでチェック済みにならない")
        XCTAssertFalse(app.buttons["保存"].exists, "丸のタップでフォームが開いた")
        attachScreenshot("list-checked")
    }

    // MARK: - 変身一覧 → シェア(#118)

    func testSwitcherCardShareOpensShareSheet() {
        launch(example: "kakeibo", ["METAMON_MOCK_SHARING": "1"])
        waitForFab()
        fab.press(forDuration: 1.0)
        XCTAssertTrue(switcher.waitForExistence(timeout: 5), "長押しで変身一覧が開かない")
        // カード右上の「…」メニュー(#144/#145)→「シェア・公開」
        let menu = app.buttons.matching(identifier: "switcher.cardMenu").firstMatch
        XCTAssertTrue(menu.waitForExistence(timeout: 5), "カードに「…」メニューが無い")
        menu.tap()
        let share = app.buttons.matching(NSPredicate(format: "identifier == 'switcher.share' OR label == 'シェア・公開'")).firstMatch
        XCTAssertTrue(share.waitForExistence(timeout: 5), "メニューに「シェア・公開」が無い")
        share.tap()
        XCTAssertTrue(app.buttons["share.close"].waitForExistence(timeout: 5), "シェアシートが開かない")
        XCTAssertTrue(app.descendants(matching: .any)["シェア用 QR コード"].waitForExistence(timeout: 5), "シェアリンク(QR)が出ない")
        attachScreenshot("switcher-share")
    }

    // MARK: - フォーム(家計簿の支出追加)

    func testKakeiboAddExpenseAppearsInList() {
        launch(example: "kakeibo")
        let add = app.buttons["支出を追加"]
        XCTAssertTrue(add.waitForExistence(timeout: 10), "「支出を追加」が無い")
        add.tap()

        let amount = app.textFields.matching(NSPredicate(format: "placeholderValue == '0'")).firstMatch
        XCTAssertTrue(amount.waitForExistence(timeout: 5), "金額の入力欄が無い")
        amount.tap()
        amount.typeText("1234")
        let memo = app.textFields.matching(NSPredicate(format: "placeholderValue == 'コンビニ'")).firstMatch
        XCTAssertTrue(memo.exists, "メモの入力欄が無い")
        memo.tap()
        memo.typeText("UIテスト")
        attachScreenshot("form-filled")

        app.buttons["保存"].firstMatch.tap()
        let row = element(labelBeginsWith: "UIテスト")
        XCTAssertTrue(row.waitForExistence(timeout: 8), "保存した支出が一覧に出ない")
        XCTAssertTrue(row.label.contains("1,234"), "一覧の金額が入力と違う: \(row.label)")
        attachScreenshot("form-saved-list")
    }
}
