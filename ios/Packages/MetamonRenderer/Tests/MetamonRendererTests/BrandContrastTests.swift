import XCTest
@testable import MetamonRenderer

/// #89: ブランド色を文字・白文字の下地に使う所が読めることを検算する(#68 の AA 方針に合わせる)。
/// - brandText: 本文の文字色として面に対し AA(4.5:1)は SemanticColorContrastTests が `all` 経由で検算
/// - brandFill: 白い 17pt semibold を載せる。両端とも 4.5:1 以上(#102 で 3:1 → 4.5:1 に引き上げ)
final class BrandContrastTests: XCTestCase {
    private let white = SemanticPalette.RGB(255, 255, 255)

    func testWhiteOnBrandFill() {
        // #102: 藍→青緑の塗りは両端とも本文サイズの AA(4.5:1)まで確保している
        XCTAssertGreaterThanOrEqual(white.contrast(with: SemanticPalette.brandFillStart), 4.5)
        XCTAssertGreaterThanOrEqual(white.contrast(with: SemanticPalette.brandFillEnd), 4.5)
    }

    func testBrandTextIsCoveredByPaletteTests() {
        XCTAssertTrue(SemanticPalette.all.contains { $0.name == "brand" })
    }

    func testBrandTintIsVisibleOnSurfaces() {
        // brandA は tint / フォーカスリング / 選択中のアイコン(非文字の UI 部品)なので面に対し 3:1 以上(WCAG 1.4.11)
        XCTAssertGreaterThanOrEqual(SemanticPalette.brandA.light.contrast(with: SemanticPalette.surfaceLight), 3.0)
        XCTAssertGreaterThanOrEqual(SemanticPalette.brandA.dark.contrast(with: SemanticPalette.surfaceDark), 3.0)
        XCTAssertGreaterThanOrEqual(SemanticPalette.brandA.light.contrast(with: SemanticPalette.canvas.light), 3.0)
        XCTAssertGreaterThanOrEqual(SemanticPalette.brandA.dark.contrast(with: SemanticPalette.canvas.dark), 3.0)
    }

    func testWhiteGlyphOnBrandGradient() {
        // 本体のマイク・送信・「新しいアプリ」は brandGradient の上に白いアイコン / 大きい白文字を載せる → 両端・両モードで 3:1 以上
        for c in [SemanticPalette.brandA.light, SemanticPalette.brandA.dark, SemanticPalette.brandB.light, SemanticPalette.brandB.dark] {
            XCTAssertGreaterThanOrEqual(white.contrast(with: c), 3.0)
        }
    }

    func testDecorativeBrandBIsNotForBodyText() {
        // 装飾用 brandB(青緑)は白文字の本文には使えない(だから brandFill を分けた)ことの記録
        XCTAssertLessThan(white.contrast(with: SemanticPalette.brandB.light), 4.5)
    }
}
