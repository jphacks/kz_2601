import XCTest
import SwiftUI
import MetamonRenderer
@testable import Metamon

/// #1995 ショートカットで自動にする: 案内の中身・英訳・今ある Intent の名前との一致・画面の描画(日本語・英語)。
/// 環境変数 AUTOMATION_SHOTS_DIR(xcodebuild では TEST_RUNNER_AUTOMATION_SHOTS_DIR)があれば PNG を書き出す(PR のスクショ用)。
@MainActor
final class AutomationGuideTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    /// 例は iPhone を使っている時のきっかけ(App を開く・閉じる・NFC)だけで、どれも今ある「記録を追加」を使う。
    /// 「記録を追加」はロック解除が要るので、ロック中になりやすい時刻・場所・充電の例は出さない。
    func testExamplesUseInUseTriggers() {
        XCTAssertEqual(AutomationGuide.examples.map(\.id), ["appOpen", "appClose", "nfc", "nfcAsk"])
        for banned in ["時になった時", "着いた時", "充電"] {
            XCTAssertFalse(AutomationGuide.allKeys.contains { $0.contains(banned) }, banned)
        }
        for example in AutomationGuide.examples {
            XCTAssertTrue(example.actionKey.contains("「記録を追加」"), example.id)
        }
        XCTAssertEqual(Set(AutomationGuide.allKeys).count, AutomationGuide.allKeys.count, "文の重複")
        XCTAssertEqual(AutomationGuide.shortcutsURL.scheme, "shortcuts")
    }

    /// 案内に出す動きの名前 = 実際の Intent の名前(ショートカット App で探す時の名前)。ja / en とも。
    func testActionNamesMatchExistingIntents() {
        // Intent の名前を en.lproj で引き、案内の英訳と同じ名前になることを確かめる(実行環境の言語に左右されない)
        XCTAssertEqual(AddRecordIntent.title.key, "記録を追加")
        XCTAssertEqual(TalkToTailorItIntent.title.key, "話しかける")
        XCTAssertEqual(english(AddRecordIntent.title.key), "Add Record")
        XCTAssertEqual(english(TalkToTailorItIntent.title.key), "Talk")
        XCTAssertTrue(AutomationGuide.otherActionsKey.contains("「話しかける」"))
        XCTAssertTrue(AutomationGuide.stepKeys.contains { $0.contains("「記録を追加」") })
        XCTAssertTrue(english(AutomationGuide.stepKeys[3]).contains("“Add Record”"))
        XCTAssertTrue(english(AutomationGuide.otherActionsKey).contains("“Talk”"))
    }

    func testEnglishTranslationsExist() {
        for key in AutomationGuide.allKeys + ["ショートカットで自動にする", "App を開いた時などをきっかけに「記録を追加」を動かす作り方を見ます"] {
            XCTAssertNotEqual(english(key), key, "英訳が無い: \(key)")
        }
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(AutomationGuide.title, "Automate with Shortcuts")
    }

    /// 審査用の版でも出すので「アプリを作る」系の言い方をしない(docs/store/wording.md)。ロック中の注意は必ず出す。
    func testWording() {
        for key in AutomationGuide.allKeys {
            for banned in ["アプリを作", "作ったアプリ", "生成", "変身"] {
                XCTAssertFalse(key.contains(banned), key)
            }
        }
        XCTAssertTrue(AutomationGuide.noteKeys.contains { $0.contains("ロック中") })
    }

    /// 「記録を追加」はロック解除が要る。案内も「記録するにはロックの解除が要る」と書く(ja / en)。
    func testAddRecordRequiresUnlockAndGuideSaysSo() {
        guard case .requiresLocalDeviceAuthentication = AddRecordIntent.authenticationPolicy else {
            return XCTFail("AddRecordIntent はロック解除が要る")
        }
        let note = AutomationGuide.noteKeys.first { $0.contains("ロック") } ?? ""
        XCTAssertTrue(note.contains("ロックを解除"), note)
        XCTAssertFalse(note.contains("確認が出ることがあります"), note)
        XCTAssertTrue(english(note).contains("unlock"), english(note))
    }

    /// 画面のスナップショット(日本語・英語)。描けて空でないことを確かめ、指定があれば PNG を書き出す。
    func testRendersInJapaneseAndEnglish() throws {
        for language in [AppLanguage.ja, .en] {
            LanguagePreference.shared.setting = language
            let image = render(NavigationStack { AutomationGuideView() }
                .environment(\.locale, LanguagePreference.shared.locale))
            XCTAssertGreaterThan(distinctColors(image), 4, "\(language): 何も描かれていない")
            let png = try XCTUnwrap(image.pngData())
            let attachment = XCTAttachment(data: png, uniformTypeIdentifier: "public.png")
            attachment.name = "automation-guide-\(language.rawValue).png"
            attachment.lifetime = .keepAlways
            add(attachment)
            if let dir = ProcessInfo.processInfo.environment["AUTOMATION_SHOTS_DIR"] {
                try png.write(to: URL(fileURLWithPath: dir).appendingPathComponent(attachment.name ?? "shot.png"))
            }
        }
    }

    // MARK: - helpers

    private func render(_ view: some View) -> UIImage {
        let size = CGSize(width: 393, height: 1400)
        // テストのホスト(本体)のシーンに載せないと drawHierarchy が空になる
        let scene = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first
        let window = scene.map { UIWindow(windowScene: $0) } ?? UIWindow()
        window.frame = CGRect(origin: .zero, size: size)
        let host = UIHostingController(rootView: view)
        window.rootViewController = host
        window.makeKeyAndVisible()
        host.view.frame = window.bounds
        host.view.setNeedsLayout()
        host.view.layoutIfNeeded()
        RunLoop.main.run(until: Date().addingTimeInterval(1))
        let image = UIGraphicsImageRenderer(bounds: window.bounds).image { _ in
            window.drawHierarchy(in: window.bounds, afterScreenUpdates: true)
        }
        window.isHidden = true
        return image
    }

    private func distinctColors(_ image: UIImage) -> Int {
        guard let cg = image.cgImage, let data = cg.dataProvider?.data, let bytes = CFDataGetBytePtr(data) else { return 0 }
        var seen = Set<UInt32>()
        let bpr = cg.bytesPerRow, bpp = cg.bitsPerPixel / 8
        for y in stride(from: 0, to: cg.height, by: 7) {
            for x in stride(from: 0, to: cg.width, by: 7) {
                let i = y * bpr + x * bpp
                seen.insert(UInt32(bytes[i]) << 16 | UInt32(bytes[i + 1]) << 8 | UInt32(bytes[i + 2]))
                if seen.count > 64 { return seen.count }
            }
        }
        return seen.count
    }

    private func english(_ key: String) -> String {
        guard let path = Bundle(for: AppRuntime.self).path(forResource: "en", ofType: "lproj"),
              let bundle = Bundle(path: path) else { return key }
        return bundle.localizedString(forKey: key, value: key, table: nil)
    }
}
