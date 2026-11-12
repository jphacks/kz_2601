import XCTest
import AppKit
@testable import Metamon

/// β の案内の依頼の例を入力欄に入れる口(#1214)
@MainActor
final class MacDraftRequestTests: XCTestCase {
    private func window() -> NSWindow {
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 100, height: 100), styleMask: [.titled], backing: .buffered, defer: true)
        window.isReleasedWhenClosed = false
        return window
    }

    func testSendIsTakenOnce() {
        let request = MacDraftRequest()
        let before = request.serial
        request.send("家計簿アプリを作って", to: nil)
        XCTAssertEqual(request.serial, before + 1)
        XCTAssertEqual(request.take(in: nil), "家計簿アプリを作って")
        XCTAssertNil(request.take(in: nil), "2 枚目のウインドウは開かない")
    }

    func testSameTextTwiceStillNotifies() {
        let request = MacDraftRequest()
        request.send("a", to: nil)
        _ = request.take(in: nil)
        request.send("a", to: nil)
        XCTAssertEqual(request.serial, 2)
        XCTAssertEqual(request.take(in: nil), "a")
    }

    func testOnlyTargetWindowTakesDraft() {
        let request = MacDraftRequest()
        let front = window()
        let back = window()
        request.send("家計簿アプリを作って", to: front)
        XCTAssertNil(request.take(in: back), "裏のウインドウでは開かない")
        XCTAssertNil(request.take(in: nil))
        XCTAssertEqual(request.take(in: front), "家計簿アプリを作って", "裏が先に見ても前のウインドウに残る")
        XCTAssertNil(request.take(in: front))
    }

    func testClosedTargetFallsBackToKeyWindowOnly() {
        let request = MacDraftRequest()
        autoreleasepool {
            request.send("a", to: window()) // 頼んだウインドウはすぐ閉じて無くなる
        }
        XCTAssertNil(request.take(in: window()), "キーでないウインドウでは開かない")
    }
}
