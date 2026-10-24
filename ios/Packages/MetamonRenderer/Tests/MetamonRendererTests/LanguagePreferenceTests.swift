import XCTest
@testable import MetamonRenderer

/// 表示言語の切替(#44): 設定値で String Catalog の引き先(ja / en)が即座に変わること。
final class LanguagePreferenceTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() { saved = LanguagePreference.shared.setting }
    override func tearDown() { LanguagePreference.shared.setting = saved }

    func testExplicitLanguageSwitchesStrings() {
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(LanguagePreference.shared.code, "en")
        XCTAssertEqual(L("保存しました"), "Saved")
        XCTAssertEqual(L("画面がありません: \("home")"), "Screen not found: home")

        LanguagePreference.shared.setting = .ja
        XCTAssertEqual(LanguagePreference.shared.code, "ja")
        XCTAssertEqual(L("保存しました"), "保存しました")
        XCTAssertEqual(LanguagePreference.shared.locale.language.languageCode?.identifier, "ja")
    }

    func testSystemResolvesToSupportedLanguage() {
        LanguagePreference.shared.setting = .system
        XCTAssertTrue(LanguagePreference.supported.contains(LanguagePreference.shared.code))
    }
}
