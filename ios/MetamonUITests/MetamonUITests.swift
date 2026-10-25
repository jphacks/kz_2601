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
        // METAMON_UI_PROBE: 生成中の表示が出ない時の切り分け用に、morphJob の状態を読める観測点を置く(#200)
        var env = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_UI_PROBE": "1", "METAMON_AI_CONSENT": "granted"]
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
        XCTAssertTrue(fab.waitForExistence(timeout: 15), "マイク FAB が出ない", file: file, line: line)
        // 起動直後の FAB_POSITION 適用・入場アニメが終わる(位置が 2 回続けて同じ)まで待つ
        var last = CGRect.null
        XCTAssertTrue(waitUntil(timeout: 15) { [fab] in
            let now = fab.frame
            defer { last = now }
            return now == last && !now.isEmpty
        }, "マイク FAB の位置が落ち着かない", file: file, line: line)
    }

    private func element(labelBeginsWith prefix: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH %@", prefix)).firstMatch
    }

    private func waitFor(_ element: XCUIElement, _ predicate: String, timeout: TimeInterval) -> Bool {
        let exp = XCTNSPredicateExpectation(predicate: NSPredicate(format: predicate), object: element)
        return XCTWaiter().wait(for: [exp], timeout: timeout) == .completed
    }

    /// 任意の条件(要素の frame など)が成り立つまで待つ。固定の sleep の代わり。
    private func waitUntil(timeout: TimeInterval, _ condition: @escaping () -> Bool) -> Bool {
        let exp = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in condition() }, object: nil)
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
        XCTAssertTrue(voiceSheet.waitForExistence(timeout: 15), "タップで音声シートが開かない")
        XCTAssertFalse(switcher.exists, "タップで変身一覧まで開いた")
        attachScreenshot("fab-tap-voice-sheet")
    }

    func testFabLongPressOpensOnlySwitcher() {
        launch(example: "kakeibo")
        waitForFab()
        fab.press(forDuration: 1.0)
        XCTAssertTrue(switcher.waitForExistence(timeout: 15), "長押しで変身一覧が開かない")
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

        // 画面の 6 割の位置まで運んで止めてから離す(止めてから離す = 弾かない = 隠れない)。
        // 押し始めの静止は短く(長押し 0.45s と取り違えないよう。遅い CI では指の移動の到着が遅れて長押しに化けた・#200)
        let dragRight = {
            let from = self.fab.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
            let to = self.app.windows.firstMatch.coordinate(withNormalizedOffset: CGVector(dx: 0.62, dy: start.midY / window.height))
            from.press(forDuration: 0.05, thenDragTo: to, withVelocity: XCUIGestureVelocity(1500), thenHoldForDuration: 0.4)
        }
        dragRight()
        if switcher.waitForExistence(timeout: 2) {
            // 遅い runner でイベントが詰まり長押し扱いになった時だけ 1 回やり直す(2 回続けば下の assert で落ちる)
            attachScreenshot("fab-drag-became-longpress")
            switcher.tap()
            XCTAssertTrue(waitFor(switcher, "exists == false", timeout: 15), "変身一覧が閉じない")
            dragRight()
        }

        // 吸着アニメ(バネ)が終わって右端に寄るまで待つ
        XCTAssertTrue(waitUntil(timeout: 15) { [fab] in fab.exists && fab.frame.maxX > window.width - 40 },
                      "右端に寄っていない(frame=\(fab.frame))")
        XCTAssertTrue(fab.exists, "ドラッグ後に FAB が消えた(隠れた?)")
        XCTAssertGreaterThan(fab.frame.midX, window.midX, "右に吸着していない")
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

        XCTAssertTrue(fabHandle.waitForExistence(timeout: 15), "端の外へ弾いても取っ手が出ない")
        XCTAssertFalse(fab.isHittable, "隠れた後も FAB が押せる")
        XCTAssertGreaterThan(fabHandle.frame.midX, window.midX, "取っ手が右端に無い")
        XCTAssertFalse(voiceSheet.exists, "弾いた後に音声シートが開いた")
        attachScreenshot("fab-hidden-handle")

        fabHandle.tap()
        XCTAssertTrue(waitFor(fabHandle, "exists == false", timeout: 15), "取っ手のタップで FAB が戻らない(取っ手が残っている)")
        // accessibilityHidden の解除がアクセシビリティツリーに反映されるまで少し遅れることがある。戻りのバネが終わるまで待つ
        XCTAssertTrue(waitFor(fab, "exists == true", timeout: 15), "取っ手のタップで FAB が戻らない")
        XCTAssertTrue(waitUntil(timeout: 15) { [fab] in fab.frame.minX > window.midX && fab.frame.maxX < window.width },
                      "FAB が右端の画面内に戻らない(frame=\(fab.frame))")
        XCTAssertGreaterThan(fab.frame.minX, window.midX, "FAB が右端の画面内に戻っていない")
        XCTAssertLessThan(fab.frame.maxX, window.width, "FAB が画面外に残っている")
        XCTAssertFalse(voiceSheet.exists, "取っ手のタップで音声シートまで開いた")
        // 戻った FAB を実際に押せること(isHittable は SwiftUI の .position 配置で偽陰性が出るので使わない)
        fab.tap()
        XCTAssertTrue(voiceSheet.waitForExistence(timeout: 15), "戻った FAB を押しても音声シートが開かない")
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
        XCTAssertTrue(element(labelBeginsWith: "あなた: \(label)").waitForExistence(timeout: 15), "チップの答えが吹き出しに出ない")
        XCTAssertTrue(app.buttons["chat.build"].waitForExistence(timeout: 15), "チップで次(この内容で作る)に進まない")
        attachScreenshot("chat-ready")
    }

    // MARK: - チャット基本化(#186)

    func testProposalChooseStartsGeneration() {
        // 作ったアプリ(家計簿)の音声シートで「見やすくして」→ モック AI が案を 2 つ出す。案の「これでいく」を指で押す
        launch(example: "kakeibo", ["METAMON_CLARIFY": "1", "METAMON_AUTO_TRANSCRIPT": "見やすくして", "METAMON_AUTO_PAUSE": "600"])
        let choose = app.buttons.matching(identifier: "chat.option.choose")
        XCTAssertTrue(choose.firstMatch.waitForExistence(timeout: 15), "案のカードの「これでいく」が出ない")
        // カードは順に差し込まれるので、遅い CI では 1 枚目だけの瞬間がある。2 枚そろうまで待つ
        XCTAssertTrue(waitUntil(timeout: 15) { choose.count == 2 }, "案のカードが 2 枚でない(\(choose.count) 枚)")
        XCTAssertTrue(voiceSheet.exists, "案を出しただけで音声シートが閉じた")
        attachScreenshot("chat-proposal-cards")

        choose.firstMatch.tap()
        XCTAssertTrue(waitFor(voiceSheet, "exists == false", timeout: 15), "「これでいく」で音声シートが閉じない(生成が始まらない)")
        let applied = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "反映しました")).firstMatch
        XCTAssertTrue(applied.waitForExistence(timeout: 15), "「これでいく」の案で改変されない")
        attachScreenshot("chat-proposal-applied")
    }

    // MARK: - 複数選択(#220)

    func testMultiSelectProposalAppliesTwoIdeas() {
        // 「機能を足したい」→ モック AI が組み合わせられる案を 3 つ(multiSelect)。2 つ選んで「これでいく(2 件)」
        launch(example: "kakeibo", ["METAMON_CLARIFY": "1", "METAMON_AUTO_TRANSCRIPT": "機能を足したい", "METAMON_AUTO_PAUSE": "600"])
        let toggles = app.buttons.matching(identifier: "chat.option.toggle")
        XCTAssertTrue(toggles.firstMatch.waitForExistence(timeout: 15), "複数選択の案のカードが出ない")
        XCTAssertTrue(waitUntil(timeout: 15) { toggles.count == 3 }, "案のカードが 3 枚でない(\(toggles.count) 枚)")
        XCTAssertFalse(app.buttons["chat.option.choose"].exists, "複数選択なのにカードごとの「これでいく」が出ている")
        let submit = app.buttons["chat.option.chooseSelected"]
        XCTAssertTrue(submit.exists, "「これでいく(N 件)」が無い")
        XCTAssertFalse(submit.isEnabled, "0 件で「これでいく」が押せる")
        attachScreenshot("multi220-proposal-none")

        toggles.element(boundBy: 0).tap()
        toggles.element(boundBy: 1).tap()
        XCTAssertTrue(waitUntil(timeout: 5) { submit.isEnabled && submit.label.contains("2 件") }, "2 件選んでも「これでいく(2 件)」にならない(\(submit.label))")
        XCTAssertEqual(toggles.element(boundBy: 0).value as? String, "選択中")
        XCTAssertEqual(toggles.element(boundBy: 2).value as? String, "未選択")
        XCTAssertTrue(voiceSheet.exists, "選んだだけで音声シートが閉じた")
        attachScreenshot("multi220-proposal-two")

        submit.tap()
        XCTAssertTrue(waitFor(voiceSheet, "exists == false", timeout: 15), "「これでいく(2 件)」で生成が始まらない")
        // モックは指示をそのまま要約に返す: 2 案の指示が 1 回の反映に入っている
        let applied = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@ AND label CONTAINS %@", "円グラフ", "予算の進捗バー")).firstMatch
        XCTAssertTrue(applied.waitForExistence(timeout: 15), "2 案が 1 回で反映されない")
        attachScreenshot("multi220-proposal-applied")
    }

    func testMultiSelectChipsSendJoinedAnswer() {
        launch(example: nil, ["METAMON_CLARIFY": "1", "METAMON_AUTO_TRANSCRIPT": "項目を選びたい", "METAMON_AUTO_PAUSE": "600"])
        let chips = app.buttons.matching(identifier: "chat.chip")
        XCTAssertTrue(chips.firstMatch.waitForExistence(timeout: 15), "質問のチップが出ない")
        let submit = app.buttons["chat.chip.submit"]
        XCTAssertTrue(submit.exists && !submit.isEnabled, "0 件で「決定」が押せる")
        chips.element(boundBy: 0).tap()
        XCTAssertTrue(element(labelBeginsWith: "AI: どの項目").exists, "1 つ押しただけで送られた")
        chips.element(boundBy: 2).tap()
        attachScreenshot("multi220-chips-two")
        submit.tap()
        XCTAssertTrue(element(labelBeginsWith: "あなた: 金額、メモ").waitForExistence(timeout: 15), "選んだ項目が「、」でつないで送られない")
        XCTAssertTrue(app.buttons["chat.build"].waitForExistence(timeout: 15), "決定で次(この内容で作る)に進まない")
    }

    // MARK: - チャット履歴(#210)

    func testChatHistoryRemainsAfterApply() {
        // 案の「これでいく」で反映 → シートを開き直すと、前の会話と「ここで反映しました(vN)・要約」の区切りが見える
        launch(example: "kakeibo", ["METAMON_CLARIFY": "1", "METAMON_AUTO_TRANSCRIPT": "見やすくして", "METAMON_AUTO_PAUSE": "600"])
        let choose = app.buttons.matching(identifier: "chat.option.choose")
        XCTAssertTrue(choose.firstMatch.waitForExistence(timeout: 15), "案のカードの「これでいく」が出ない")
        XCTAssertTrue(waitUntil(timeout: 15) { choose.count == 2 }, "案のカードが 2 枚でない")
        attachScreenshot("history-1-before-apply")
        choose.firstMatch.tap()
        XCTAssertTrue(waitFor(voiceSheet, "exists == false", timeout: 15), "「これでいく」で音声シートが閉じない")
        let applied = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "反映しました")).firstMatch
        XCTAssertTrue(applied.waitForExistence(timeout: 15), "改変されない")
        attachScreenshot("history-2-applied")

        waitForFab()
        fab.tap()
        XCTAssertTrue(voiceSheet.waitForExistence(timeout: 15), "音声シートが開き直せない")
        let divider = app.descendants(matching: .any)["chat.history.applied"]
        XCTAssertTrue(divider.waitForExistence(timeout: 15), "反映の区切りが見えない")
        XCTAssertTrue(divider.label.contains("ここで反映しました(v"), "区切りの文言: \(divider.label)")
        XCTAssertTrue(element(labelBeginsWith: "あなた: 見やすくして").exists, "前の会話が見えない")
        XCTAssertTrue(app.buttons["chat.restart"].exists, "「最初から」が無い")
        attachScreenshot("history-3-reopened")

        app.buttons["chat.restart"].tap()
        XCTAssertTrue(waitFor(divider, "exists == false", timeout: 15), "「最初から」で履歴が消えない")
        attachScreenshot("history-4-restarted")
    }

    // MARK: - 一覧のチェック丸(#127)

    func testListCheckCircleMarksDone() {
        launch(example: "shopping")
        let row = element(labelBeginsWith: "牛乳")
        XCTAssertTrue(row.waitForExistence(timeout: 15), "買い物リストの行が出ない")
        XCTAssertEqual(row.value as? String, "未チェック")
        // 丸は行頭(行の左端から 16pt + 半径 14pt)。行の中央を押すとフォームが開くので丸だけを狙う
        row.coordinate(withNormalizedOffset: CGVector(dx: 0, dy: 0.5)).withOffset(CGVector(dx: 30, dy: 0)).tap()

        let checked = app.descendants(matching: .any)
            .matching(NSPredicate(format: "label BEGINSWITH %@ AND value == %@", "牛乳", "チェック済み")).firstMatch
        XCTAssertTrue(checked.waitForExistence(timeout: 15), "丸のタップでチェック済みにならない")
        XCTAssertFalse(app.buttons["保存"].exists, "丸のタップでフォームが開いた")
        attachScreenshot("list-checked")
    }

    // MARK: - 変身一覧 → シェア(#118)

    func testSwitcherCardShareOpensShareSheet() {
        launch(example: "kakeibo", ["METAMON_MOCK_SHARING": "1"])
        waitForFab()
        fab.press(forDuration: 1.0)
        XCTAssertTrue(switcher.waitForExistence(timeout: 15), "長押しで変身一覧が開かない")
        // カード右上の「…」メニュー(#144/#145)→「シェア・公開」
        let menu = app.buttons.matching(identifier: "switcher.cardMenu").firstMatch
        XCTAssertTrue(menu.waitForExistence(timeout: 15), "カードに「…」メニューが無い")
        menu.tap()
        let share = app.buttons.matching(NSPredicate(format: "identifier == 'switcher.share' OR label == 'シェア・公開'")).firstMatch
        XCTAssertTrue(share.waitForExistence(timeout: 15), "メニューに「シェア・公開」が無い")
        share.tap()
        XCTAssertTrue(app.buttons["share.close"].waitForExistence(timeout: 15), "シェアシートが開かない")
        XCTAssertTrue(app.descendants(matching: .any)["シェア用 QR コード"].waitForExistence(timeout: 15), "シェアリンク(QR)が出ない")
        attachScreenshot("switcher-share")
    }

    // MARK: - 変身一覧 → 削除(#180)

    func testSwitcherCardDeleteRemovesCard() {
        launch(example: "kakeibo", ["METAMON_MOCK_SHARING": "1"])
        waitForFab()
        fab.press(forDuration: 1.0)
        XCTAssertTrue(switcher.waitForExistence(timeout: 15), "長押しで変身一覧が開かない")
        let menus = app.buttons.matching(identifier: "switcher.cardMenu")
        XCTAssertTrue(menus.firstMatch.waitForExistence(timeout: 15), "カードに「…」メニューが無い")
        XCTAssertTrue(waitUntil(timeout: 15) { menus.count == 2 }, "モックの一覧は 2 件(\(menus.count) 件)")
        let target = app.buttons["家計簿のメニュー"]
        XCTAssertTrue(target.waitForExistence(timeout: 15), "家計簿カードのメニューが無い")
        target.tap()
        let delete = app.buttons.matching(NSPredicate(format: "identifier == 'switcher.delete' OR label == '削除'")).firstMatch
        XCTAssertTrue(delete.waitForExistence(timeout: 15), "メニューに「削除」が無い")
        attachScreenshot("app-delete-1-menu")
        delete.tap()
        // 確認ダイアログ(破壊的操作なので赤の「削除」)
        let alert = app.alerts.firstMatch
        XCTAssertTrue(alert.waitForExistence(timeout: 15), "確認ダイアログが出ない")
        XCTAssertTrue(alert.label.contains("家計簿"), "ダイアログにアプリ名が無い: \(alert.label)")
        attachScreenshot("app-delete-2-confirm")
        alert.buttons.matching(NSPredicate(format: "label == %@", "削除")).firstMatch.tap()
        XCTAssertTrue(waitFor(target, "exists == false", timeout: 15), "削除したカードが一覧から消えない")
        XCTAssertTrue(waitUntil(timeout: 15) { menus.count == 1 }, "残りは 1 件(\(menus.count) 件)")
        XCTAssertTrue(app.buttons["買い物リストのメニュー"].exists, "他のアプリは残る")
        attachScreenshot("app-delete-3-after")
    }

    // MARK: - フォーム(家計簿の支出追加)

    func testKakeiboAddExpenseAppearsInList() {
        launch(example: "kakeibo")
        let add = app.buttons["支出を追加"]
        XCTAssertTrue(add.waitForExistence(timeout: 15), "「支出を追加」が無い")
        add.tap()

        let amount = app.textFields.matching(NSPredicate(format: "placeholderValue == '0'")).firstMatch
        XCTAssertTrue(amount.waitForExistence(timeout: 15), "金額の入力欄が無い")
        amount.tap()
        amount.typeText("1234")
        let memo = app.textFields.matching(NSPredicate(format: "placeholderValue == 'コンビニ'")).firstMatch
        XCTAssertTrue(memo.waitForExistence(timeout: 15), "メモの入力欄が無い")
        memo.tap()
        memo.typeText("UIテスト")
        attachScreenshot("form-filled")

        app.buttons["保存"].firstMatch.tap()
        let row = element(labelBeginsWith: "UIテスト")
        XCTAssertTrue(row.waitForExistence(timeout: 15), "保存した支出が一覧に出ない")
        XCTAssertTrue(row.label.contains("1,234"), "一覧の金額が入力と違う: \(row.label)")
        attachScreenshot("form-saved-list")
    }

    // MARK: - テキスト入力(#196)

    /// ソフトキーボードの上端(出ていなければ nil)。
    private var keyboardTop: CGFloat? {
        let kb = app.keyboards.firstMatch
        return kb.exists ? kb.frame.minY : nil
    }

    func testChatComposerReturnInsertsNewlineAndGrows() {
        launch(example: nil, ["METAMON_CLARIFY": "1", "METAMON_AUTO_TRANSCRIPT": "日記", "METAMON_AUTO_PAUSE": "600"])
        XCTAssertTrue(app.buttons.matching(identifier: "chat.chip").firstMatch.waitForExistence(timeout: 15), "対話が始まらない")
        let field = composerField
        XCTAssertTrue(field.waitForExistence(timeout: 15), "対話の入力欄が無い")
        field.tap()
        field.typeText("one")
        let oneLine = field.frame.height
        field.typeText("\ntwo\nthree")
        Thread.sleep(forTimeInterval: 0.5)
        let value = field.value as? String ?? ""
        XCTAssertTrue(value.contains("one\ntwo\nthree"), "return で改行が入らない(送信・確定になった?): \(value)")
        XCTAssertGreaterThan(field.frame.height, oneLine + 20, "複数行で入力欄が伸びない")
        XCTAssertFalse(element(labelBeginsWith: "あなた: one").exists, "return で送信された")
        if let top = keyboardTop {
            XCTAssertLessThanOrEqual(field.frame.maxY, top + 1, "キーボード表示中に入力欄が隠れている")
        }
        attachScreenshot("chat-composer-multiline")

        app.buttons["composer.send"].tap()
        XCTAssertTrue(element(labelBeginsWith: "あなた: one").waitForExistence(timeout: 15), "送信ボタンで送れない")
        XCTAssertFalse((field.value as? String ?? "").contains("three"), "送信後に入力欄が空にならない")
    }

    /// 返事待ちの間にキーボードを出して測るための、モックの対話の待ち(秒)。CI(macos-26)は XCUI の問い合わせが 1 回 0.3〜1 秒と遅く、
    /// 16 秒では送信 → キーボード表示 → 計測の前に返事が届いた(run 40158670179)。余裕を見て 40 秒
    private let kbReplyDelay = "40"

    /// 返事待ちの間にキーボードを出しても、返事が届いた後も、入力欄はキーボードのすぐ上にあってタップできる(#208)。
    /// edit = 作ったアプリの音声シート、create = 初期画面の対話。モックの対話の待ちを延ばして返事待ちの間に操作する。
    func testChatComposerStaysAboveKeyboardWhileWaitingAndAfterReply_edit() {
        launch(example: "kakeibo", ["METAMON_CLARIFY": "1", "METAMON_AUTO_TRANSCRIPT": "見やすくして", "METAMON_AUTO_PAUSE": "600",
                                    "METAMON_MOCK_AI_DELAY": kbReplyDelay])
        assertComposerAboveKeyboardAcrossReply("edit", replied: app.buttons.matching(identifier: "chat.option.choose").firstMatch,
                                                userBubble: "あなた: 見やすくして")
    }

    /// 実機の手順どおり: FAB → シートの文字入力で 1 つ目を打って送る(キーボードが閉じる)→ 返事待ちの間に対話の入力欄をタップ
    func testChatComposerStaysAboveKeyboardWhileWaitingAndAfterReply_editTyped() {
        launch(example: "kakeibo", ["METAMON_CLARIFY": "1", "METAMON_MOCK_AI_DELAY": kbReplyDelay])
        waitForFab()
        fab.tap()
        XCTAssertTrue(voiceSheet.waitForExistence(timeout: 15), "音声シートが開かない")
        let field = composerField
        XCTAssertTrue(field.waitForExistence(timeout: 15), "シートの文字入力が無い")
        field.tap()
        field.typeText("見やすくして")
        app.buttons["composer.send"].tap()
        assertComposerAboveKeyboardAcrossReply("editTyped", replied: app.buttons.matching(identifier: "chat.option.choose").firstMatch,
                                                userBubble: "あなた: 見やすくして")
    }

    /// 1 つ目を声で(シミュレータのマイクはダミーの定型文)
    func testChatComposerStaysAboveKeyboardWhileWaitingAndAfterReply_editVoice() {
        launch(example: "kakeibo", ["METAMON_CLARIFY": "1", "METAMON_MOCK_AI_DELAY": kbReplyDelay])
        waitForFab()
        fab.tap()
        XCTAssertTrue(voiceSheet.waitForExistence(timeout: 15), "音声シートが開かない")
        let mic = app.buttons["マイク。タップして話す"].firstMatch
        XCTAssertTrue(mic.waitForExistence(timeout: 30), "マイクが無い")
        mic.tap()
        let stop = app.buttons["録音中。タップで終了"].firstMatch
        XCTAssertTrue(stop.waitForExistence(timeout: 10), "録音が始まらない")
        Thread.sleep(forTimeInterval: 1.5)
        stop.tap()
        let replied = app.descendants(matching: .any).matching(NSPredicate(format: "identifier IN %@",
            ["chat.option.choose", "chat.chip", "chat.applyNow", "chat.build"])).firstMatch
        assertComposerAboveKeyboardAcrossReply("editVoice", replied: replied, userBubble: "あなた: ")
    }

    func testChatComposerStaysAboveKeyboardWhileWaitingAndAfterReply_createTyped() {
        launch(example: nil, ["METAMON_CLARIFY": "1", "METAMON_MOCK_AI_DELAY": kbReplyDelay])
        let field = composerField
        XCTAssertTrue(field.waitForExistence(timeout: 15), "初期画面の文字入力が無い")
        field.tap()
        field.typeText("日記")
        app.buttons["composer.send"].tap()
        assertComposerAboveKeyboardAcrossReply("createTyped", replied: app.buttons.matching(identifier: "chat.chip").firstMatch,
                                                userBubble: "あなた: 日記")
    }

    func testChatComposerStaysAboveKeyboardWhileWaitingAndAfterReply_create() {
        launch(example: nil, ["METAMON_CLARIFY": "1", "METAMON_AUTO_TRANSCRIPT": "日記", "METAMON_AUTO_PAUSE": "600",
                              "METAMON_MOCK_AI_DELAY": kbReplyDelay])
        assertComposerAboveKeyboardAcrossReply("create", replied: app.buttons.matching(identifier: "chat.chip").firstMatch,
                                                userBubble: "あなた: 日記")
    }

    private func assertComposerAboveKeyboardAcrossReply(_ name: String, replied: XCUIElement, userBubble: String,
                                                        file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertTrue(element(labelBeginsWith: userBubble).waitForExistence(timeout: 20), "1 つ目の発話が吹き出しに出ない", file: file, line: line)
        let field = composerField
        XCTAssertTrue(field.waitForExistence(timeout: 10), "対話の入力欄が無い", file: file, line: line)
        attachScreenshot("kb208-\(name)-sent")
        XCTAssertFalse(replied.exists, "返事待ちの前に返事が届いた(待ちが短い)", file: file, line: line)
        field.tap()
        let kb = app.keyboards.firstMatch
        XCTAssertTrue(kb.waitForExistence(timeout: 5), "ソフトウェアキーボードが出ない(ハードウェアキーボード接続を切る)", file: file, line: line)
        // キーボードと入力欄のアニメーションが落ち着くまで(frame が 2 回続けて同じ)
        func settle() {
            var last = CGRect.null
            _ = waitUntil(timeout: 5) {
                let now = field.frame
                defer { last = now }
                return now == last
            }
        }
        settle()
        // 入力欄の外枠の下端 = 送信ボタン(44pt 角)の下端。XCUI の keyboard.frame は候補バーを含まない iOS 26 の値なので、
        // 「キーボードのすぐ上」= 0〜80pt の範囲、「動かない」= 返事の前後で差 2pt 以内で見る
        let send = app.buttons["composer.send"]
        let gapWaiting = kb.frame.minY - send.frame.maxY
        XCTContext.runActivity(named: "#208 \(name) 返事待ち: send=\(send.frame) keyboardTop=\(kb.frame.minY) gap=\(gapWaiting)") { _ in }
        print("[#208] \(name) waiting send=\(send.frame) keyboardTop=\(kb.frame.minY) gap=\(gapWaiting)")
        attachScreenshot("kb208-\(name)-waiting")
        XCTAssertFalse(replied.exists, "キーボードを出す前に返事が届いた(待ちが短い)", file: file, line: line)
        XCTAssertGreaterThanOrEqual(gapWaiting, -1, "返事待ち: 入力欄がキーボードに隠れている", file: file, line: line)
        XCTAssertLessThanOrEqual(gapWaiting, 80, "返事待ち: 入力欄がキーボードから離れすぎ(二重に持ち上がっている)", file: file, line: line)

        XCTAssertTrue(replied.waitForExistence(timeout: 60), "返事が届かない", file: file, line: line)
        Thread.sleep(forTimeInterval: 0.8) // 返事の吹き出し・スクロールのアニメーション
        settle()
        XCTAssertTrue(kb.exists, "返事が届いたらキーボードが閉じた", file: file, line: line)
        let gapReplied = kb.frame.minY - send.frame.maxY
        XCTContext.runActivity(named: "#208 \(name) 返事後: send=\(send.frame) keyboardTop=\(kb.frame.minY) gap=\(gapReplied)") { _ in }
        print("[#208] \(name) replied send=\(send.frame) keyboardTop=\(kb.frame.minY) gap=\(gapReplied)")
        attachScreenshot("kb208-\(name)-replied")
        XCTAssertGreaterThanOrEqual(gapReplied, -1, "返事後: 入力欄がキーボードに隠れている", file: file, line: line)
        XCTAssertLessThanOrEqual(gapReplied, 80, "返事後: 入力欄がキーボードから離れすぎ", file: file, line: line)
        XCTAssertEqual(gapReplied, gapWaiting, accuracy: 2, "返事が届いて入力欄が動いた", file: file, line: line)
        // 一覧だけが伸びて最新(案のカードなら 2 枚目の「これでいく」)まで見えている
        let cards = app.buttons.matching(identifier: "chat.option.choose")
        if cards.count == 2 {
            let lastCard = cards.element(boundBy: 1)
            XCTAssertTrue(waitUntil(timeout: 3) { lastCard.frame.maxY <= send.frame.minY + 1 },
                          "返事後: 最新の案がスクロールされず入力欄の裏にある(\(lastCard.frame))", file: file, line: line)
        }
        // タップして打てる
        field.tap()
        field.typeText("ok")
        XCTAssertTrue((field.value as? String ?? "").contains("ok"), "返事後に入力欄へ打てない", file: file, line: line)
    }

    func testDiaryBodyReturnInsertsNewlineAboveKeyboard() {
        launch(example: "diary_ai", ["METAMON_AUTO_OPEN": "diary_form:push"])
        let body = app.descendants(matching: .any)["field.body"].firstMatch
        XCTAssertTrue(body.waitForExistence(timeout: 15), "日記の本文欄が無い")
        body.tap()
        body.typeText("a\nb\nc\nd\ne\nf")
        Thread.sleep(forTimeInterval: 0.6)
        let value = body.value as? String ?? ""
        XCTAssertTrue(value.contains("a\nb\nc"), "本文に改行が入らない: \(value)")
        if let top = keyboardTop {
            XCTAssertLessThanOrEqual(body.frame.minY + 30, top, "編集中の本文欄がキーボードに隠れている")
        }
        attachScreenshot("diary-body-multiline")
    }

    // MARK: - 生成中の表示(#182)

    /// 全画面の「変身中」(GeneratingView)
    private var generating: XCUIElement { app.descendants(matching: .any)["generating.create"] }
    /// モック ai に待ちを入れる(生成中の表示を確実に捉えるため)。生成 6 秒・対話 3 秒(MockAIClient は chat に半分を使う)。
    /// 3 秒だと、音声シートが閉じる間や遅い CI で「出た」と気づくまでに完成してしまい誤検知になった(#200)
    private let slowAI = ["METAMON_MOCK_AI_DELAY": "6"]

    /// 音声画面・対話の文字入力(ChatComposer・#196)
    private var composerField: XCUIElement { app.descendants(matching: .any)["composer.field"].firstMatch }

    /// 生成中の表示が `appearWithin` 秒以内に出て、完成まで出続け、消えたら生成したアプリ(マイク FAB)に切り替わる。
    /// 途中で初期画面に戻らないことは「消えた直後に初期画面の文字入力が無い」+「ある程度の時間出続けた」で見る。
    private func assertGeneratingThenDone(_ name: String, appearWithin: TimeInterval = 15, file: StaticString = #filePath, line: UInt = #line) {
        let appeared = generating.waitForExistence(timeout: appearWithin)
        // 出なかった時は、アプリ側の状態(morphJob が立ったか・GeneratingView の onAppear が呼ばれたか)を一緒に残す
        let probe = app.descendants(matching: .any)["debug.morphProbe"]
        let state = probe.exists ? probe.label : "probe なし"
        if !appeared { attachScreenshot("generating-missing-\(name)") }
        // 通った時も残す(RootView の置き換えと VoiceInputView の保険のどちらで出たかを CI で見る)
        XCTContext.runActivity(named: "morphProbe(\(name)): \(state)") { _ in }
        XCTAssertTrue(appeared, "生成中の表示が出ない(\(name)・\(state))", file: file, line: line)
        let shown = Date()
        attachScreenshot("generating-\(name)")
        XCTAssertTrue(waitFor(generating, "exists == false", timeout: 30), "完成しても生成中の表示が消えない(\(name))", file: file, line: line)
        let visible = Date().timeIntervalSince(shown)
        XCTContext.runActivity(named: "morphProbe(\(name)) 完成後: \(probe.exists ? probe.label : "probe なし")") { _ in }
        XCTAssertFalse(composerField.exists, "生成中の表示が消えて初期画面に戻った(\(name))", file: file, line: line)
        // モックの待ちは 6 秒。気づくまでの遅れを引いても 1.5 秒は出続けるはず(すぐ消えたら途中で落ちている)
        XCTAssertGreaterThan(visible, 1.5, "生成中の表示が完成前に消えた(\(name)・\(visible) 秒)", file: file, line: line)
        XCTAssertTrue(fab.waitForExistence(timeout: 15), "完成後に生成したアプリに切り替わらない(\(name))", file: file, line: line)
    }

    /// 対話なし(設定 OFF): 初期画面で文字入力して送ると、すぐ生成中になり完成で消える
    func testGeneratingShownWithoutChat() {
        launch(example: nil, slowAI.merging(["METAMON_CLARIFY": "0"]) { $1 })
        let field = composerField // 複数行の入力欄(#196)は placeholder が出ないので id で探す
        XCTAssertTrue(field.waitForExistence(timeout: 15), "初期画面の文字入力が無い")
        field.tap()
        field.typeText("家計簿アプリを作って")
        app.buttons["送信"].firstMatch.tap()
        assertGeneratingThenDone("no-chat")
    }

    /// 対話あり:「これでいく」を押すと生成中(初期画面に戻らない)
    func testGeneratingShownAfterChatBuild() {
        launch(example: nil, slowAI.merging(["METAMON_CLARIFY": "1", "METAMON_AUTO_TRANSCRIPT": "毎日の支出を記録する家計簿アプリを作って",
                                             "METAMON_AUTO_PAUSE": "600"]) { $1 })
        let build = app.buttons["chat.build"]
        XCTAssertTrue(build.waitForExistence(timeout: 20), "「これでいく」が出ない")
        build.tap()
        assertGeneratingThenDone("chat-build")
    }

    /// 声で答えて ready → 自動で作る(#163)も同じ表示
    func testGeneratingShownForVoiceAutoBuild() {
        launch(example: nil, slowAI.merging(["METAMON_CLARIFY": "1", "METAMON_CLARIFY_AUTOBUILD": "1", "METAMON_AUTO_SOURCE": "voice",
                                             "METAMON_AUTO_TRANSCRIPT": "毎日の支出を記録する家計簿アプリを作って"]) { $1 })
        assertGeneratingThenDone("voice-auto-build", appearWithin: 25)
    }

    /// 生成後のアプリから音声シートで「〇〇アプリを作って」(作り直し)も全画面の生成中にする
    func testGeneratingShownForWholeAppRequestFromGeneratedApp() {
        // 対話を挟むと edit の「これでいく」待ちで止まるので、対話は OFF に固定する(既定値・端末の設定に依存させない)
        launch(example: "kakeibo", slowAI.merging(["METAMON_CLARIFY": "0"]) { $1 })
        waitForFab()
        fab.tap()
        XCTAssertTrue(voiceSheet.waitForExistence(timeout: 15), "音声シートが開かない")
        let field = composerField // 複数行の入力欄(#196)は placeholder が出ないので id で探す
        XCTAssertTrue(field.waitForExistence(timeout: 15), "シートの文字入力が無い")
        field.tap()
        field.typeText("筋トレの記録アプリを作って")
        app.buttons["送信"].firstMatch.tap()
        assertGeneratingThenDone("from-app")
    }
}

