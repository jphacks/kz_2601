import XCTest
import SwiftUI
import CoreImage
import MetamonDSL
@testable import MetamonRenderer

/// `qrCode`(#422): 生成(誤り訂正 M・整数倍で拡大・余白)・qrScanner と同じ CoreImage で読み戻せること・空 / 長すぎ・
/// 値の出どころ(text / {{}} / field / computed / value)・描画(空で落ちない・描いた画像からも読める)。
final class QRCodeTests: XCTestCase {
    private func decode(_ image: UIImage) -> [String] {
        guard let cg = image.cgImage,
              let detector = CIDetector(ofType: CIDetectorTypeQRCode, context: nil, options: [CIDetectorAccuracy: CIDetectorAccuracyHigh])
        else { return [] }
        return detector.features(in: CIImage(cgImage: cg)).compactMap { ($0 as? CIQRCodeFeature)?.messageString }
    }

    private func component(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    private func member() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"card","name":"会員証","fields":[
          {"id":"name","name":"名前","type":"string"},
          {"id":"number","name":"会員番号","type":"string"},
          {"id":"points","name":"ポイント","type":"integer"},
          {"id":"code","name":"コード","type":"string","computed":{"op":"concat","args":["M-",{"ref":"number"}]}}
        ]}
        """#.utf8))
    }

    func testRoundTripDecodes() throws {
        for text in ["https://example.com/members?id=00123", "会員番号 00123 / 山田 花子", String(repeating: "abc123", count: 100)] {
            let img = try XCTUnwrap(QRCodeImage.image(for: text), "生成できない: \(text.prefix(20))")
            XCTAssertEqual(decode(img), [text], "読み戻せない: \(text.prefix(20))")
        }
    }

    func testCrispIntegerScaleWithQuietZone() throws {
        let cg = try XCTUnwrap(QRCodeImage.modules(for: "MEMBER:00123"))
        XCTAssertEqual(cg.width, cg.height)
        let img = try XCTUnwrap(QRCodeImage.image(for: "MEMBER:00123", scale: 8))
        XCTAssertEqual(img.scale, 1)
        XCTAssertEqual(Int(img.size.width), (cg.width + QRCodeImage.quietZone * 2) * 8, "1 モジュール = 8px の整数倍")

        // 画素を読む: 余白は白、ファインダーパターンの角は黒、モジュール内は単色(補間のぼけが無い)
        let side = Int(img.size.width)
        var px = [UInt8](repeating: 0, count: side * side)
        let ctx = try XCTUnwrap(CGContext(data: &px, width: side, height: side, bitsPerComponent: 8, bytesPerRow: side,
                                          space: CGColorSpaceCreateDeviceGray(), bitmapInfo: CGImageAlphaInfo.none.rawValue))
        ctx.draw(try XCTUnwrap(img.cgImage), in: CGRect(x: 0, y: 0, width: side, height: side))
        let at = { (x: Int, y: Int) in px[y * side + x] }
        XCTAssertEqual(at(2, 2), 255, "余白が白くない")
        // CIQRCodeGenerator 自体の縁を含めても、余白の内側 1 モジュール目のどこかから黒が始まる
        let firstDark = (0..<side).first { at($0, side / 2) < 128 }
        XCTAssertNotNil(firstDark)
        XCTAssertTrue(Set(px).isSubset(of: [0, 255]), "灰色の画素がある(拡大で補間された)")
    }

    func testExportSizeAndEmptyOrTooLong() throws {
        let img = try XCTUnwrap(QRCodeImage.image(for: "https://example.com"))
        XCTAssertGreaterThanOrEqual(img.size.width, 700)
        XCTAssertLessThanOrEqual(img.size.width, 1100)
        XCTAssertNil(QRCodeImage.modules(for: ""))
        XCTAssertNil(QRCodeImage.image(for: ""))
        XCTAssertNil(QRCodeImage.image(for: String(repeating: "あ", count: 1000)), "3000 バイトはバージョン 40・M に入らない")
    }

    @MainActor func testContentSources() throws {
        let entity = try member()
        let form = FormDraft(entity: entity)
        form.values["name"] = "山田"
        form.values["number"] = "00123"
        form.values["points"] = 120
        let ev = { ExprEvaluator(entities: [entity], records: { _ in [] }) }

        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode","text":"https://example.com"}"#), form: nil, evaluator: ev), "https://example.com")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode","text":"https://x.com/{{number}}?p={{points}}"}"#), form: form, evaluator: ev), "https://x.com/00123?p=120")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode","field":"number"}"#), form: form, evaluator: ev), "00123")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode","field":"points"}"#), form: form, evaluator: ev), "120")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode","field":"code"}"#), form: form, evaluator: ev), "M-00123", "computed は評価する")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode","value":{"op":"concat","args":["NAME:",{"ref":"name"}]}}"#), form: form, evaluator: ev), "NAME:山田")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode","value":"固定"}"#), form: nil, evaluator: ev), "固定")
        // 空: フォーム外の field・未入力・式が評価できない
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode","field":"number"}"#), form: nil, evaluator: ev), "")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode","field":"number"}"#), form: FormDraft(entity: entity), evaluator: ev), "")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode","value":{"ref":"name"}}"#), form: nil, evaluator: ev), "")
        XCTAssertEqual(QRCodeContent.resolve(try component(#"{"type":"qrCode"}"#), form: nil, evaluator: ev), "")
    }

    /// 描画: 空・長すぎは案内(落ちない)、値があれば描いた画面の QR を CIDetector で読める。
    @MainActor func testRenderEmptyAndFilled() throws {
        let entity = try member()
        let form = FormDraft(entity: entity)
        let c = try component(#"{"type":"qrCode","field":"number","label":"受付で見せてください","title":"会員証"}"#)
        let ctx = RenderContext(route: Route(screen: "card_form"), form: form)

        func render() throws -> UIImage {
            let r = ImageRenderer(content: QrCodeLeaf(c: c, ctx: ctx).frame(width: 320, height: 480).background(Color.white))
            r.scale = 2
            return try XCTUnwrap(r.uiImage, "描画できない")
        }
        XCTAssertEqual(decode(try render()), [], "空なのに QR が出た")

        form.values["number"] = "00123"
        XCTAssertEqual(decode(try render()), ["00123"], "描いた QR が読めない")

        form.values["number"] = .string(String(repeating: "あ", count: 1000))
        XCTAssertEqual(decode(try render()), [], "長すぎる値で QR が出た")

        // レジストリ経由でも未知 type のプレースホルダにならない
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "qrCode"))
    }
}
