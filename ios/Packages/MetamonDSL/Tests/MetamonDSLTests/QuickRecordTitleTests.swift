import XCTest
@testable import MetamonDSL

/// QuickRecord の題の整え方(cleanTitle は private なので parse の title で確かめる)。
final class QuickRecordTitleTests: XCTestCase {
    private func title(_ raw: String) -> String? { QuickRecord.parse(raw).title }

    func testTrimsSurroundingWhitespace() {
        XCTAssertEqual(title("  ランチ  800円 "), "ランチ")
        XCTAssertEqual(title("\n散歩した\t"), "散歩した")
    }

    func testTrimsPunctuationAroundTitle() {
        XCTAssertEqual(title("、ランチ。 800"), "ランチ")
        XCTAssertEqual(title("ランチ,800円."), "ランチ")
    }

    func testDropsLeadingAndTrailingParticles() {
        XCTAssertEqual(title("ランチを 800円"), "ランチ")
        XCTAssertEqual(title("の ランチ 800円"), "ランチ")
        XCTAssertEqual(title("をにランチ 800円"), "ランチ", "助詞が続いても全部外す")
    }

    func testKeepsParticlesInsideTitle() {
        XCTAssertEqual(title("朝のランチ 800円"), "朝のランチ")
    }

    func testCollapsesInnerWhitespace() {
        XCTAssertEqual(title("ランチ   定食  800円"), "ランチ 定食")
    }

    func testFullwidthDigitsAndSpace() {
        XCTAssertEqual(title("ランチ　定食　８００円"), "ランチ 定食", "全角スペースは半角 1 つにまとまる")
    }

    func testEmptyAfterCleaningIsNil() {
        XCTAssertNil(title("800円"))
        XCTAssertNil(title("、。"))
        XCTAssertNil(title("を 800円 に"))
        XCTAssertNil(title("   "))
    }
}
