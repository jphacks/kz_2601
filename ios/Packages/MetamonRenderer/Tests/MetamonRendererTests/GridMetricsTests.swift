import XCTest
@testable import MetamonRenderer

/// #625: 横向きのグリッドの幅・行の高さ。
final class GridMetricsTests: XCTestCase {
    private let inset: CGFloat = 14

    func testPortraitFillsViewportLikeBefore() {
        let m = GridMetrics.resolve(width: 393, height: 680, inset: inset, compactHeight: false)
        XCTAssertEqual(m.frameWidth, 393)
        XCTAssertEqual(m.cellW, (393 - 28) / 6, accuracy: 0.001)
        XCTAssertEqual(m.cellH, (680 - 28) / 12, accuracy: 0.001)
        XCTAssertFalse(m.scrollsVertically)
        XCTAssertFalse(m.fabInGutter)
    }

    func testLandscapeLeavesFabGutterAndCentersReadableWidth() {
        // iPhone 15 横: セーフエリアの内側 734 × 330 前後
        let m = GridMetrics.resolve(width: 734, height: 330, inset: inset, compactHeight: true)
        XCTAssertEqual(m.frameWidth, 734 - GridMetrics.fabGutter * 2)
        XCTAssertTrue(m.fabInGutter)
        XCTAssertTrue(m.scrollsVertically)
        // 高さ ÷ 12(≒ 25pt)では潰れるので固定の行の高さ
        XCTAssertEqual(m.cellH, GridMetrics.landscapeCellHeight)
        XCTAssertEqual(m.cellW, (m.frameWidth - 28) / 6, accuracy: 0.001)
    }

    func testLandscapeCapsWidthOnLargePhones() {
        // Pro Max 横: 814 - 192 = 622 < 640 なので余白優先。もっと広い領域では最大幅で止まる
        XCTAssertEqual(GridMetrics.resolve(width: 814, height: 360, inset: inset, compactHeight: true).frameWidth, 622)
        let wide = GridMetrics.resolve(width: 1000, height: 360, inset: inset, compactHeight: true)
        XCTAssertEqual(wide.frameWidth, GridMetrics.maxLandscapeWidth)
        XCTAssertTrue(wide.fabInGutter)
    }

    func testLandscapeOnSmallPhoneDropsGutterForWidth() {
        // SE 横(667 幅): 余白を取ると 475 で足りるので余白を取る
        XCTAssertTrue(GridMetrics.resolve(width: 667, height: 320, inset: inset, compactHeight: true).fabInGutter)
        // さらに狭い領域では余白を諦め、幅いっぱい(FAB は末尾の余白で避ける)
        let narrow = GridMetrics.resolve(width: 560, height: 320, inset: inset, compactHeight: true)
        XCTAssertEqual(narrow.frameWidth, 560)
        XCTAssertFalse(narrow.fabInGutter)
        XCTAssertTrue(narrow.scrollsVertically)
    }

    func testLandscapeWithTallAreaKeepsProportionalRows() {
        let m = GridMetrics.resolve(width: 900, height: 1200, inset: inset, compactHeight: true)
        XCTAssertEqual(m.cellH, (1200 - 28) / 12, accuracy: 0.001)
    }

    // MARK: - Mac(#938)

    func testDesktopDefaultWindowCentersPortraitWidth() {
        // 1100 × 720 のウィンドウ(サイドバー 220): 中央の列 ≒ 880 × 668
        let m = GridMetrics.resolveDesktop(width: 880, height: 668, inset: inset)
        XCTAssertEqual(m.frameWidth, 501, accuracy: 0.001) // 668 × 3/4
        XCTAssertEqual(m.cellH, (668 - 28) / 12, accuracy: 0.001)
        XCTAssertFalse(m.scrollsVertically)
        XCTAssertFalse(m.fabInGutter)
    }

    func testDesktopFullScreenCapsWidth() {
        let m = GridMetrics.resolveDesktop(width: 1700, height: 1050, inset: inset)
        XCTAssertEqual(m.frameWidth, GridMetrics.maxDesktopWidth)
        XCTAssertEqual(m.cellW, (GridMetrics.maxDesktopWidth - 28) / 6, accuracy: 0.001)
    }

    func testDesktopLowWindowKeepsRowHeightAndScrolls() {
        let m = GridMetrics.resolveDesktop(width: 600, height: 400, inset: inset)
        XCTAssertEqual(m.frameWidth, GridMetrics.minDesktopWidth)
        XCTAssertEqual(m.cellH, GridMetrics.landscapeCellHeight)
        XCTAssertTrue(m.scrollsVertically)
    }

    func testDesktopNarrowColumnUsesFullWidth() {
        XCTAssertEqual(GridMetrics.resolveDesktop(width: 460, height: 700, inset: inset).frameWidth, 460)
    }
}
