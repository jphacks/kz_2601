import XCTest
import PencilKit
import MetamonDSL
@testable import MetamonRenderer

/// #1342: Mac の消しゴム(線ごと消す)・取り消し / やり直しの順・iPhone との往復。両方の OS で回す。
final class DrawingEraserTests: XCTestCase {
    private let canvas = CGSize(width: 390, height: 600)

    /// (x0,y) から (x1,y) への横線。
    private func line(y: CGFloat, from x0: CGFloat = 40, to x1: CGFloat = 300) throws -> PKStroke {
        var b = DrawingStrokeBuilder()
        for (i, x) in stride(from: x0, through: x1, by: 10).enumerated() {
            b.add(CGPoint(x: x, y: y), time: TimeInterval(i) * 0.01)
        }
        return try XCTUnwrap(b.stroke())
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

    // MARK: - 当たり判定

    func testEraserRemovesOnlyTouchedStroke() throws {
        let drawing = PKDrawing(strokes: [try line(y: 40), try line(y: 200)])
        let erased = DrawingEraser.erase(drawing, from: CGPoint(x: 150, y: 190), to: CGPoint(x: 150, y: 210))
        XCTAssertEqual(erased.strokes.count, 1, "触れた線だけ 1 本ごと消える(iOS の .vector と同じ)")
        XCTAssertEqual(erased.strokes.first?.path.first?.location.y, 40)
    }

    func testEraserMissesFarStroke() throws {
        let drawing = PKDrawing(strokes: [try line(y: 40)])
        XCTAssertEqual(DrawingEraser.erase(drawing, from: CGPoint(x: 150, y: 80), to: CGPoint(x: 200, y: 80)).strokes.count, 1)
        XCTAssertEqual(DrawingEraser.erase(drawing, from: CGPoint(x: 340, y: 40), to: CGPoint(x: 340, y: 40)).strokes.count, 1,
                       "線の端より先は当たらない")
    }

    func testEraserHitsWithinRadiusAndOnFastSwipe() throws {
        let drawing = PKDrawing(strokes: [try line(y: 40)])
        // 線の太さの半分(3)+ 消しゴムの半径(6)より内側
        XCTAssertTrue(DrawingEraser.erase(drawing, from: CGPoint(x: 150, y: 48), to: CGPoint(x: 150, y: 48)).strokes.isEmpty,
                      "クリックだけでも近ければ消える")
        // 速く動かして点が線の上下に飛んでも、間の線分で当たる
        XCTAssertTrue(DrawingEraser.erase(drawing, from: CGPoint(x: 120, y: 0), to: CGPoint(x: 180, y: 120)).strokes.isEmpty)
    }

    func testEraserUsesStrokeTransform() throws {
        var moved = try line(y: 40)
        moved.transform = CGAffineTransform(translationX: 0, y: 300)
        let drawing = PKDrawing(strokes: [moved])
        XCTAssertEqual(DrawingEraser.erase(drawing, from: CGPoint(x: 150, y: 30), to: CGPoint(x: 150, y: 50)).strokes.count, 1)
        XCTAssertTrue(DrawingEraser.erase(drawing, from: CGPoint(x: 150, y: 330), to: CGPoint(x: 150, y: 350)).strokes.isEmpty)
    }

    func testSegmentDistance() {
        XCTAssertEqual(DrawingEraser.distance(CGPoint(x: 5, y: 3), toSegment: .zero, CGPoint(x: 10, y: 0)), 3, accuracy: 0.0001)
        XCTAssertEqual(DrawingEraser.distance(CGPoint(x: 13, y: 4), toSegment: .zero, CGPoint(x: 10, y: 0)), 5, accuracy: 0.0001)
        XCTAssertEqual(DrawingEraser.segmentDistance(.zero, CGPoint(x: 10, y: 10), CGPoint(x: 0, y: 10), CGPoint(x: 10, y: 0)), 0)
    }

    // MARK: - 取り消し / やり直し

    @MainActor func testUndoRedoOrder() throws {
        let undo = UndoManager()
        undo.groupsByEvent = false
        func step(_ body: () -> Void) { undo.beginUndoGrouping(); body(); undo.endUndoGrouping() }
        let session = DrawingSession(value: .null)
        XCTAssertFalse(undo.canUndo)

        step { session.add(try! line(y: 40), undo: undo) }
        step { session.add(try! line(y: 200), undo: undo) }
        // 消しゴムのドラッグ 1 回(何度か触れても取り消しは 1 回)
        step {
            let start = session.drawing
            XCTAssertTrue(session.erase(from: CGPoint(x: 100, y: 190), to: CGPoint(x: 100, y: 210)))
            XCTAssertFalse(session.erase(from: CGPoint(x: 100, y: 210), to: CGPoint(x: 120, y: 260)))
            session.finishErasing(from: start, undo: undo)
        }
        XCTAssertEqual(session.drawing.strokes.count, 1)

        undo.undo()
        XCTAssertEqual(session.drawing.strokes.count, 2, "消した線が戻る")
        undo.undo()
        XCTAssertEqual(session.drawing.strokes.count, 1, "2 本目が消える")
        XCTAssertTrue(undo.canRedo)
        undo.redo()
        XCTAssertEqual(session.drawing.strokes.count, 2, "2 本目をやり直す")
        undo.redo()
        XCTAssertEqual(session.drawing.strokes.count, 1, "消しゴムをやり直す")
        XCTAssertEqual(session.drawing.strokes.first?.path.first?.location.y, 40)
        XCTAssertFalse(undo.canRedo)

        // 新しく描くとやり直しは消える
        undo.undo()
        step { session.add(try! line(y: 400), undo: undo) }
        XCTAssertFalse(undo.canRedo)
        XCTAssertEqual(session.drawing.strokes.count, 3)
    }

    @MainActor func testEraseWithoutHitRegistersNothing() throws {
        let undo = UndoManager()
        undo.groupsByEvent = false
        let session = DrawingSession(value: .null)
        session.drawing = PKDrawing(strokes: [try line(y: 40)])
        let revision = session.revision
        // 開いたグループの外で登録すると例外になるので、積まれないことの確かめになる(アプリは groupsByEvent で登録時にだけグループが開く)
        let start = session.drawing
        XCTAssertFalse(session.erase(from: CGPoint(x: 10, y: 500), to: CGPoint(x: 20, y: 500)))
        session.finishErasing(from: start, undo: undo)
        XCTAssertFalse(undo.canUndo, "何も消えなければ取り消しを積まない")
        XCTAssertEqual(session.revision, revision)
    }

    // MARK: - iPhone との往復(DrawingCodec の data URL)

    /// iPhone(PKCanvasView の絵をそのまま session.drawing に入れる)で描いた絵を Mac で下絵にして描き足し・消しゴム → 保存。
    @MainActor func testIPhoneDrawingContinuesOnMac() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"{"id":"sketch","name":"スケッチ","fields":[{"id":"art","name":"絵","type":"photo"}]}"#.utf8))
        let form = FormDraft(entity: entity)
        let phone = DrawingSession(value: .null)
        phone.drawing = PKDrawing(strokes: [try line(y: 40)])
        phone.commit(to: form, field: "art", canvasSize: canvas)

        let mac = DrawingSession(value: form.values["art"] ?? .null)
        XCTAssertNotNil(mac.background, "iPhone の絵が Mac の下絵になる")
        mac.add(try line(y: 300))
        mac.add(try line(y: 500))
        XCTAssertTrue(mac.erase(from: CGPoint(x: 150, y: 490), to: CGPoint(x: 150, y: 510)), "Mac で足した線を消せる")
        XCTAssertEqual(mac.drawing.strokes.count, 1)
        mac.commit(to: form, field: "art", canvasSize: canvas)

        let saved = try XCTUnwrap(PhotoCodec.image(from: form.values["art"] ?? .null))
        XCTAssertLessThan(try brightness(saved, atX: 170 / canvas.width, y: 40 / canvas.height), 140, "iPhone の線が残る")
        XCTAssertLessThan(try brightness(saved, atX: 170 / canvas.width, y: 300 / canvas.height), 120, "Mac で描いた線")
        XCTAssertGreaterThan(try brightness(saved, atX: 170 / canvas.width, y: 500 / canvas.height), 230, "消した線は残らない")
    }

