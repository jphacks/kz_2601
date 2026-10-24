import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #140: progressBar の tone(目標 = positive / 予算 = negative)で 100% の色を変える。
final class ProgressBarToneTests: XCTestCase {
    func testLevels() {
        typealias L = ProgressBarLeaf
        // 省略時は従来どおり 100% 以上で警告色
        XCTAssertEqual(L.level(raw: 0.5, tone: nil), .normal)
        XCTAssertEqual(L.level(raw: 1.0, tone: nil), .over)
        // 目標の達成: 100% 以上は成功色
        XCTAssertEqual(L.level(raw: 0.99, tone: "positive"), .normal)
        XCTAssertEqual(L.level(raw: 1.0, tone: "positive"), .success)
        XCTAssertEqual(L.level(raw: 1.4, tone: "positive"), .success)
        // 予算の消化: ちょうど使い切りは通常、超過で警告色
        XCTAssertEqual(L.level(raw: 1.0, tone: "negative"), .normal)
        XCTAssertEqual(L.level(raw: 1.12, tone: "negative"), .over)
    }

    func testToneDecodes() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"progressBar","value":0.5,"tone":"positive"}"#.utf8))
        XCTAssertEqual(c.tone, "positive")
    }
}
