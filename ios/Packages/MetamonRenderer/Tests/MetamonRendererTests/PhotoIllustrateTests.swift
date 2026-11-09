import XCTest
import UIKit
import MetamonDSL
@testable import MetamonRenderer

/// photoField.illustrate: ボタンの出し分け・最初の案・作った絵の保存(Image Playground 自体はシミュレータで動かないので、返るファイルを偽物で代える)。
final class PhotoIllustrateTests: XCTestCase {
    private func entity(_ titleField: String? = "name") throws -> Entity {
        let tf = titleField.map { #""titleField":"\#($0)","# } ?? ""
        return try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"item","name":"項目",\#(tf)"fields":[
          {"id":"secret","name":"答え","type":"string","hidden":true},
          {"id":"memo","name":"メモ","type":"text"},
          {"id":"name","name":"名前","type":"string"},
          {"id":"n","name":"数","type":"integer"},
          {"id":"pic","name":"絵","type":"photo"}]}
        """#.utf8))
    }

    func testDecodesIllustrate() throws {
        let on = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"photoField","field":"pic","illustrate":true}"#.utf8))
        XCTAssertTrue(on.illustrate)
        let off = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"photoField","field":"pic"}"#.utf8))
        XCTAssertFalse(off.illustrate, "省略は false")
        let broken = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"photoField","field":"pic","illustrate":"yes"}"#.utf8))
        XCTAssertFalse(broken.illustrate, "壊れた値でも落ちず false")
    }

    func testButtonOnlyWhenRequestedAndAvailable() {
        XCTAssertTrue(PhotoIllustrate.showsButton(requested: true, available: true))
        XCTAssertFalse(PhotoIllustrate.showsButton(requested: true, available: false), "使えない端末では黙って出さない")
        XCTAssertFalse(PhotoIllustrate.showsButton(requested: false, available: true))
    }

    /// シミュレータには Apple Intelligence が無いので、ボタンは出ない(落ちない)。
    @MainActor func testUnavailableOnSimulator() {
        #if targetEnvironment(simulator)
        XCTAssertFalse(PhotoIllustrate.isAvailable)
        #endif
    }

    func testConceptPrefersTitleField() throws {
        let e = try entity()
        XCTAssertEqual(PhotoIllustrate.concept(entity: e, values: ["memo": .string("長いメモ"), "name": .string(" 猫のアイコン ")]), "猫のアイコン")
    }

    func testConceptFallsBackToFirstVisibleText() throws {
        let e = try entity(nil)
        XCTAssertEqual(PhotoIllustrate.concept(entity: e, values: ["secret": .string("答え"), "memo": .string("海の絵"), "name": .string("x")]), "海の絵", "hidden は使わない")
        XCTAssertEqual(PhotoIllustrate.concept(entity: try entity(), values: ["name": .string("  "), "memo": .string("山")]), "山", "空のタイトルは飛ばす")
        XCTAssertEqual(PhotoIllustrate.concept(entity: e, values: [:]), "")
    }

    func testConceptIsCapped() throws {
        let long = String(repeating: "あ", count: 500)
        XCTAssertEqual(PhotoIllustrate.concept(entity: try entity(), values: ["name": .string(long)]).count, PhotoIllustrate.maxConceptLength)
    }

    /// Image Playground が返すファイル(偽物の PNG)を、選んだ写真と同じ data URL(JPEG・800px 以下)にする。
    func testSavesGeneratedFileLikeAPickedPhoto() throws {
        let format = UIGraphicsImageRendererFormat.default()
        format.scale = 1
        let img = UIGraphicsImageRenderer(size: CGSize(width: 1024, height: 1024), format: format).image { ctx in
            UIColor.systemPink.setFill(); ctx.fill(CGRect(x: 0, y: 0, width: 1024, height: 1024))
        }
        let url = FileManager.default.temporaryDirectory.appendingPathComponent("illustrate-\(UUID().uuidString).png")
        try XCTUnwrap(img.pngData()).write(to: url)
        defer { try? FileManager.default.removeItem(at: url) }

        let encoded = try XCTUnwrap(PhotoIllustrate.dataURL(fromFileAt: url))
        XCTAssertTrue(encoded.hasPrefix("data:image/jpeg;base64,"))
        let decoded = try XCTUnwrap(PhotoCodec.image(from: .string(encoded)))
        XCTAssertLessThanOrEqual(decoded.size.width * decoded.scale, 801)
        XCTAssertNil(PhotoIllustrate.dataURL(fromFileAt: url.appendingPathExtension("missing")), "読めないファイルは何も入れない")
    }
}
