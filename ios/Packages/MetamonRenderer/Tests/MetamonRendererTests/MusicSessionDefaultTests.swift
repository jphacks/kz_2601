import XCTest
@testable import MetamonRenderer

/// `musicPlayer`(#412・Mac は #960): 既定の再生元は iOS・Mac とも MusicKit。偽物は渡した時だけ。
@MainActor
final class MusicSessionDefaultTests: XCTestCase {
    func testDefaultPlayerIsMusicKit() {
        XCTAssertTrue(MusicSession().player is AppleMusicPlayback)
    }

    func testInjectedPlayerIsKept() {
        XCTAssertTrue(MusicSession(player: FakeMusicPlayback()).player is FakeMusicPlayback)
    }

    func testDeniedMessagePointsToThePlatformSettings() throws {
        let message = try XCTUnwrap(MusicAvailability.denied.message)
        #if os(macOS)
        XCTAssertTrue(message.contains("System Settings") || message.contains("システム設定"), message)
        #else
        XCTAssertFalse(message.contains("System Settings") || message.contains("システム設定"), message)
        #endif
    }

    /// #1377: Mac で断られた時だけ「メディアと Apple Music」の画面を開ける。iOS は出さない。
    func testSettingsURLOnlyForDeniedOnMac() {
        #if os(macOS)
        XCTAssertEqual(MusicAvailability.denied.settingsURL?.absoluteString,
                       "x-apple.systempreferences:com.apple.preference.security?Privacy_Media")
        #else
        XCTAssertNil(MusicAvailability.denied.settingsURL)
        #endif
        for a in [MusicAvailability.notDetermined, .ready, .restricted, .noSubscription, .unavailable] {
            XCTAssertNil(a.settingsURL)
        }
    }
}
