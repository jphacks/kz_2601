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
        #if os(macOS)
        XCTAssertEqual(AIAvailability.unavailable(.osTooOld).message, "Requires macOS 26 or later")
        XCTAssertEqual(AIAvailability.unavailable(.appleIntelligenceNotEnabled).message,
                       "Turn it on in System Settings > Apple Intelligence & Siri")
        #else
        XCTAssertEqual(AIAvailability.unavailable(.osTooOld).message, "Requires iOS 26 or later")
        XCTAssertEqual(AIAvailability.unavailable(.appleIntelligenceNotEnabled).message, "Turn it on in Settings > Apple Intelligence & Siri")
        #endif
        XCTAssertEqual(AIGenerateBuilder.BuildError.noRecords.errorDescription, "Add a record first")
        XCTAssertEqual(OnDeviceAIError.failed("x", reason: .invalidResponse).errorDescription, "AI processing failed (x)")
    }

    func testMusicCopyIsEnglish() {
        XCTAssertEqual(MusicAvailability.noSubscription.message, "Playing songs needs an Apple Music subscription")
    }

    /// 数と名詞が合う(1 records にならない)。String Catalog の plural で 1 と 2 以上を分ける。
    func testPluralsMatchCount() {
        let cases: [(Int, String, String)] = [
            (1, L("\(1) 件"), "1 record"), (2, L("\(2) 件"), "2 records"),
            (1, L("記録\(1)件"), "1 record"), (2, L("記録\(2)件"), "2 records"),
            (1, L("\(1) 件を追加しました"), "Added 1 record"), (2, L("\(2) 件を追加しました"), "Added 2 records"),
            (1, L("\(1) 件を追加しました(\(3) 件は追加できませんでした)"), "Added 1 record (3 couldn't be added)"),
            (2, L("\(2) 件を追加しました(\(3) 件は追加できませんでした)"), "Added 2 records (3 couldn't be added)"),
            (1, L("\(1) 回"), "1 time"), (2, L("\(2) 回"), "2 times"),
            (1, L("\(1) 項目。\("3月") の \("12")"), "1 item. 3月: 12"),
            (2, L("\(2) 項目。\("3月") の \("12")"), "2 items. 3月: 12"),
            (1, L("\(1) 項目を入力欄に入れました。確認して保存してください"), "Filled 1 field. Check it and save."),
            (2, L("\(2) 項目を入力欄に入れました。確認して保存してください"), "Filled 2 fields. Check them and save."),
            (1, L("\(1)点を記録・あと\(5)分で自動停止"), "1 point · auto-stops in 5 min"),
            (2, L("\(2)点を記録・あと\(5)分で自動停止"), "2 points · auto-stops in 5 min"),
            (1, L("「\("a.txt")」から \(1) 文字を「\("Notes")」に 1 件追加します。"), "Add 1 record to \"Notes\" with 1 character from \"a.txt\"."),
            (2, L("「\("a.txt")」から \(2) 文字を「\("Notes")」に 1 件追加します。"), "Add 1 record to \"Notes\" with 2 characters from \"a.txt\"."),
            (1, L("「\("Notes")」に \(1) 件を追加します。"), "Add 1 record to \"Notes\"."),
            (2, L("「\("Notes")」に \(2) 件を追加します。"), "Add 2 records to \"Notes\"."),
            (1, L("上限(1 回 \(100) 件・合計 \(500) 件)を超える \(1) 行は入れません"), "1 row over the limit (100 per import, 500 in total) is skipped"),
            (2, L("上限(1 回 \(100) 件・合計 \(500) 件)を超える \(2) 行は入れません"), "2 rows over the limit (100 per import, 500 in total) are skipped"),
            (1, L("線 \(1) 本"), "1 stroke"), (2, L("線 \(2) 本"), "2 strokes"),
            (1, L("近くの端末を探して \(1) 件を送ります"), "Finds a nearby device and sends 1 record"),
            (2, L("近くの端末を探して \(2) 件を送ります"), "Finds a nearby device and sends 2 records"),
            (1, L("録音が長すぎます(\(1) 分まで)。分けた録音を選んでください"), "The recording is too long (up to 1 minute). Choose a shorter part"),
            (2, L("録音が長すぎます(\(2) 分まで)。分けた録音を選んでください"), "The recording is too long (up to 2 minutes). Choose a shorter part"),
            (1, L("長いので先頭の \(1) 文字だけ入れます。"), "It's long, so only the first 1 character is kept."),
            (2, L("長いので先頭の \(2) 文字だけ入れます。"), "It's long, so only the first 2 characters are kept."),
            (1, L("開始後に追加された \(1) 人は、この表に入りません"), "1 entrant added after the start is not in this bracket"),
            (2, L("開始後に追加された \(2) 人は、この表に入りません"), "2 entrants added after the start are not in this bracket"),
        ]
        for (n, actual, expected) in cases { XCTAssertEqual(actual, expected, "n=\(n)") }
    }
}