// MARK: - 有料機能の確認カード(#77)

extension MetamonUITests {
    /// 課金を有効(`METAMON_IAP_ENFORCE=1`)にして「みんなで書き込む掲示板」を相談 →「これでいく」で確認カード →
    /// 「この機能なしで作る」で生成が始まり、アプリができる。無料枠に当たらないよう `METAMON_IAP_FREE_LIMIT` を大きくする。
    func testPaidFeatureCardBuildWithoutFeature() {
        launch(example: nil, slowAI.merging(["METAMON_CLARIFY": "1", "METAMON_IAP_ENFORCE": "1", "METAMON_IAP_FREE_LIMIT": "1000",
                                             "METAMON_AUTO_TRANSCRIPT": "サークルのみんなで予定を書き込める掲示板アプリを作って",
                                             "METAMON_AUTO_PAUSE": "600"]) { $1 })
        let build = app.buttons["chat.build"]
        XCTAssertTrue(build.waitForExistence(timeout: 15), "「これでいく」が出ない")
        build.tap()
        let headline = app.descendants(matching: .any).matching(identifier: "paid.headline").firstMatch
        XCTAssertTrue(headline.waitForExistence(timeout: 5), "有料機能の確認カードが出ない")
        XCTAssertTrue(headline.label.contains("Tailor It Pro"), "見出し: \(headline.label)")
        XCTAssertTrue(app.buttons["paid.purchase"].exists, "「購入して作る」が無い")
        XCTAssertFalse(generating.exists, "カードの間は生成しない")
        attachScreenshot("paid-feature-card")
        app.buttons["paid.withoutFeature"].tap()
        assertGeneratingThenDone("paid-without")
    }
}

