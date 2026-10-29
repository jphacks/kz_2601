import XCTest
@testable import MetamonDSL

/// `photoField.analyze`(#418): デコード・レシートの金額 / 日付・分類のフォールバック・関節の角度・保存形式。
final class PhotoReadingTests: XCTestCase {
    func testExamplesDecodeAnalyze() throws {
        let receipt = try ExamplesFixture.load("receipt_ai")
        let shot = try XCTUnwrap(receipt.screens.flatMap(\.allComponents).first { $0.type == .photoField })
        XCTAssertEqual(shot.analyze.map(\.kind), [.amount, .date, .classify, .text])
        XCTAssertEqual(shot.analyze.map(\.into), ["amount", "date", "category", "memo"])

        let form = try ExamplesFixture.load("form_check")
        let pose = try XCTUnwrap(form.screens.flatMap(\.allComponents).first { $0.type == .photoField })
        XCTAssertEqual(pose.analyze, [PhotoAnalyze(kind: .jointAngle, into: "knee_angle", joint: "knee"),
                                      PhotoAnalyze(kind: .jointAngle, into: "hip_angle", joint: "hip")])
        XCTAssertTrue(DSLExamples.names.contains("receipt_ai"))
        XCTAssertTrue(DSLExamples.names.contains("form_check"))
    }