    /// Mac で描いた絵を iPhone で読み(PhotoCodec.image)、下絵にして続きを描いて保存できる。
    @MainActor func testMacDrawingContinuesOnIPhone() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"{"id":"sketch","name":"スケッチ","fields":[{"id":"art","name":"絵","type":"photo"}]}"#.utf8))
        let form = FormDraft(entity: entity)
        let mac = DrawingSession(value: .null)
        mac.add(try line(y: 40))
        mac.commit(to: form, field: "art", canvasSize: canvas)
        let value = form.values["art"] ?? .null
        XCTAssertNotNil(PhotoCodec.image(from: value), "iPhone の一覧・プレビューで出せる")

        let phone = DrawingSession(value: value)
        XCTAssertNotNil(phone.background)
        var drawing = phone.drawing
        drawing.strokes.append(try line(y: 500))
        phone.drawing = drawing // iPhone は PKCanvasView の絵をそのまま入れる
        phone.commit(to: form, field: "art", canvasSize: canvas)

        let saved = try XCTUnwrap(PhotoCodec.image(from: form.values["art"] ?? .null))
        XCTAssertLessThan(try brightness(saved, atX: 170 / canvas.width, y: 40 / canvas.height), 140, "Mac の線が残る")
        XCTAssertLessThan(try brightness(saved, atX: 170 / canvas.width, y: 500 / canvas.height), 120, "iPhone で足した線")
    }
}
