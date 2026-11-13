import XCTest
@testable import MetamonRenderer

/// #2257: 断られた権限の案内から開く設定の欄(macOS は権限ごと・iOS は今までどおりアプリの設定)。
final class PrivacySettingsPane2257Tests: XCTestCase {
    private let privacy = "x-apple.systempreferences:com.apple.preference.security?"

    func testMacOpensEachPrivacyPane() {
        XCTAssertEqual(PrivacySettingsPane.camera.url(on: .macOS).absoluteString, privacy + "Privacy_Camera")
        XCTAssertEqual(PrivacySettingsPane.microphone.url(on: .macOS).absoluteString, privacy + "Privacy_Microphone")
        XCTAssertEqual(PrivacySettingsPane.location.url(on: .macOS).absoluteString, privacy + "Privacy_LocationServices")
        XCTAssertEqual(PrivacySettingsPane.photos.url(on: .macOS).absoluteString, privacy + "Privacy_Photos")
        XCTAssertEqual(PrivacySettingsPane.speechRecognition.url(on: .macOS).absoluteString, privacy + "Privacy_SpeechRecognition")
        XCTAssertEqual(PrivacySettingsPane.calendars.url(on: .macOS).absoluteString, privacy + "Privacy_Calendars")
    }

    func testMacNotificationsOpenThisAppInNotificationSettings() {
        XCTAssertEqual(PrivacySettingsPane.notifications.url(on: .macOS, bundleIdentifier: "jp.kz2601.metamon").absoluteString,
                       "x-apple.systempreferences:com.apple.Notifications-Settings.extension?id=jp.kz2601.metamon")
    }

    func testIOSAlwaysOpensAppSettings() {
        for pane in PrivacySettingsPane.allCases {
            XCTAssertEqual(pane.url(on: .iOS).absoluteString, "app-settings:", "\(pane)")
        }
    }

    @MainActor
    func testPlatformSystemMatchesCurrentOS() {
        #if os(macOS)
        XCTAssertEqual(PlatformSystem.settingsURL(for: .photos)?.absoluteString, privacy + "Privacy_Photos")
        XCTAssertEqual(PlatformSystem.cameraSettingsURL?.absoluteString, privacy + "Privacy_Camera")
        XCTAssertEqual(LocationPermissionCopy.settingsURL?.absoluteString, privacy + "Privacy_LocationServices")
        #else
        XCTAssertEqual(PlatformSystem.settingsURL(for: .photos), PlatformSystem.appSettingsURL)
        XCTAssertEqual(PlatformSystem.cameraSettingsURL, PlatformSystem.appSettingsURL)
        XCTAssertEqual(PrivacySettingsPane.camera.url(on: .iOS), PlatformSystem.appSettingsURL)
        #endif
    }
}