    func testUnknownKindAndBrokenEntriesAreDropped() throws {
        let json = #"{"type":"photoField","field":"p","analyze":[{"kind":"barcode","into":"a"},{"kind":"text"},{"kind":"text","into":"memo"}]}"#
        let c = try JSONDecoder().decode(Component.self, from: Data(json.utf8))
        XCTAssertEqual(c.analyze, [PhotoAnalyze(kind: .text, into: "memo")], "古い iOS に新しい kind が来ても落ちない")
        let plain = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"photoField","field":"p"}"#.utf8))
        XCTAssertTrue(plain.analyze.isEmpty)
    }

    // MARK: レシート

    func testAmountPrefersTotalLine() {
        let lines = ["スーパーまるや", "2026年10月28日(火) 18:42", "牛乳 ¥198", "食パン ¥258", "小計 ¥456", "合計点数 2点",
                     "合計 ¥493", "お預り ¥1,000", "お釣り ¥507"]
        XCTAssertEqual(ReceiptParser.amount(lines: lines), 493)
    }

    func testAmountOnNextLineAndFullWidth() {
        XCTAssertEqual(ReceiptParser.amount(lines: ["お会計", "￥１，２８０"]), 1280)
        XCTAssertEqual(ReceiptParser.amount(lines: ["TOTAL", "\\3,456"]), 3456, "円記号を \\ と読んだ OCR")
    }

    func testAmountFallsBackToLargestMarkedValue() {
        XCTAssertEqual(ReceiptParser.amount(lines: ["コーヒー 450円", "ケーキ 520円", "TEL 076-123-4567"]), 520)
        XCTAssertNil(ReceiptParser.amount(lines: ["会員番号 12345678", "ありがとうございました"]))
    }

    func testDateFormats() {
        XCTAssertEqual(ReceiptParser.date(lines: ["領収書", "2026年10月28日(火) 18:42"]),
                       DateComponents(year: 2026, month: 10, day: 28, hour: 18, minute: 42))
        XCTAssertEqual(ReceiptParser.date(lines: ["2026/1/5"]), DateComponents(year: 2026, month: 1, day: 5))
        XCTAssertEqual(ReceiptParser.date(lines: ["令和8年10月28日"]), DateComponents(year: 2026, month: 10, day: 28))
        XCTAssertNil(ReceiptParser.date(lines: ["2026/2/30"]), "無い日付は拾わない")
        XCTAssertNil(ReceiptParser.date(lines: ["合計 ¥493"]))
    }

    // MARK: 保存形式

    func testJSONValueForFieldTypes() {
        let tokyo = TimeZone(identifier: "Asia/Tokyo")!
        XCTAssertEqual(PhotoReadingValue.number(492.6).jsonValue(for: .currency), .number(493))
        XCTAssertEqual(PhotoReadingValue.number(87.25).jsonValue(for: .number), .number(87.3))
        XCTAssertEqual(PhotoReadingValue.date(DateComponents(year: 2026, month: 10, day: 28)).jsonValue(for: .date), .string("2026-10-28"))
        XCTAssertEqual(PhotoReadingValue.date(DateComponents(year: 2026, month: 10, day: 28, hour: 18, minute: 42)).jsonValue(for: .datetime, timeZone: tokyo),
                       .string("2026-10-28T18:42:00+09:00"))
        XCTAssertEqual(PhotoReadingValue.text("  牛乳  ").jsonValue(for: .text), .string("牛乳"))
        XCTAssertNil(PhotoReadingValue.text(" ").jsonValue(for: .string))
        XCTAssertNil(PhotoReadingValue.text("x").jsonValue(for: .currency), "型が合わない組み合わせは入れない")
    }

    // MARK: 分類

    func testClassifierUsesLabelsAndText() {
        let options = ["食費", "日用品", "交通", "その他"]
        XCTAssertEqual(PhotoClassifier.classify(options: options, labels: ["food", "fruit"], text: ""), "食費")
        XCTAssertEqual(PhotoClassifier.classify(options: options, labels: ["document"], text: "JR 東日本 交通系IC"), "交通")
        XCTAssertNil(PhotoClassifier.classify(options: options, labels: ["structure"], text: ""), "当てはまらなければ入れない")
        XCTAssertEqual(PhotoClassifier.classify(options: ["花", "観葉植物", "多肉植物"], labels: ["succulent", "plant"], text: ""), "多肉植物")
    }

    // MARK: 関節の角度

    private func pose(_ pts: [PoseKeypoint: (Double, Double)], confidence: Double = 0.9) -> BodyPose {
        pts.mapValues { PosePoint(x: $0.0, y: $0.1, confidence: confidence) }
    }

    func testDegrees() {
        let o = PosePoint(x: 0, y: 0)
        XCTAssertEqual(PoseAngle.degrees(PosePoint(x: 1, y: 0), vertex: o, PosePoint(x: 0, y: 1))!, 90, accuracy: 1e-9)
        XCTAssertEqual(PoseAngle.degrees(PosePoint(x: -1, y: 0), vertex: o, PosePoint(x: 1, y: 0))!, 180, accuracy: 1e-9)
        XCTAssertEqual(PoseAngle.degrees(PosePoint(x: 1, y: 1), vertex: o, PosePoint(x: 1, y: 0))!, 45, accuracy: 1e-9)
        XCTAssertNil(PoseAngle.degrees(o, vertex: o, PosePoint(x: 1, y: 0)))
    }

    /// 立った脚(まっすぐ)= 180 度、スクワットの直角 = 90 度。
    func testKneeAngle() throws {
        let straight = pose([.leftHip: (0.5, 0.6), .leftKnee: (0.5, 0.4), .leftAnkle: (0.5, 0.2)])
        XCTAssertEqual(try XCTUnwrap(PoseAngle.angle(joint: "leftKnee", in: straight)), 180, accuracy: 1e-6)
        let squat = pose([.rightHip: (0.3, 0.4), .rightKnee: (0.5, 0.4), .rightAnkle: (0.5, 0.2)])
        XCTAssertEqual(try XCTUnwrap(PoseAngle.angle(joint: "knee", in: squat)), 90, accuracy: 1e-6, "左右無しは見えている側")
        XCTAssertNil(PoseAngle.angle(joint: "leftKnee", in: squat))
    }

    /// 正規化座標は縦横比で戻してから測る(横長の写真で 45 度に見える線)。
    func testAspectRatioIsApplied() throws {
        let p = pose([.leftShoulder: (0.5, 0.8), .leftElbow: (0.5, 0.5), .leftWrist: (0.75, 0.5)])
        XCTAssertEqual(try XCTUnwrap(PoseAngle.angle(joint: "leftElbow", in: p, aspect: 1)), 90, accuracy: 1e-6)
        let slanted = pose([.leftShoulder: (0.3, 0.8), .leftElbow: (0.5, 0.5), .leftWrist: (0.75, 0.5)])
        // 幅 = 高さ × 1.5 なら (−0.3, 0.3) は 45 度、上腕と前腕の角度は 135 度
        XCTAssertEqual(try XCTUnwrap(PoseAngle.angle(joint: "leftElbow", in: slanted, aspect: 1.5)), 135, accuracy: 1e-6)
    }

    func testBareJointPicksMoreConfidentSideAndSkipsLowConfidence() throws {
        var p = pose([.leftHip: (0.5, 0.6), .leftKnee: (0.5, 0.4), .leftAnkle: (0.5, 0.2)], confidence: 0.5)
        for (k, v) in pose([.rightHip: (0.3, 0.4), .rightKnee: (0.5, 0.4), .rightAnkle: (0.5, 0.2)], confidence: 0.9) { p[k] = v }
        XCTAssertEqual(try XCTUnwrap(PoseAngle.angle(joint: "knee", in: p)), 90, accuracy: 1e-6)
        let faint = pose([.leftHip: (0.5, 0.6), .leftKnee: (0.5, 0.4), .leftAnkle: (0.5, 0.2)], confidence: 0.1)
        XCTAssertNil(PoseAngle.angle(joint: "knee", in: faint))
        XCTAssertNil(PoseAngle.angle(joint: "neck", in: p), "未知の関節は nil")
    }
}
