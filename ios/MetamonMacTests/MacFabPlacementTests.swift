import XCTest
@testable import MetamonRenderer

/// 生成したアプリの FAB を中央の列の外の余白に置く試作(#2327): フラグの既定・余白の判定・置き場所。
final class MacFabPlacementTests: XCTestCase {
    private let inset = RenderTheme.Spacing.screenInset

    func testFlagIsOffByDefault() throws {
        let suite = "test-macFab-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        XCTAssertFalse(MacFabExperiment.isEnabled(defaults), "既定は FAB を出さない")
        defaults.set(true, forKey: MacFabExperiment.defaultsKey)
        XCTAssertTrue(MacFabExperiment.isEnabled(defaults))
    }

    func testOffKeepsDesktopMetricsUnchanged() {
        for size in [CGSize(width: 720, height: 480), CGSize(width: 1100, height: 720), CGSize(width: 1512, height: 945)] {
            let off = GridMetrics.resolveDesktop(width: size.width, height: size.height, inset: inset)
            XCTAssertFalse(off.fabInGutter, "フラグ off は今と同じ(\(size))")
            let on = GridMetrics.resolveDesktop(width: size.width, height: size.height, inset: inset, fabInGutter: true)
            XCTAssertEqual(on.frameWidth, off.frameWidth, "列の幅は変えない")
            XCTAssertEqual(on.cellH, off.cellH)
        }
    }

    /// 3 つの窓の大きさ: どれも余白に収まり、FAB は列の右・最後の部品に重ならない
    func testGutterAtThreeWindowSizes() {
        for size in [CGSize(width: 720, height: 480), CGSize(width: 1100, height: 720), CGSize(width: 1512, height: 945)] {
            let metrics = GridMetrics.resolveDesktop(width: size.width, height: size.height, inset: inset, fabInGutter: true)
            XCTAssertTrue(metrics.fabInGutter, "\(size)")
            let p = MacFabPlacement.placement(container: size, columnWidth: metrics.frameWidth, obstructionTop: nil, inset: inset)
            XCTAssertTrue(p.inGutter)
            let columnRight = (size.width + metrics.frameWidth) / 2
            XCTAssertGreaterThanOrEqual(p.frame.minX, columnRight, "列の外に置く(\(size))")
            XCTAssertLessThanOrEqual(p.frame.maxX, size.width, "窓からはみ出さない(\(size))")
            XCTAssertEqual(p.frame.maxY, size.height - MacFabPlacement.edge, accuracy: 0.5, "下に置く")
        }
    }

    /// 狭い窓は余白が無いのでグリッドの右下に戻し、最下段の右端の部品の上へ逃がす
    func testNarrowWindowFallsBackToGridCorner() {
        let size = CGSize(width: 560, height: 480)
        let metrics = GridMetrics.resolveDesktop(width: size.width, height: size.height, inset: inset, fabInGutter: true)
        XCTAssertFalse(metrics.fabInGutter)
        let plain = MacFabPlacement.placement(container: size, columnWidth: metrics.frameWidth, obstructionTop: nil, inset: inset)
        XCTAssertFalse(plain.inGutter)
        XCTAssertEqual(plain.frame.maxX, (size.width + metrics.frameWidth) / 2 - inset, accuracy: 0.5, "グリッドの右端に揃える")

        let raised = MacFabPlacement.placement(container: size, columnWidth: metrics.frameWidth, obstructionTop: 380, inset: inset)
        XCTAssertLessThanOrEqual(raised.frame.maxY, 380 - MacFabPlacement.gap, "最下段の部品に重ならない(#83)")
    }

    func testUnknownColumnWidthIsTreatedAsNoGutter() {
        let p = MacFabPlacement.placement(container: CGSize(width: 1100, height: 720), columnWidth: nil, obstructionTop: nil, inset: inset)
        XCTAssertFalse(p.inGutter, "列の幅が分かるまでは右下(重なりを避ける側)")
    }
}