// MARK: - 1 つ前に戻す / やり直す(#209)

extension MetamonUITests {
    private func text(_ label: String) -> XCUIElement {
        app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", label)).firstMatch
    }

    /// 家計簿で修正(モックは「筋トレの記録」でトレーニング記録に変わる)→ 要約の「元に戻す」で家計簿に戻る →「やり直す」で再び修正後に。
    /// 修正チャット(音声シート)の上部の「1つ前に戻す」も、戻せる版が無い時は押せない。
    func testUndoFromSummaryBannerAndRedo() {
        launch(example: "kakeibo", ["METAMON_CLARIFY": "0", "METAMON_MOCK_AI_DELAY": "0.5"])
        waitForFab()
        XCTAssertTrue(text("最近の支出").waitForExistence(timeout: 15), "家計簿で起動していない")
        fab.tap()
        XCTAssertTrue(voiceSheet.waitForExistence(timeout: 15), "音声シートが開かない")
        let headerUndo = app.buttons["voice.undo"]
        XCTAssertTrue(headerUndo.exists, "修正チャットの上部に「1つ前に戻す」が無い")
        XCTAssertFalse(headerUndo.isEnabled, "戻せる版が無いのに押せる")
        attachScreenshot("undo-header-disabled")

        let field = composerField
        XCTAssertTrue(field.waitForExistence(timeout: 15), "シートの文字入力が無い")
        field.tap()
        field.typeText("筋トレの記録")
        app.buttons["送信"].firstMatch.tap()

        let undo = app.buttons["banner.undo"]
        XCTAssertTrue(undo.waitForExistence(timeout: 20), "修正直後の要約に「元に戻す」が出ない")
        XCTAssertTrue(text("反映しました").exists, "要約が出ていない")
        XCTAssertFalse(text("最近の支出").exists, "修正が反映されていない")
        attachScreenshot("undo-banner-after-edit")
        undo.tap()

        XCTAssertTrue(text("1つ前に戻しました").waitForExistence(timeout: 15), "「1つ前に戻しました」が出ない")
        XCTAssertTrue(text("最近の支出").waitForExistence(timeout: 15), "前の画面(家計簿)に戻らない")
        let redo = app.buttons["banner.redo"]
        XCTAssertTrue(redo.exists, "戻した後に「やり直す」が無い")
        attachScreenshot("undo-banner-restored")
        redo.tap()

        XCTAssertTrue(text("やり直しました").waitForExistence(timeout: 15), "「やり直しました」が出ない")
        XCTAssertTrue(waitFor(text("最近の支出"), "exists == false", timeout: 15), "やり直すで修正後に戻らない")
        attachScreenshot("undo-redo-done")

        // 修正チャットの上部からも戻せる
        fab.tap()
        XCTAssertTrue(headerUndo.waitForExistence(timeout: 15), "音声シートが開かない")
        XCTAssertTrue(headerUndo.isEnabled, "戻せる版があるのに押せない")
        attachScreenshot("undo-header-enabled")
        headerUndo.tap()
        XCTAssertTrue(text("最近の支出").waitForExistence(timeout: 15), "上部の「1つ前に戻す」で前の画面に戻らない")
        attachScreenshot("undo-header-restored")
    }
}
