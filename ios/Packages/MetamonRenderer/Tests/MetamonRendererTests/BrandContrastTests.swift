import XCTest
@testable import MetamonRenderer

/// #89: ブランド色を文字・白文字の下地に使う所が読めることを検算する(#68 の AA 方針に合わせる)。
/// - brandText: 本文の文字色として面に対し AA(4.5:1)は SemanticColorContrastTests が `all` 経由で検算
/// - brandFill: 白い 17pt semibold(large text)を載せるので両端とも 3:1 以上
final class BrandContrastTests: XCTestCase {
    private let white = SemanticPalette.RGB(255, 255, 255)

    func testWhiteOnBrandFill() {
        XCTAssertGreaterThanOrEqual(white.contrast(with: SemanticPalette.brandFillStart), 3.0)
        XCTAssertGreaterThanOrEqual(white.contrast(with: SemanticPalette.brandFillEnd), 3.0)
    }

    func testBrandTextIsCoveredByPaletteTests() {
        XCTAssertTrue(SemanticPalette.all.contains { $0.name == "brand" })
    }

    func testDecorativeBrandBIsNotForWhiteText() {
        // 装飾用 brandB(#4CC9FF)は白文字に使えない(だから brandFill を分けた)ことの記録
        XCTAssertLessThan(white.contrast(with: SemanticPalette.RGB(0x4C, 0xC9, 0xFF)), 3.0)
    }
}
