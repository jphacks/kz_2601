import XCTest
@testable import MetamonRenderer

/// `chatThread` の入力欄はマイク FAB(左下)に重なる時だけ左を空ける(#260)。
final class ChatInputInsetTests: XCTestCase {
    private let bar = CGRect(x: 16, y: 700, width: 360, height: 56)

    func testNoFabNoInset() {
        XCTAssertEqual(ChatInputInset.leading(bar: bar, fab: nil), 0)
    }

    func testFabOverLeftOfBarPushesInput() {
        let fab = CGRect(x: 20, y: 690, width: 64, height: 64)
        XCTAssertEqual(ChatInputInset.leading(bar: bar, fab: fab), 84 - 16 + ChatInputInset.gap)
    }

    func testFabAboveOrRightIsIgnored() {
        XCTAssertEqual(ChatInputInset.leading(bar: bar, fab: CGRect(x: 20, y: 500, width: 64, height: 64)), 0)  // 上へ逃げた
        XCTAssertEqual(ChatInputInset.leading(bar: bar, fab: CGRect(x: 300, y: 690, width: 64, height: 64)), 0) // 右側
    }

    func testInsetNeverEatsMoreThanHalf() {
        let wide = CGRect(x: 0, y: 690, width: 190, height: 64)
        XCTAssertEqual(ChatInputInset.leading(bar: bar, fab: wide), bar.width / 2)
    }
}
