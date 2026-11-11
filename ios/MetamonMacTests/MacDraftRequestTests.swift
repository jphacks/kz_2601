import XCTest
@testable import Metamon

/// β の案内の依頼の例を入力欄に入れる口(#1214)
@MainActor
final class MacDraftRequestTests: XCTestCase {
    func testSendIsTakenOnce() {
        let request = MacDraftRequest()
        let before = request.serial
        request.send("家計簿アプリを作って")
        XCTAssertEqual(request.serial, before + 1)
        XCTAssertEqual(request.take(), "家計簿アプリを作って")
        XCTAssertNil(request.take(), "2 枚目のウインドウは開かない")
    }

    func testSameTextTwiceStillNotifies() {
        let request = MacDraftRequest()
        request.send("a")
        _ = request.take()
        request.send("a")
        XCTAssertEqual(request.serial, 2)
        XCTAssertEqual(request.take(), "a")
    }
}
