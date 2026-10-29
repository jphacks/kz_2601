import XCTest

/// #64: 審査デモ動画の下書き用。`docs/03_demo_script.md` §1(3 分版)の場面をモック(`METAMON_MOCK=1`)で順に流す。
/// 発話の代わりに文字入力で送る。画面収録は外側(`xcrun simctl io <id> recordVideo`)で行い、字幕は後から ffmpeg で焼く。
/// CI では回さない: `DEMO_RECORD=1` の時だけ実行(xcodebuild には `TEST_RUNNER_DEMO_RECORD=1` で渡す)。
/// 各場面の開始時刻を `DEMO_MARK <名前> <UNIX 秒>` で標準出力に出す(字幕の位置合わせ用)。
final class DemoRecordingUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUpWithError() throws {
        try XCTSkipUnless(ProcessInfo.processInfo.environment["DEMO_RECORD"] == "1", "DEMO_RECORD=1 の時だけ実行(デモ動画の収録用)")
        continueAfterFailure = true
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    func testDemoScript3Min() {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted",
                                 "METAMON_CLARIFY": "0", "METAMON_MOCK_AI_DELAY": "3"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja",
                               "-metamon.accountLinkPrompted", "YES"]
        // 通知の許可(デモ B)はシステムのダイアログ。言語に関わらず「許可」側を押す
        addUIInterruptionMonitor(withDescription: "system alert") { alert in
            for label in ["許可", "Allow", "OK"] where alert.buttons[label].exists {
                alert.buttons[label].tap()
                return true
            }
            return false
        }
        app.launch()

        // 0:00 導入: 初期画面
        mark("intro")
        let field = composerField
        XCTAssertTrue(field.waitForExistence(timeout: 20), "初期画面の文字入力が無い")
        pause(6)

        // 0:20 デモ A 家計簿(モックは「レシート」で別サンプルになるので最後の 1 文は省く)
        mark("demoA")
        type(into: field, "毎日の出費をメモできる家計簿アプリを作って。今月いくら使ったかと支出の一覧、カテゴリ別のグラフも見たい")
        pause(1)
        app.buttons["送信"].firstMatch.tap()
        mark("demoA_generating")
        XCTAssertTrue(fab.waitForExistence(timeout: 40), "家計簿が生成されない")
        mark("demoA_done")
        pause(4)
        dismissSummary()

        let add = app.buttons["支出を追加"].firstMatch
        if add.waitForExistence(timeout: 10) {
            mark("demoA_add")
            add.tap()
            let amount = app.textFields.matching(NSPredicate(format: "placeholderValue == '0'")).firstMatch
            if amount.waitForExistence(timeout: 10) {
                amount.tap()
                slowType("480")
            }
            let memo = app.textFields.matching(NSPredicate(format: "placeholderValue == 'コンビニ'")).firstMatch
            if memo.waitForExistence(timeout: 5) {
                memo.tap()
                slowType("コンビニ")
            }
            pause(1.5)
            app.buttons["保存"].firstMatch.tap()
            mark("demoA_saved")
            pause(4)
        }

        // 0:55 改変: FAB → 音声シートの文字入力
        mark("edit")
        sendFromFab("今月の合計をもっと大きく表示して")
        mark("edit_done")
        pause(5)
        dismissSummary()
        let summaryTab = app.buttons["集計"].firstMatch
        if summaryTab.waitForExistence(timeout: 5) {
            mark("demoA_chart")
            summaryTab.tap()
            pause(4)
            app.buttons["ホーム"].firstMatch.tap()
            pause(1)
        }

        // 1:15 デモ B 筋トレ(通知)
        mark("demoB")
        sendFromFab("別のアプリ。筋トレの記録アプリ。腕立てとスクワットの回数を毎日つけて、朝8時にリマインドして")
        mark("demoB_done")
        pause(3)
        app.tap() // 割り込みモニタを発火させる(通知の許可ダイアログ)
        pause(4)
        dismissSummary()
        pause(2)

        // 1:45 デモ C 散歩メモ(写真の撮影は実機で撮り直す)
        mark("demoC")
        sendFromFab("別のアプリ。散歩で見つけたいい場所を写真とひとことで残すメモアプリ。夜に見返したい")
        mark("demoC_done")
        pause(4)
        dismissSummary()
        // 散歩メモのサンプルは追加ボタンが無く、記録は右上のカメラから(シミュレータにカメラは無い)。一覧を見せて終わる
        pause(4)

        // 2:15 技術の肝 / 2:45 締め: 口頭パート。画面は最後のアプリのまま(字幕を載せる尺)
        mark("outro")
        pause(15)
        mark("end")
    }

    // MARK: - 補助

    private var fab: XCUIElement { app.buttons["voice.fab"] }
    private var composerField: XCUIElement { app.descendants(matching: .any)["composer.field"].firstMatch }

    private func sendFromFab(_ text: String) {
        XCTAssertTrue(fab.waitForExistence(timeout: 20), "マイク FAB が無い")
        fab.tap()
        let field = composerField
        XCTAssertTrue(field.waitForExistence(timeout: 15), "音声シートの文字入力が無い")
        pause(1)
        type(into: field, text)
        pause(1)
        let send = app.buttons["composer.send"].firstMatch
        if send.exists { send.tap() } else { app.buttons["送信"].firstMatch.tap() }
        // 生成中 → 完了でシートが閉じて FAB が戻る
        pause(2)
        XCTAssertTrue(waitUntil(timeout: 40) { [self] in fab.exists && !composerField.exists }, "生成が終わらない: \(text)")
    }

    /// 完成時の要約カード(タップで消える)が画面に被らないように
    private func dismissSummary() {
        let card = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "(モック)")).firstMatch
        if card.exists { card.tap() }
    }

    /// 打っている様子が映るよう、1 回の typeText を数文字ずつに分ける
    private func type(into field: XCUIElement, _ text: String) {
        field.tap()
        slowType(text)
    }

    private func slowType(_ text: String) {
        var chunk = ""
        for ch in text {
            chunk.append(ch)
            if chunk.count >= 4 {
                app.typeText(chunk)
                chunk = ""
            }
        }
        if !chunk.isEmpty { app.typeText(chunk) }
    }

    private func mark(_ name: String) {
        print("DEMO_MARK \(name) \(String(format: "%.2f", Date().timeIntervalSince1970))")
    }

    private func pause(_ seconds: TimeInterval) { Thread.sleep(forTimeInterval: seconds) }

    private func waitUntil(timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(timeout)
        while Date() < end {
            if condition() { return true }
            Thread.sleep(forTimeInterval: 0.5)
        }
        return false
    }
}
