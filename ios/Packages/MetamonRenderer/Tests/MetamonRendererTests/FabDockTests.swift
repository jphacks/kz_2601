import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #52: マイク FAB のドラッグ吸着・隠す・右側の部品回避。
final class FabDockTests: XCTestCase {
    private let width: CGFloat = 390
    private let size: CGFloat = 60

    private func areas(_ specs: String...) -> [GridArea] { specs.map { GridArea($0)! } }

    // MARK: 右に吸着した時の部品回避

    func testTrailingClearance() {
        // 家計簿ホーム: 右下の「追加」(D11:F12)は右吸着の FAB に被る → 2 行ぶん上へ
        XCTAssertEqual(FabClearance.obstructedRows(areas("A1:F2", "A3:F10", "D11:F12"), side: .trailing), 2)
        // 左寄せのボタンは右吸着なら共存
        XCTAssertEqual(FabClearance.obstructedRows(areas("A11:D12"), side: .trailing), 0)
        XCTAssertEqual(FabClearance.obstructedRows(areas("A11:E12"), side: .trailing), 2)
        XCTAssertEqual(FabClearance.obstructedRows(areas("F12:F12"), side: .trailing), 1)
        // 全面 list は背景扱い
        XCTAssertEqual(FabClearance.obstructedRows(areas("A1:F12"), side: .trailing), 0)
        // 既定(左)は従来どおり
        XCTAssertEqual(FabClearance.obstructedRows(areas("D11:F12")), 0)
        XCTAssertEqual(FabClearance.obstructedRows(areas("D11:F12"), side: .leading), 0)
    }

    // MARK: 吸着

    func testSnapsToNearestSideByPredictedPosition() {
        XCTAssertEqual(FabDock.snap(centerX: 100, predictedCenterX: 120, width: width, fabSize: size), .init(side: .leading, hidden: false))
        XCTAssertEqual(FabDock.snap(centerX: 300, predictedCenterX: 280, width: width, fabSize: size), .init(side: .trailing, hidden: false))
        // 左半分で離しても右へ弾いたら右
        XCTAssertEqual(FabDock.snap(centerX: 180, predictedCenterX: 260, width: width, fabSize: size).side, .trailing)
    }

    // MARK: 隠す

    func testHidesNearEdge() {
        let threshold = size * FabDock.hideFraction // 18
        XCTAssertTrue(FabDock.snap(centerX: threshold, predictedCenterX: threshold, width: width, fabSize: size).hidden)
        XCTAssertFalse(FabDock.snap(centerX: threshold + 1, predictedCenterX: threshold + 1, width: width, fabSize: size).hidden)
        // 実位置は内側でも、端の外へ弾いたら隠す
        XCTAssertEqual(FabDock.snap(centerX: 60, predictedCenterX: -40, width: width, fabSize: size), .init(side: .leading, hidden: true))
        // 右端
        XCTAssertEqual(FabDock.snap(centerX: width - 10, predictedCenterX: width - 10, width: width, fabSize: size), .init(side: .trailing, hidden: true))
        XCTAssertEqual(FabDock.snap(centerX: width + 20, predictedCenterX: width + 80, width: width, fabSize: size), .init(side: .trailing, hidden: true))
    }

    func testRevealByPullingInward() {
        XCTAssertTrue(FabDock.shouldReveal(translationX: 20, side: .leading))
        XCTAssertFalse(FabDock.shouldReveal(translationX: 19, side: .leading))
        XCTAssertFalse(FabDock.shouldReveal(translationX: 40, side: .trailing), "右の取っ手を右へ引いても戻らない")
        XCTAssertTrue(FabDock.shouldReveal(translationX: -25, side: .trailing))
    }

    // MARK: 縦位置

    func testDisplayedBottom() {
        // 自動(-1)は従来の inset そのまま
        XCTAssertEqual(FabDock.displayedBottom(stored: FabDock.autoBottom, autoInset: 24, maxBottom: 600), 24)
        // ユーザー値は部品退避より下に置かない
        XCTAssertEqual(FabDock.displayedBottom(stored: 30, autoInset: 140, maxBottom: 600), 140)
        XCTAssertEqual(FabDock.displayedBottom(stored: 300, autoInset: 140, maxBottom: 600), 300)
        // ナビバーにかからない上限で止める
        XCTAssertEqual(FabDock.displayedBottom(stored: 900, autoInset: 24, maxBottom: 600), 600)
    }

    func testStoredBottomFromDrop() {
        // 高さ 700 の領域で中心 y=400 → 下端距離 700-400-30 = 270
        XCTAssertEqual(FabDock.storedBottom(centerY: 400, height: 700, fabSize: size, minBottom: 24, maxBottom: 580), 270)
        XCTAssertEqual(FabDock.storedBottom(centerY: 690, height: 700, fabSize: size, minBottom: 24, maxBottom: 580), 24)
        XCTAssertEqual(FabDock.storedBottom(centerY: -50, height: 700, fabSize: size, minBottom: 24, maxBottom: 580), 580)
    }
}
