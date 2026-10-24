import XCTest
@testable import MetamonRenderer

/// Issue #68: semantic color(red/green/orange/blue)が本文サイズの文字色として WCAG AA(4.5:1)を満たすことを検算する。
/// 面 = `RenderTheme.Colors.surface`(secondarySystemGroupedBackground: ライト白 / ダーク #1C1C1E)、
/// badge = 同色 15% のピルの上。どれかを崩すトーンに変えたら赤。
final class SemanticColorContrastTests: XCTestCase {
    private let aa = 4.5

    func testLightTonesAgainstSurface() {
        for c in SemanticPalette.all {
            XCTAssertGreaterThanOrEqual(c.light.contrast(with: SemanticPalette.surfaceLight), aa, "\(c.name) light vs surface")
        }
    }

    func testDarkTonesAgainstSurface() {
        for c in SemanticPalette.all {
            XCTAssertGreaterThanOrEqual(c.dark.contrast(with: SemanticPalette.surfaceDark), aa, "\(c.name) dark vs surface")
        }
    }

    func testTonesAgainstBadgePill() {
        for c in SemanticPalette.all {
            let pillLight = c.light.blended(over: SemanticPalette.surfaceLight, alpha: SemanticPalette.badgeFill)
            let pillDark = c.dark.blended(over: SemanticPalette.surfaceDark, alpha: SemanticPalette.badgeFill)
            XCTAssertGreaterThanOrEqual(c.light.contrast(with: pillLight), aa, "\(c.name) light vs badge pill")
            XCTAssertGreaterThanOrEqual(c.dark.contrast(with: pillDark), aa, "\(c.name) dark vs badge pill")
        }
    }

    func testContrastFormula() {
        // 白/黒 = 21:1、同色 = 1:1(WCAG 定義)
        XCTAssertEqual(SemanticPalette.RGB(255, 255, 255).contrast(with: SemanticPalette.RGB(0, 0, 0)), 21, accuracy: 0.01)
        XCTAssertEqual(SemanticPalette.surfaceLight.contrast(with: SemanticPalette.surfaceLight), 1, accuracy: 0.001)
        XCTAssertEqual(SemanticPalette.RGB(255, 255, 255).readableForeground, .black)
        XCTAssertEqual(SemanticPalette.RGB(0, 0, 0).readableForeground, .white)
    }
}
