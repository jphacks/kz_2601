import XCTest
import MetamonRenderer
@testable import Metamon

/// 「ショートカットで自動にする」の Mac の案内(#3080): 例が Mac にある Intent だけを使うこと・英訳・言い回し。
@MainActor
final class MacAutomationGuideTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    /// 例の動きは、Mac にある「記録を追加」だけ。名前は Intent の名前(ショートカット App で探す名前)と同じ
    func testExamplesUseOnlyExistingIntent() {
        XCTAssertEqual(AddRecordIntent.title.key, "記録を追加")
        XCTAssertEqual(MacAutomationGuide.examples.map(\.id), ["appOpen", "appClose", "focusOn", "focusOff", "wifi"])
        for example in MacAutomationGuide.examples {
            XCTAssertTrue(example.actionKey.contains("「\(AddRecordIntent.title.key)」"), example.id)
            // Mac に無い Intent の名前(アプリを開く・作る系)を例に入れない
            XCTAssertFalse(example.actionKey.contains("「アプリを"), example.id)
        }
        XCTAssertTrue(MacAutomationGuide.stepKeys.contains { $0.contains("「\(AddRecordIntent.title.key)」") })
        XCTAssertTrue(MacAutomationGuide.otherActionsKey.contains("「\(TalkToTailorItIntent.title.key)」"))
        XCTAssertEqual(english(AddRecordIntent.title.key), "Add Record")
        XCTAssertEqual(english(TalkToTailorItIntent.title.key), "Talk")
        XCTAssertTrue(english(MacAutomationGuide.stepKeys[3]).contains("“Add Record”"))
        XCTAssertTrue(english(MacAutomationGuide.otherActionsKey).contains("“Talk”"))
    }

    /// Mac に無いきっかけ(iPhone・NFC)と、ロック中になりやすいきっかけは出さない
    func testTriggersFitMac() {
        for banned in ["iPhone", "NFC", "時になった時", "着いた時", "充電"] {
            XCTAssertFalse(MacAutomationGuide.allKeys.contains { $0.contains(banned) }, banned)
        }
        XCTAssertEqual(Set(MacAutomationGuide.allKeys).count, MacAutomationGuide.allKeys.count, "文の重複")
        XCTAssertEqual(MacAutomationGuide.shortcutsURL.scheme, "shortcuts")
    }

    /// 「記録を追加」はロック解除が要る。案内も Mac のロック解除と書く
    func testAddRecordRequiresUnlockAndGuideSaysSo() {
        guard case .requiresLocalDeviceAuthentication = AddRecordIntent.authenticationPolicy else {
            return XCTFail("AddRecordIntent はロック解除が要る")
        }
        let note = MacAutomationGuide.noteKeys.first { $0.contains("ロックを解除") } ?? ""
        XCTAssertTrue(note.contains("Mac のロック"), note)
        XCTAssertTrue(english(note).contains("unlock"), english(note))
    }

    func testEnglishTranslationsExist() {
        for key in MacAutomationGuide.allKeys {
            XCTAssertNotEqual(english(key), key, "英訳が無い: \(key)")
        }
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(MacAutomationGuide.title, "Automate with Shortcuts")
    }

    /// 審査用の版でも出すので「アプリを作る」系の言い方をしない(docs/store/wording.md)
    func testWording() {
        for key in MacAutomationGuide.allKeys {
            for banned in ["アプリを作", "作ったアプリ", "生成", "変身"] {
                XCTAssertFalse(key.contains(banned), key)
            }
        }
    }

    private func english(_ key: String) -> String {
        guard let path = Bundle(for: MacAppRuntime.self).path(forResource: "en", ofType: "lproj"),
              let bundle = Bundle(path: path) else { return key }
        return bundle.localizedString(forKey: key, value: key, table: nil)
    }
}
