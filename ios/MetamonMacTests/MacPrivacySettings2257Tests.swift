import XCTest
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// #2257: Mac の断りの案内(マイク・音声認識・通知・カレンダー)はその権限の欄を直接開く。
final class MacPrivacySettings2257Tests: XCTestCase {
    private let privacy = "x-apple.systempreferences:com.apple.preference.security?"

    func testVoiceFailuresOpenTheirPane() {
        XCTAssertEqual(MacVoiceCopy.privacyPane(for: .microphoneDenied)?.url.absoluteString, privacy + "Privacy_Microphone")
        XCTAssertEqual(MacVoiceCopy.privacyPane(for: .speechRecognitionDenied)?.url.absoluteString,
                       privacy + "Privacy_SpeechRecognition")
    }

    func testNotificationsOpenThisApp() {
        let bundleId = Bundle.main.bundleIdentifier ?? ""
        XCTAssertEqual(PrivacySettingsPane.notifications.url.absoluteString,
                       "x-apple.systempreferences:com.apple.Notifications-Settings.extension?id=\(bundleId)")
    }

    func testEveryPaneIsMacSpecific() {
        for pane in PrivacySettingsPane.allCases {
            XCTAssertTrue(pane.url.absoluteString.hasPrefix("x-apple.systempreferences:"), "\(pane)")
        }
        XCTAssertEqual(PrivacySettingsPane.photos.url.absoluteString, privacy + "Privacy_Photos")
        XCTAssertEqual(PrivacySettingsPane.calendars.url.absoluteString, privacy + "Privacy_Calendars")
    }
}
