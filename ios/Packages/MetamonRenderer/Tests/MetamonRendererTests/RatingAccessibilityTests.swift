import XCTest
@testable import MetamonRenderer

/// #3061 rating の VoiceOver: 値の文字・ラベル・増減。
final class RatingAccessibilityTests: XCTestCase {
    private var saved: AppLanguage = .system
    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }
    override func tearDown() { LanguagePreference.shared.setting = saved }

    func testValueReadsStarsOutOfTotal() {
        XCTAssertEqual(RatingAccessibility.value(current: 3, count: 5), "星 3、全部で 5")
        XCTAssertEqual(RatingAccessibility.value(current: 0, count: 10), "星 0、全部で 10")
    }

    func testLabelFallsBackToRating() {
        XCTAssertEqual(RatingAccessibility.label("満足度"), "満足度")
        XCTAssertEqual(RatingAccessibility.label(""), "評価")
        XCTAssertEqual(RatingAccessibility.label(nil), "評価")
    }

    func testAdjustClampsAtEnds() {
        XCTAssertEqual(MacRatingKeys.adjusted(increment: true, current: 5, count: 5), 5)
        XCTAssertEqual(MacRatingKeys.adjusted(increment: false, current: 0, count: 5), 0)
        XCTAssertEqual(MacRatingKeys.adjusted(increment: true, current: 2, count: 5), 3)
    }
}
