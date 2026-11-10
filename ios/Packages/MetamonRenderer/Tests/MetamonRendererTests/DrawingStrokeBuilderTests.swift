import XCTest
import PencilKit
import MetamonDSL
@testable import MetamonRenderer

/// #1341: Mac のマウス・トラックパッドの点の列 → PKStroke → iOS と同じ JPEG の data URL。両方の OS で回す。
final class DrawingStrokeBuilderTests: XCTestCase {
    private let canvas = CGSize(width: 390, height: 600)

    /// (40,y) から (300,y) へドラッグした線。
    private func drag(y: CGFloat = 40) -> DrawingStrokeBuilder {
        var b = DrawingStrokeBuilder()
        for (i, x) in stride(from: CGFloat(40), through: 300, by: 10).enumerated() {
            b.add(CGPoint(x: x, y: y), time: 100 + TimeInterval(i) * 0.01)
        }
        return b
    }

    private func brightness(_ image: PlatformImage, atX fx: CGFloat, y fy: CGFloat) throws -> Int {
        let cg = try XCTUnwrap(image.cgImage)
        var px = [UInt8](repeating: 0, count: 4)
        let ctx = try XCTUnwrap(CGContext(data: &px, width: 1, height: 1, bitsPerComponent: 8, bytesPerRow: 4,
                                          space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue))
        let x = (CGFloat(cg.width) * fx).rounded(.down), y = (CGFloat(cg.height) * fy).rounded(.down)
        ctx.draw(cg, in: CGRect(x: -x, y: -(CGFloat(cg.height) - 1 - y), width: CGFloat(cg.width), height: CGFloat(cg.height)))
        return (Int(px[0]) + Int(px[1]) + Int(px[2])) / 3
    }

    func testPointsBecomePenStroke() throws {
        let b = drag()
        XCTAssertEqual(b.points.count, 27)
        XCTAssertEqual(b.points.first?.timeOffset ?? -1, 0, accuracy: 0.0001, "時刻は押した時から数える")
        XCTAssertEqual(b.points.last?.timeOffset ?? -1, 0.26, accuracy: 0.0001)
        XCTAssertEqual(b.points.first?.size.width, DrawingStrokeBuilder.penWidth, "iOS のペン(太さ 6)と同じ")
        let stroke = try XCTUnwrap(b.stroke())
        XCTAssertEqual(stroke.ink.inkType, .pen)
        XCTAssertEqual(stroke.path.count, 27)
        XCTAssertEqual(stroke.path.first?.location, CGPoint(x: 40, y: 40))
    }

    func testEmptyAndClick() throws {
        XCTAssertNil(DrawingStrokeBuilder().stroke(), "押していなければ線にしない")
        var click = DrawingStrokeBuilder()
        click.add(CGPoint(x: 100, y: 100), time: 5)
        XCTAssertEqual(try XCTUnwrap(click.stroke()).path.count, 2, "クリックだけでも点が残る")
    }

    func testPressureOnlyForTablet() {
        XCTAssertEqual(DrawingStrokeBuilder.width(pressure: nil), 6)
        XCTAssertEqual(DrawingStrokeBuilder.width(pressure: 1), 6)
        XCTAssertEqual(DrawingStrokeBuilder.width(pressure: 0), 3)
        XCTAssertEqual(DrawingStrokeBuilder.width(pressure: 5), 6, "範囲外は丸める")
    }

    /// 描いた線は iOS と同じ白地に黒の JPEG(data URL)になり、開き直すと下絵として描き足せる。
    @MainActor func testStrokesSaveLikeIOS() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"{"id":"sketch","name":"スケッチ","fields":[{"id":"art","name":"絵","type":"photo"}]}"#.utf8))
        let form = FormDraft(entity: entity)
        let session = DrawingSession(value: .null)
        XCTAssertEqual(session.accessibilityValue, L("何も描いていません"))
        session.add(try XCTUnwrap(drag().stroke()))
        XCTAssertEqual(session.drawing.strokes.count, 1)
        XCTAssertEqual(session.accessibilityValue, L("線 \(1) 本"))
        session.commit(to: form, field: "art", canvasSize: canvas)
        let saved = try XCTUnwrap(form.values["art"]?.stringValue)
        XCTAssertTrue(saved.hasPrefix("data:image/jpeg;base64,"), "iOS と同じ形")
        let image = try XCTUnwrap(PhotoCodec.image(from: form.values["art"] ?? .null))
        XCTAssertLessThanOrEqual(max(image.size.width * image.scale, image.size.height * image.scale), 800)
        XCTAssertLessThan(try brightness(image, atX: 170 / canvas.width, y: 40 / canvas.height), 120, "線が上に黒く残る")
        XCTAssertGreaterThan(try brightness(image, atX: 0.5, y: 0.8), 230, "白地")

        let again = DrawingSession(value: form.values["art"] ?? .null)
        XCTAssertNotNil(again.background)
        again.add(try XCTUnwrap(drag(y: 500).stroke()))
        again.commit(to: form, field: "art", canvasSize: canvas)
        let both = try XCTUnwrap(PhotoCodec.image(from: form.values["art"] ?? .null))
        XCTAssertLessThan(try brightness(both, atX: 170 / canvas.width, y: 40 / canvas.height), 140, "前の線が残る")
        XCTAssertLessThan(try brightness(both, atX: 170 / canvas.width, y: 500 / canvas.height), 120, "足した線")
    }
}
