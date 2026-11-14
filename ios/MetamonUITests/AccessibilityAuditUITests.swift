import XCTest

/// #3074: 主な 3 画面(起動 → 話しかける画面 → 設定)で iOS 17 の `performAccessibilityAudit` を 1 本だけ回す。
/// 今すぐ直せない指摘は `allowedIssues` に理由付きで置き、それ以外が 1 件でも出たら落ちる。
final class AccessibilityAuditUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted",
                                 "METAMON_TUTORIAL": "seen", "METAMON_CLARIFY": "0",
                                 "METAMON_INITIAL_EXAMPLE": "kakeibo"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    func testMainScreensPassAccessibilityAudit() throws {
        let fab = app.buttons["voice.fab"]
        XCTAssertTrue(fab.waitForExistence(timeout: 30), "マイク FAB が出ない")
        try audit(screen: "launch")

        fab.tap()
        let settingsEntry = app.buttons["voice.settings"]
        XCTAssertTrue(settingsEntry.waitForExistence(timeout: 15), "音声シートが開かない")
        try audit(screen: "voice-sheet")

        settingsEntry.tap()
        XCTAssertTrue(app.staticTexts["settings.accountStatus"].waitForExistence(timeout: 15), "設定が開かない")
        try audit(screen: "settings")
    }

    // MARK: - 許す一覧

    /// 直せない・直すと別の Issue になる指摘。監査の種類と、要素の説明に含まれる文字列で照合する
    private struct AllowedIssue {
        let auditType: XCUIAccessibilityAuditType
        /// 空なら要素を問わずその種類すべてを許す
        let elementContains: String
        let reason: String
    }

    private let allowedIssues: [AllowedIssue] = [
        AllowedIssue(auditType: .dynamicType, elementContains: "",
                     reason: "生成されたアプリの中身は DSL 次第で文字の大きさに追従しきれない。部品ごとの対応は別 Issue"),
        AllowedIssue(auditType: .textClipped, elementContains: "",
                     reason: "最大の文字サイズで行がはみ出す。行の高さの見直しは別 Issue"),
        AllowedIssue(auditType: .contrast, elementContains: "",
                     reason: "ブランド色の配色の見直しはデザインの判断が要るので別 Issue"),
        AllowedIssue(auditType: .hitRegion, elementContains: "voice.screenContext",
                     reason: "音声シートの「画面の指定を外す」印。見た目を変えずに触れる広さだけ足す対応は別 Issue"),
        AllowedIssue(auditType: .sufficientElementDescription, elementContains: "voice.screenContext",
                     reason: "SF Symbol の名前がそのままラベルになっている。読み上げ用の文言を付ける対応は別 Issue"),
        AllowedIssue(auditType: .sufficientElementDescription, elementContains: "person.crop.circle.dashed",
                     reason: "設定のアカウント欄の飾りの記号。同じく SF Symbol の名前がラベルに出る。別 Issue"),
        AllowedIssue(auditType: .hitRegion, elementContains: "voice.tips",
                     reason: "音声シートの「頼み方のコツ」の小さい文字。触れる広さの見直しは別 Issue"),
    ]

    private func audit(screen: String, file: StaticString = #filePath, line: UInt = #line) throws {
        var unexpected: [String] = []
        try app.performAccessibilityAudit { issue in
            let element = issue.element?.debugDescription ?? ""
            if let allowed = self.allowedIssues.first(where: {
                issue.auditType.contains($0.auditType) && ($0.elementContains.isEmpty || element.contains($0.elementContains))
            }) {
                print("[a11y:\(screen)] 許した指摘: \(issue.compactDescription) / 理由: \(allowed.reason)")
                return true
            }
            unexpected.append("\(issue.compactDescription)[type=\(issue.auditType.rawValue)](element=\(element))")
            return true
        }
        attach(screen)
        XCTAssertTrue(unexpected.isEmpty, "\(screen) の監査で許していない指摘: \(unexpected.joined(separator: " / "))", file: file, line: line)
    }

    private func attach(_ name: String) {
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = "a11y-\(name)"
        shot.lifetime = .keepAlways
        add(shot)
    }
}
