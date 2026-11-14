import XCTest
import SwiftUI
@testable import Metamon

/// 「透明度を下げる」「コントラストを上げる」の面と枠(#1206)と、表示メニューの文字の拡大 / 縮小の段。
final class MacAccessibilityStyleTests: XCTestCase {
    // MARK: - 面と枠

    func testStandardKeepsMaterialAndHairline() {
        let style = MacSurfaceStyle.resolve(reduceTransparency: false, contrast: .standard)
        XCTAssertEqual(style, .standard)
        XCTAssertEqual(style.fill, .material)
        XCTAssertEqual(style.border, .hairline)
        XCTAssertEqual(style.secondaryText, .secondary)
        XCTAssertEqual(style.borderWidth, 1)
    }

    func testReduceTransparencyUsesSolidFill() {
        let style = MacSurfaceStyle.resolve(reduceTransparency: true, contrast: .standard)
        XCTAssertEqual(style.fill, .solid)
        XCTAssertEqual(style.border, .hairline, "透明度だけでは枠は変えない")
        XCTAssertEqual(style.secondaryText, .secondary)
    }

    func testIncreasedContrastStrengthensBorderAndText() {
        let style = MacSurfaceStyle.resolve(reduceTransparency: false, contrast: .increased)
        XCTAssertEqual(style.fill, .material, "コントラストだけでは材質のまま")
        XCTAssertEqual(style.border, .strong)
        XCTAssertEqual(style.secondaryText, .primary)
        XCTAssertGreaterThan(style.borderWidth, MacSurfaceStyle.standard.borderWidth)
    }

    func testBothSettingsCombine() {
        let style = MacSurfaceStyle.resolve(reduceTransparency: true, contrast: .increased)
        XCTAssertEqual(style, MacSurfaceStyle(fill: .solid, border: .strong, secondaryText: .primary))
    }

    // MARK: - 文字の拡大 / 縮小

    func testStepsAreOrderedAndDefaultIsLarge() {
        XCTAssertEqual(MacTextZoom.steps, MacTextZoom.steps.sorted())
        XCTAssertEqual(MacTextZoom.size(at: MacTextZoom.defaultStep), .large)
        XCTAssertTrue(MacTextZoom.isActualSize(MacTextZoom.defaultStep))
    }

    func testZoomInAndOutMoveOneStep() {
        let start = MacTextZoom.defaultStep
        XCTAssertEqual(MacTextZoom.size(at: MacTextZoom.zoomedIn(start)), .xLarge)
        XCTAssertEqual(MacTextZoom.size(at: MacTextZoom.zoomedOut(start)), .medium)
        XCTAssertFalse(MacTextZoom.isActualSize(MacTextZoom.zoomedIn(start)))
    }

    func testBoundsDisableAndClamp() {
        let last = MacTextZoom.steps.count - 1
        XCTAssertFalse(MacTextZoom.canZoomIn(last))
        XCTAssertTrue(MacTextZoom.canZoomOut(last))
        XCTAssertEqual(MacTextZoom.zoomedIn(last), last)
        XCTAssertFalse(MacTextZoom.canZoomOut(0))
        XCTAssertTrue(MacTextZoom.canZoomIn(0))
        XCTAssertEqual(MacTextZoom.zoomedOut(0), 0)
        // 範囲外の値(古い・壊れた保存値)は端に寄せる
        XCTAssertEqual(MacTextZoom.size(at: 99), .accessibility3)
        XCTAssertEqual(MacTextZoom.size(at: -5), .xSmall)
        XCTAssertFalse(MacTextZoom.canZoomIn(99))
    }

    func testStepPersistsInDefaults() throws {
        let suite = "mac-text-zoom-test-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }

        XCTAssertEqual(MacTextZoom.step(in: defaults), MacTextZoom.defaultStep, "保存が無ければ実際のサイズ")
        MacTextZoom.setStep(MacTextZoom.zoomedIn(MacTextZoom.defaultStep), in: defaults)
        XCTAssertEqual(MacTextZoom.size(at: MacTextZoom.step(in: defaults)), .xLarge)
        // `@AppStorage` と同じ鍵に入る
        XCTAssertEqual(defaults.integer(forKey: MacTextZoom.storageKey), MacTextZoom.defaultStep + 1)
        MacTextZoom.setStep(42, in: defaults)
        XCTAssertEqual(MacTextZoom.step(in: defaults), MacTextZoom.steps.count - 1)
    }
}
