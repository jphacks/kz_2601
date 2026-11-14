import XCTest
@testable import Metamon

/// 通知からの切り替えをシートの後に回す(#943 `Shared/Notifications/SheetDeferredSwitch.swift`・Mac が使う)。
final class SheetDeferredSwitchTests: XCTestCase {
    func testSwitchesNowWithoutSheet() {
        var queue = SheetDeferredSwitch()
        XCTAssertEqual(queue.request("memo", sheetOpen: false), "memo")
        XCTAssertNil(queue.pending)
        XCTAssertNil(queue.sheetClosed())
    }

    func testWaitsForSheetToClose() {
        var queue = SheetDeferredSwitch()
        XCTAssertNil(queue.request("memo", sheetOpen: true))
        XCTAssertEqual(queue.sheetClosed(), "memo")
        XCTAssertNil(queue.sheetClosed()) // 1 回だけ
    }

    /// シートの間に何度押されても、最後に押したアプリへ 1 回だけ
    func testKeepsLatestRequest() {
        var queue = SheetDeferredSwitch()
        _ = queue.request("memo", sheetOpen: true)
        _ = queue.request("habit", sheetOpen: true)
        XCTAssertEqual(queue.sheetClosed(), "habit")
    }

    /// 閉じた時にまだ別のシートが開いていれば待ち続ける
    func testKeepsWaitingWhileAnotherSheetIsOpen() {
        var queue = SheetDeferredSwitch()
        _ = queue.request("memo", sheetOpen: true)
        XCTAssertNil(queue.sheetClosed(sheetOpen: true))
        XCTAssertEqual(queue.sheetClosed(sheetOpen: false), "memo")
    }

    /// シートが無い時に押したら、前に待っていた分は捨てる(後から古い方へ戻らない)
    func testImmediateRequestDropsStalePending() {
        var queue = SheetDeferredSwitch()
        _ = queue.request("memo", sheetOpen: true)
        XCTAssertEqual(queue.request("habit", sheetOpen: false), "habit")
        XCTAssertNil(queue.sheetClosed())
    }
}
