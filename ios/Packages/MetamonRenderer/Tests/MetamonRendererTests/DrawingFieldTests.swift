// TODO mac-026: UIKit を直接使うテスト。Mac で回すには描画の仕方を置き換え層に寄せる
#if os(iOS)
import XCTest
import PencilKit
import MetamonDSL
@testable import MetamonRenderer

/// drawingField: 描いた絵が写真と同じ形の画像になって photo field に入る・全部消す・下絵に描き足す。
final class DrawingFieldTests: XCTestCase {
    private let canvas = CGSize(width: 390, height: 600)

    /// (40,40) から (300,40) への太い横線 1 本。
    private func line(y: CGFloat = 40) -> PKDrawing {
        let points = stride(from: CGFloat(40), through: 300, by: 10).enumerated().map { i, x in
            PKStrokePoint(location: CGPoint(x: x, y: y), timeOffset: TimeInterval(i) * 0.01,
                          size: CGSize(width: 12, height: 12), opacity: 1, force: 1, azimuth: 0, altitude: .pi / 2)
        }
        let path = PKStrokePath(controlPoints: points, creationDate: Date())
        return PKDrawing(strokes: [PKStroke(ink: PKInk(.pen, color: .black), path: path)])
    }

    /// 画像の (x, y)(0...1 の割合)の明るさ 0...255。
    private func brightness(_ image: UIImage, atX fx: CGFloat, y fy: CGFloat) throws -> Int {
        let cg = try XCTUnwrap(image.cgImage)
        var px = [UInt8](repeating: 0, count: 4)
        let ctx = try XCTUnwrap(CGContext(data: &px, width: 1, height: 1, bitsPerComponent: 8, bytesPerRow: 4,
                                          space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue))
        let x = (CGFloat(cg.width) * fx).rounded(.down), y = (CGFloat(cg.height) * fy).rounded(.down)
        ctx.draw(cg, in: CGRect(x: -x, y: -(CGFloat(cg.height) - 1 - y), width: CGFloat(cg.width), height: CGFloat(cg.height)))
        return (Int(px[0]) + Int(px[1]) + Int(px[2])) / 3
    }

    func testStrokeBecomesPhotoValue() throws {
        let value = DrawingCodec.fieldValue(drawing: line(), background: nil, canvasSize: canvas)
        let s = try XCTUnwrap(value.stringValue)
        XCTAssertTrue(s.hasPrefix("data:image/jpeg;base64,"), "写真の field と同じ形")
        let image = try XCTUnwrap(PhotoCodec.image(from: value), "一覧・詳細の写真の表示で読める")
        XCTAssertLessThanOrEqual(max(image.size.width * image.scale, image.size.height * image.scale), 800)
        XCTAssertEqual(image.size.width / image.size.height, canvas.width / canvas.height, accuracy: 0.01, "キャンバスの縦横比のまま")
        XCTAssertLessThan(try brightness(image, atX: 170 / canvas.width, y: 40 / canvas.height), 100, "線が黒く残る")
        XCTAssertGreaterThan(try brightness(image, atX: 0.5, y: 0.8), 230, "白地(透明にしない)")
        XCTAssertLessThan(s.utf8.count, 300_000, "Firestore の 1 件 1MB に十分収まる")
    }

    /// ダークモードでもインクは黒・地は白で保存する。
    func testDarkModeStillSavesBlackOnWhite() throws {
        var value: JSONValue = .null
        UITraitCollection(userInterfaceStyle: .dark).performAsCurrent {
            value = DrawingCodec.fieldValue(drawing: line(), background: nil, canvasSize: canvas)
        }
        let image = try XCTUnwrap(PhotoCodec.image(from: value))
        XCTAssertLessThan(try brightness(image, atX: 170 / canvas.width, y: 40 / canvas.height), 100)
        XCTAssertGreaterThan(try brightness(image, atX: 0.5, y: 0.8), 230)
    }

    func testNothingDrawnIsNull() {
        XCTAssertEqual(DrawingCodec.fieldValue(drawing: PKDrawing(), background: nil, canvasSize: canvas), .null)
        XCTAssertEqual(DrawingCodec.fieldValue(drawing: line(), background: nil, canvasSize: .zero), .null)
    }

    @MainActor func testCommitAndClear() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"{"id":"sketch","name":"スケッチ","fields":[{"id":"art","name":"絵","type":"photo"}]}"#.utf8))
        let form = FormDraft(entity: entity)
        let session = DrawingSession(value: form.values["art"] ?? .null)
        XCTAssertTrue(session.isEmpty)
        session.drawing = line()
        session.commit(to: form, field: "art", canvasSize: canvas)
        let saved = try XCTUnwrap(form.values["art"]?.stringValue)
        XCTAssertTrue(saved.hasPrefix("data:image/jpeg;base64,"))

        // 開き直すと保存済みの絵が下絵になり、その上に描き足せる
        let again = DrawingSession(value: form.values["art"] ?? .null)
        XCTAssertNotNil(again.background)
        XCTAssertFalse(again.isEmpty)
        again.drawing = line(y: 500)
        again.commit(to: form, field: "art", canvasSize: canvas)
        let both = try XCTUnwrap(PhotoCodec.image(from: form.values["art"] ?? .null))
        XCTAssertLessThan(try brightness(both, atX: 170 / canvas.width, y: 40 / canvas.height), 120, "前の線が残る")
        XCTAssertLessThan(try brightness(both, atX: 170 / canvas.width, y: 500 / canvas.height), 120, "足した線")

        // 全部消す → 線も下絵も消え、完了で field が空になる
        let revision = again.revision
        again.clear()
        XCTAssertTrue(again.isEmpty)
        XCTAssertNil(again.background)
        XCTAssertEqual(again.revision, revision + 1, "キャンバスに空の絵を戻す合図")
        again.commit(to: form, field: "art", canvasSize: canvas)
        XCTAssertEqual(form.values["art"], JSONValue.null)
    }

    func testDrawingFieldIsKnownToBothBuilds() {
        XCTAssertEqual(ComponentType(rawValue: "drawingField"), .drawingField)
        XCTAssertTrue(ClientCapabilities.reviewAllowedComponents.contains("drawingField"), "審査用の版にも入れる(写真と同じ扱い)")
        XCTAssertTrue(RenderTree.formBound.contains(.drawingField))
    }
}
#endif
