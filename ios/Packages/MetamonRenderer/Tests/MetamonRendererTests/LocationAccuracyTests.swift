import XCTest
@testable import MetamonRenderer

/// 現在地の精度の分け方(#1344): しきい値・不明な値・一言の文言。
final class LocationAccuracyTests: XCTestCase {
    private var savedLanguage: AppLanguage = .system

    override func setUp() {
        savedLanguage = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = savedLanguage }

    func testClassifyByThreshold() {
        XCTAssertEqual(LocationAccuracy.classify(5), .precise)
        XCTAssertEqual(LocationAccuracy.classify(LocationAccuracy.approximateThreshold), .precise, "しきい値ちょうどは正確")
        XCTAssertEqual(LocationAccuracy.classify(1_200), .approximate(meters: 1_200))
    }

    func testUnknownAccuracyIsNotApproximate() {
        XCTAssertEqual(LocationAccuracy.classify(-1), .precise, "負の値(不明)で「おおよそ」と言わない")
        XCTAssertEqual(LocationAccuracy.classify(.infinity), .precise)
        XCTAssertEqual(LocationAccuracy.classify(.nan), .precise)
    }

    func testNotes() {
        XCTAssertNil(LocationAccuracy.precise.note)
        XCTAssertNil(LocationAccuracy.precise.pickerNote)
        XCTAssertNil(LocationAccuracy.precise.searchNote)
        let approx = LocationAccuracy.classify(1_234)
        XCTAssertEqual(approx.note, "おおよその位置(±1.2 km)")
        XCTAssertEqual(approx.pickerNote, "おおよその位置(±1.2 km)です。地図を動かしてピンで直せます")
        XCTAssertEqual(approx.searchNote, "おおよその位置(±1.2 km)から探しました。距離は目安です")
        XCTAssertEqual(LocationAccuracy.classify(347).note, "おおよその位置(±350 m)")
    }

    func testNotesInEnglish() {
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(LocationAccuracy.classify(1_234).note, "Approximate location (±1.2 km)")
    }
}
