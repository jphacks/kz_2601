import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// 固定文言が英語で出る(String Catalog に en がある)。抜けると日本語のキーのまま出る。
final class EnglishCopyTests: XCTestCase {
    private var savedLanguage: AppLanguage = .system

    override func setUp() {
        savedLanguage = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .en
    }

    override func tearDown() { LanguagePreference.shared.setting = savedLanguage }

    func testAIErrorsAreEnglish() {
        XCTAssertEqual(AIPhotoInput.PhotoError.noPhoto.errorDescription, "Take or choose a photo first")
        XCTAssertEqual(AIPhotoInput.PhotoError.unreadable.errorDescription, "Couldn't load the photo. Retake it and try again")
        XCTAssertEqual(AIAvailability.title, "AI isn't available on this device")
        XCTAssertEqual(AIAvailability.unavailable(.osTooOld).message, "Requires iOS 26 or later")
        XCTAssertEqual(AIGenerateBuilder.BuildError.noRecords.errorDescription, "Add a record first")
        XCTAssertEqual(OnDeviceAIError.failed("x", reason: .invalidResponse).errorDescription, "AI processing failed (x)")
    }

    func testMusicCopyIsEnglish() {
        XCTAssertEqual(MusicAvailability.noSubscription.message, "Playing songs needs an Apple Music subscription")
    }
}
