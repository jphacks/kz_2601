#if !APP_STORE_REVIEW
import XCTest
import MetamonServices
@testable import Metamon

/// 中央のアプリの部品が求めた課金画面は、頼んだ時に前面だったウインドウ 1 つにだけ出す
@MainActor
final class MacPaywallRoutingTests: XCTestCase {
    private func pro(front: [Int], presenters: [Int]) -> MacPro {
        let pro = MacPro(store: ProEntitlementStore(mode: .mock))
        pro.frontWindows = { front }
        presenters.forEach { pro.registerPresenter(window: $0) }
        return pro
    }

    func testShowsOnlyInKeyWindow() {
        let pro = pro(front: [2, 1, 3], presenters: [1, 2, 3])
        pro.requestPro(.paidFeature([]))
        XCTAssertNil(pro.paywall(for: 1))
        XCTAssertNotNil(pro.paywall(for: 2), "前面のウインドウに出す")
        XCTAssertNil(pro.paywall(for: 3))
        XCTAssertNil(pro.paywall(for: nil))
    }

    func testSkipsWindowsWithoutPresenter() {
        // 前面が設定のウインドウ(課金画面を出さない)なら、その次に前のメインのウインドウ
        let pro = pro(front: [9, 3, 1], presenters: [1, 3])
        pro.requestPro(.planLimit)
        XCTAssertEqual(pro.paywall?.window, 3)
    }

    func testFallsBackToFirstPresenter() {
        let pro = pro(front: [], presenters: [4, 5])
        pro.requestPro(.planLimit)
        XCTAssertEqual(pro.paywall?.window, 4)
    }

    func testUnroutedRequestGoesToFirstWindowThatAppears() {
        let pro = pro(front: [], presenters: [])
        pro.requestPro(.planLimit)
        XCTAssertNil(pro.paywall?.window)
        pro.registerPresenter(window: 7)
        pro.registerPresenter(window: 8)
        XCTAssertNotNil(pro.paywall(for: 7))
        XCTAssertNil(pro.paywall(for: 8))
    }

    /// 出しているのは 1 つだけなので、ほかのウインドウを閉じても残る。出していたウインドウを閉じたら下げる
    func testClosingWindows() {
        let pro = pro(front: [1], presenters: [1, 2])
        pro.requestPro(.planLimit)
        pro.unregisterPresenter(window: 2)
        XCTAssertNotNil(pro.paywall(for: 1))
        pro.unregisterPresenter(window: 1)
        XCTAssertNil(pro.paywall)
        XCTAssertEqual(pro.presenterWindows, [])
    }

    func testTarget() {
        XCTAssertEqual(MacPro.target(front: [3, 2], presenters: [1, 2, 3]), 3)
        XCTAssertEqual(MacPro.target(front: [9], presenters: [1, 2]), 1)
        XCTAssertNil(MacPro.target(front: [1], presenters: []))
    }
}
#endif
