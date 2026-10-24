import XCTest
@testable import MetamonRenderer

/// #157: 背の高い list の末尾に FAB ぶんの余白 / 1 行ぶんの area の stat を 1 段にする判定。
final class FabContentInsetTests: XCTestCase {
    /// iPhone 幅の全幅 list(y 100〜700)
    private let list = CGRect(x: 14, y: 100, width: 362, height: 600)
    private let gap = FabClearance.contentGap

    private func fab(x: CGFloat, y: CGFloat) -> CGRect { CGRect(x: x, y: y, width: 60, height: 60) }

    func testNoFabNoMargin() {
        XCTAssertEqual(FabClearance.contentMargin(viewport: list, fab: nil), 0)
    }

    func testFabOverListBottomAddsMarginUpToFabTop() {
        // 左下の FAB が list の下端に 40pt 食い込む → 40 + gap
        XCTAssertEqual(FabClearance.contentMargin(viewport: list, fab: fab(x: 24, y: 660)), 40 + gap)
        // 右に吸着していても横に重なれば同じ
        XCTAssertEqual(FabClearance.contentMargin(viewport: list, fab: fab(x: 306, y: 660)), 40 + gap)
    }

    func testFabBelowOrBesideListNoMargin() {
        // list より下(最下段のボタンの上に退避していない・list が行 10 まで)
        XCTAssertEqual(FabClearance.contentMargin(viewport: list, fab: fab(x: 24, y: 720)), 0)
        // 右半分の list(D 列〜)と左の FAB は横に重ならない
        let rightHalf = CGRect(x: 195, y: 100, width: 181, height: 600)
        XCTAssertEqual(FabClearance.contentMargin(viewport: rightHalf, fab: fab(x: 24, y: 660)), 0)
        // list より上
        XCTAssertEqual(FabClearance.contentMargin(viewport: list, fab: fab(x: 24, y: 20)), 0)
    }

    func testFabDraggedToMiddleLiftsUpToItsTop() {
        // FAB を list の途中(上端 400)へ動かした → 下端 700 から FAB の上端まで
        XCTAssertEqual(FabClearance.contentMargin(viewport: list, fab: fab(x: 24, y: 400)), 300 + gap)
    }

    func testEmptyViewportNoMargin() {
        // まだ測れていない器
        XCTAssertEqual(FabClearance.contentMargin(viewport: .zero, fab: fab(x: 24, y: 660)), 0)
    }

    func testStatSingleLineThreshold() {
        // 1 行ぶんの area(iPhone で 45pt 前後)は 1 段、2 行(100pt 前後)は従来の 2 段。未計測(0)は 2 段
        XCTAssertTrue(StatLeaf.usesSingleLine(height: 45))
        XCTAssertFalse(StatLeaf.usesSingleLine(height: 100))
        XCTAssertFalse(StatLeaf.usesSingleLine(height: 0))
    }
}
