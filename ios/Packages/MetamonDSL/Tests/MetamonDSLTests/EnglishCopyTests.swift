import XCTest
@testable import MetamonDSL

// キーがカタログにあるかの検査は MetamonServicesTests/EnglishCopyTests(DSL と Services の両方を見る)

/// 固定文言が英語で出る(String Catalog に en がある)。抜けると日本語のキーのまま出る。
final class EnglishCopyTests: XCTestCase {
    private var saved: String?
    override func setUp() { saved = setAppLanguage("en") }
    override func tearDown() { setAppLanguage(saved) }

    func testNearbyCopyIsEnglish() {
        XCTAssertEqual(NearbyPassby.directionText(azimuth: 0), "Ahead")
        XCTAssertEqual(NearbyPassby.directionText(azimuth: .pi / 2), "Right")
        XCTAssertEqual(ShareTarget.line.defaultLabel, "Send via LINE")
        XCTAssertEqual(AITextError.badOutput.errorDescription, "Couldn't read the AI's answer. Please try again")
        XCTAssertEqual(RecordExport.fileName("", ext: "csv"), "Export.csv")
    }

    func testJapaneseStaysJapanese() {
        setAppLanguage("ja")
        XCTAssertEqual(NearbyPassby.directionText(azimuth: 0), "正面")
        XCTAssertEqual(ShareTarget.any.defaultLabel, "共有")
    }
}

/// アプリ内の UI 言語(`PackageLocalization` が読む設定)を切り替え、前の値を返す。nil は端末の言語。
@discardableResult
func setAppLanguage(_ code: String?) -> String? {
    let old = UserDefaults.standard.string(forKey: PackageLocalization.storageKey)
    UserDefaults.standard.set(code, forKey: PackageLocalization.storageKey)
    return old
}
