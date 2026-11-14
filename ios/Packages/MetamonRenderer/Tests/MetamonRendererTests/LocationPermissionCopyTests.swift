import XCTest
@testable import MetamonRenderer

/// #2731: 位置情報を断られた時の案内(`LocationPermissionCopy`)。
/// Mac は「システム設定 > プライバシーとセキュリティ > 位置情報サービス」へ、iOS は「設定」アプリのこのアプリの画面へ(#956)。
/// 開き先の表の中身は PrivacySettingsPane2257Tests。ここは 4 つの文言と開き先が OS で入れ替わることを見る。
final class LocationPermissionCopyTests: XCTestCase {
    private var savedLanguage: AppLanguage = .system

    override func setUp() {
        savedLanguage = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = savedLanguage }

    private var copies: [String] {
        [LocationPermissionCopy.notAllowed,
         LocationPermissionCopy.cannotGuide,
         LocationPermissionCopy.cannotRecord,
         LocationPermissionCopy.cannotSearchNearby]
    }

    // MARK: - 開き先

    func testSettingsURLMatchesThisOS() {
        let url = LocationPermissionCopy.settingsURL
        #if os(macOS)
        XCTAssertEqual(url?.absoluteString, "x-apple.systempreferences:com.apple.preference.security?Privacy_LocationServices")
        XCTAssertEqual(url, PrivacySettingsPane.location.url(on: .macOS))
        #else
        XCTAssertEqual(url?.absoluteString, "app-settings:")
        XCTAssertEqual(url, PrivacySettingsPane.location.url(on: .iOS))
        #endif
    }

    func testSettingsURLSchemeIsOpenable() throws {
        let scheme = try XCTUnwrap(LocationPermissionCopy.settingsURL?.scheme)
        #if os(macOS)
        XCTAssertEqual(scheme, "x-apple.systempreferences")
        #else
        XCTAssertEqual(scheme, "app-settings")
        #endif
    }

    // MARK: - 4 つの文言

    func testFourCopiesAreFilledAndDistinct() {
        for copy in copies {
            XCTAssertFalse(copy.isEmpty)
            XCTAssertTrue(copy.contains("位置情報"), copy)
        }
        XCTAssertEqual(Set(copies).count, 4, "4 つの場面で別の文言を出す")
    }

    func testCopiesGuideToThisOSSettings() {
        #if os(macOS)
        for copy in copies {
            XCTAssertTrue(copy.contains("「システム設定 > プライバシーとセキュリティ > 位置情報サービス」"), copy)
            XCTAssertFalse(copy.contains("このAppの使用中のみ許可"), copy)
        }
        #else
        for copy in copies {
            XCTAssertFalse(copy.contains("システム設定"), copy)
        }
        for copy in [LocationPermissionCopy.cannotGuide,
                     LocationPermissionCopy.cannotRecord,
                     LocationPermissionCopy.cannotSearchNearby] {
            XCTAssertTrue(copy.contains("設定で「このAppの使用中のみ許可」"), copy)
        }
        XCTAssertEqual(LocationPermissionCopy.notAllowed, "位置情報の利用が許可されていません")
        #endif
    }

    func testEachCopyNamesWhatCannotBeDone() {
        XCTAssertTrue(LocationPermissionCopy.cannotGuide.contains("案内できません"))
        XCTAssertTrue(LocationPermissionCopy.cannotRecord.contains("記録できません"))
        XCTAssertTrue(LocationPermissionCopy.cannotSearchNearby.contains("近くを探せません"))
        XCTAssertTrue(LocationPermissionCopy.notAllowed.contains("許可されていません"))
    }

    // MARK: - 英語

    func testCopiesAreEnglish() {
        LanguagePreference.shared.setting = .en
        #if os(macOS)
        XCTAssertEqual(LocationPermissionCopy.notAllowed,
                       "Location access is not allowed. Allow it in System Settings > Privacy & Security > Location Services.")
        XCTAssertEqual(LocationPermissionCopy.cannotGuide,
                       "Location is off, so guidance can’t start. Allow this app in System Settings > Privacy & Security > Location Services.")
        XCTAssertEqual(LocationPermissionCopy.cannotRecord,
                       "Location is off. Allow this app in System Settings > Privacy & Security > Location Services to record.")
        for copy in copies {
            XCTAssertTrue(copy.contains("System Settings"), copy)
        }
        #else
        XCTAssertEqual(LocationPermissionCopy.notAllowed, "Location access is not allowed")
        XCTAssertEqual(LocationPermissionCopy.cannotGuide,
                       "Location is off, so guidance can't start. In Settings, choose “While Using the App”.")
        XCTAssertEqual(LocationPermissionCopy.cannotRecord,
                       "Location is off. In Settings, choose “While Using the App” to record.")
        for copy in copies {
            XCTAssertFalse(copy.contains("System Settings"), copy)
        }
        #endif
        for copy in copies {
            XCTAssertTrue(copy.contains("Location"), copy)
        }
    }
}
