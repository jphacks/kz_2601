import XCTest
@testable import Metamon

/// サイドバーの検索(#1282)の絞り込み。
final class SampleAppSearchTests: XCTestCase {
    private let app = SampleApp(id: "kakeibo", name: "家計簿", description: "毎日の支出を記録する Budget", url: nil)

    func testEmptyQueryMatchesAll() {
        XCTAssertTrue(app.matches(""))
        XCTAssertTrue(app.matches("   "))
    }

    func testMatchesNameAndDescription() {
        XCTAssertTrue(app.matches("家計"))
        XCTAssertTrue(app.matches("支出"))
        XCTAssertFalse(app.matches("日記"))
    }

    func testIgnoresCaseAndRequiresEveryWord() {
        XCTAssertTrue(app.matches("budget"))
        XCTAssertTrue(app.matches("家計簿 支出"))
        XCTAssertFalse(app.matches("家計簿 日記"))
    }

    func testAppWithoutDescription() {
        let plain = SampleApp(id: "x", name: "ToDo", description: nil, url: nil)
        XCTAssertTrue(plain.matches("todo"))
        XCTAssertFalse(plain.matches("支出"))
    }
}
