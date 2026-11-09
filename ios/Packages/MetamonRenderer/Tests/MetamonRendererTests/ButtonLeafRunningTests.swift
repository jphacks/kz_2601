import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// 通信を待つボタン(lookupBook)は、押した時点で同期に実行中にして二度押しを防ぐ(PR #720 のレビュー)。
final class ButtonLeafRunningTests: XCTestCase {
    func testWaitingActionBlocksSecondTapSynchronously() {
        var running = false
        let lookup = Action(type: .lookupBook)
        XCTAssertTrue(ButtonLeaf.begin(lookup, running: &running))
        XCTAssertTrue(running, "最初の await より前に実行中")
        XCTAssertFalse(ButtonLeaf.begin(lookup, running: &running), "二度押しは通さない")
    }

    func testOtherActionsDoNotHoldButton() {
        var running = false
        let save = Action(type: .saveRecord)
        XCTAssertTrue(ButtonLeaf.begin(save, running: &running))
        XCTAssertFalse(running)
        XCTAssertTrue(ButtonLeaf.begin(save, running: &running))
    }
}
