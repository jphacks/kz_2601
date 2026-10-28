import XCTest
import UIKit
import MetamonDSL
@testable import MetamonRenderer

/// #418 photoField.analyze: OCR の行まとめ、テスト内で描いたレシート画像の読み取り、骨格の重ね描きと角度の座標。
/// 画像はテストの中で文字を描いて作る(同梱の画像・外部の写真は使わない)。
final class PhotoAnalyzeTests: XCTestCase {
    private func example(_ name: String) throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/\(name).json"))
    }

    /// 白地に黒文字のレシート(1 行ずつ・右に金額)。
    private func receiptImage(_ rows: [(String, String)]) -> UIImage {
        let format = UIGraphicsImageRendererFormat.default()
        format.scale = 1
        format.opaque = true
        let size = CGSize(width: 640, height: 80 + rows.count * 60)
        return UIGraphicsImageRenderer(size: size, format: format).image { ctx in
            UIColor.white.setFill()
            ctx.fill(CGRect(origin: .zero, size: size))
            let attrs: [NSAttributedString.Key: Any] = [.font: UIFont.systemFont(ofSize: 34, weight: .medium), .foregroundColor: UIColor.black]
            for (i, row) in rows.enumerated() {
                let y = CGFloat(40 + i * 60)
                (row.0 as NSString).draw(at: CGPoint(x: 40, y: y), withAttributes: attrs)
                (row.1 as NSString).draw(at: CGPoint(x: 400, y: y), withAttributes: attrs)
            }
        }
    }

    // MARK: OCR の行

    func testPiecesOnTheSameRowAreJoinedLeftToRight() {
        let pieces = [
            OCRPiece(text: "¥493", box: CGRect(x: 0.6, y: 0.30, width: 0.2, height: 0.05)),
            OCRPiece(text: "合計", box: CGRect(x: 0.1, y: 0.31, width: 0.2, height: 0.05)),
            OCRPiece(text: "店名", box: CGRect(x: 0.1, y: 0.90, width: 0.3, height: 0.05)),
            OCRPiece(text: "牛乳", box: CGRect(x: 0.1, y: 0.50, width: 0.2, height: 0.05)),
        ]
        XCTAssertEqual(OCRPiece.lines(pieces), ["店名", "牛乳", "合計 ¥493"])
    }

    /// 描いたレシートから金額・日付・文字を読み、into の型(currency / date / text)で入れる。
    func testReadsRenderedReceipt() async throws {
        let dsl = try example("receipt_ai")
        let entity = try XCTUnwrap(dsl.entity("expense"))
        let image = receiptImage([("TAILOR MART", ""), ("2026/10/28", "18:42"), ("Milk", "¥198"), ("Bread", "¥258"), ("TOTAL", "¥493")])
        let specs = [PhotoAnalyze(kind: .amount, into: "amount"), PhotoAnalyze(kind: .date, into: "date"), PhotoAnalyze(kind: .text, into: "memo")]

        let out = await PhotoReader.read(image, specs: specs, entity: entity)
        XCTAssertEqual(out.values["amount"], .number(493), "合計の行の金額: \(out)")
        XCTAssertEqual(out.values["date"], .string("2026-10-28"))
        XCTAssertTrue(out.values["memo"]?.stringValue?.contains("Milk") ?? false)
        XCTAssertEqual(out.missed, [])
        XCTAssertNil(out.pose, "jointAngle が無ければ骨格は探さない")
    }

    func testJapaneseReceiptText() async throws {
        let image = receiptImage([("領収書", ""), ("牛乳", "¥198"), ("合計", "¥493")])
        let lines = PhotoVision.textLines(in: try XCTUnwrap(image.cgImage))
        XCTAssertTrue(lines.contains { $0.contains("合計") }, "日本語の OCR: \(lines)")
        XCTAssertEqual(ReceiptParser.amount(lines: lines), 493)
    }

    /// 文字の無い写真は読めなかった項目として返し、値は入れない(今の値のまま)。
    func testBlankPhotoMissesEverything() async throws {
        let dsl = try example("receipt_ai")
        let entity = try XCTUnwrap(dsl.entity("expense"))
        let out = await PhotoReader.read(receiptImage([]), specs: [PhotoAnalyze(kind: .amount, into: "amount"), PhotoAnalyze(kind: .date, into: "date")], entity: entity)
        XCTAssertTrue(out.values.isEmpty)
        XCTAssertEqual(out.missed, ["amount", "date"])
    }

    // MARK: 骨格

    /// 人の写っていない写真では骨格は無し(落ちない)。
    func testNoBodyInBlankImage() throws {
        XCTAssertNil(PhotoVision.bodyPose(in: try XCTUnwrap(receiptImage([("TEXT", "")]).cgImage)))
    }

    /// 関節の点を差し込んで、jointAngle が into の number に入る(Vision を通さずに角度の経路を確かめる)。
    func testJointAngleFromInjectedPose() async throws {
        let dsl = try example("form_check")
        let entity = try XCTUnwrap(dsl.entity("check"))
        let knee = try XCTUnwrap(entity.field("knee_angle"))
        let squat: BodyPose = [
            .leftHip: PosePoint(x: 0.3, y: 0.4, confidence: 0.9),
            .leftKnee: PosePoint(x: 0.5, y: 0.4, confidence: 0.9),
            .leftAnkle: PosePoint(x: 0.5, y: 0.2, confidence: 0.9),
        ]
        let spec = PhotoAnalyze(kind: .jointAngle, into: "knee_angle", joint: "knee")
        let v = await PhotoReader.reading(spec, field: knee, lines: [], labels: [], pose: squat, aspect: 1)
        XCTAssertEqual(v?.jsonValue(for: knee.type), .number(90))
        let none = await PhotoReader.reading(spec, field: knee, lines: [], labels: [], pose: nil, aspect: 1)
        XCTAssertNil(none)
    }

    func testOverlayGeometry() {
        // 横長 400x200 を 200x200 の枠に収める → 上下に 50 の余白
        let rect = PosePhotoView.fittedRect(imageSize: CGSize(width: 400, height: 200), in: CGSize(width: 200, height: 200))
        XCTAssertEqual(rect, CGRect(x: 0, y: 50, width: 200, height: 100))
        // Vision は左下原点 → 画面は左上原点
        XCTAssertEqual(PosePhotoView.point(PosePoint(x: 0, y: 1), in: rect), CGPoint(x: 0, y: 50))
        XCTAssertEqual(PosePhotoView.point(PosePoint(x: 1, y: 0), in: rect), CGPoint(x: 200, y: 150))
        XCTAssertEqual(PosePhotoView.fittedRect(imageSize: .zero, in: CGSize(width: 10, height: 10)), .zero)
    }

    // MARK: 分類・説明の依頼文

    func testClassifyRequestCarriesCluesAndOptions() {
        let r = PhotoReader.classifyRequest(options: ["食費", "交通"], labels: ["food"], text: "合計 ¥493")
        XCTAssertEqual(r.task, .classify)
        XCTAssertEqual(r.options, ["食費", "交通"])
        XCTAssertTrue(r.prompt.contains("food") && r.prompt.contains("合計 ¥493") && r.prompt.contains("食費 / 交通"))
        XCTAssertEqual(AIRequestBuilder.normalize("「交通」", for: r), "交通")
    }
}
