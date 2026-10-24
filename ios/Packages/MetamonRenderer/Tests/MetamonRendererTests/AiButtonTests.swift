import XCTest
import MetamonDSL
#if canImport(FoundationModels)
import FoundationModels
#endif
@testable import MetamonRenderer

/// #78 aiButton: 可用性の分岐(使えない理由の文言)と、AI に渡すリクエストの組み立て。
final class AiButtonTests: XCTestCase {
    private func diary() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/diary_ai.json"))
    }

    private func component(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    // MARK: 可用性

    func testUnavailableReasonsHaveMessages() {
        XCTAssertTrue(AIAvailability.available.isAvailable)
        XCTAssertNil(AIAvailability.available.message)
        let reasons: [AIAvailability.Reason] = [.deviceNotEligible, .appleIntelligenceNotEnabled, .modelNotReady, .osTooOld, .other]
        let messages = reasons.map { AIAvailability.unavailable($0).message }
        XCTAssertFalse(messages.contains(nil))
        XCTAssertEqual(Set(messages).count, reasons.count, "理由ごとに別の文言")
        XCTAssertTrue(AIAvailability.unavailable(.appleIntelligenceNotEnabled).message!.contains("設定"))
        XCTAssertFalse(AIAvailability.unavailable(.deviceNotEligible).isAvailable)
    }

    #if canImport(FoundationModels)
    func testMapsSystemLanguageModelAvailability() throws {
        guard #available(iOS 26.0, macOS 26.0, *) else { throw XCTSkip("iOS 26 未満") }
        XCTAssertEqual(AIAvailability(.available), .available)
        XCTAssertEqual(AIAvailability(.unavailable(.deviceNotEligible)), .unavailable(.deviceNotEligible))
        XCTAssertEqual(AIAvailability(.unavailable(.appleIntelligenceNotEnabled)), .unavailable(.appleIntelligenceNotEnabled))
        XCTAssertEqual(AIAvailability(.unavailable(.modelNotReady)), .unavailable(.modelNotReady))
        // 実機/シミュレータの現状がどれかに必ず落ちる(クラッシュしない)
        _ = AIAvailability.current
    }
    #endif

    // MARK: リクエスト

    func testSummarizeUsesInputFieldAndAppendsInstructionTemplate() throws {
        let dsl = try diary()
        let entity = try XCTUnwrap(dsl.entity("diary"))
        let c = try XCTUnwrap(dsl.screen("diary_form")?.components.first { $0.id == "ai_summary" })
        let req = try AIRequestBuilder.build(c, entity: entity, values: ["body": "公園で友達と遊んだ。楽しかった。", "summary": ""])
        XCTAssertEqual(req.task, .summarize)
        XCTAssertTrue(req.prompt.hasPrefix("日記の本文を 30 字以内"), req.prompt)
        XCTAssertTrue(req.prompt.hasSuffix("公園で友達と遊んだ。楽しかった。"), "プレースホルダ無しのテンプレは本文を後ろに付ける")
        XCTAssertTrue(req.options.isEmpty)
    }

    func testClassifyTakesOptionsFromEnumField() throws {
        let dsl = try diary()
        let entity = try XCTUnwrap(dsl.entity("diary"))
        let c = try XCTUnwrap(dsl.screen("diary_form")?.components.first { $0.id == "ai_mood" })
        let req = try AIRequestBuilder.build(c, entity: entity, values: ["body": "雨で予定が流れた"])
        XCTAssertEqual(req.options, ["うれしい", "ふつう", "かなしい", "イライラ", "つかれた"])
        XCTAssertTrue(req.prompt.contains("候補: うれしい / ふつう"))
        XCTAssertEqual(AIRequestBuilder.normalize("「かなしい」", for: req), "かなしい")
        XCTAssertEqual(AIRequestBuilder.normalize("たぶん つかれた です", for: req), "つかれた")
        XCTAssertNil(AIRequestBuilder.normalize("最高", for: req), "候補外は入れない")
    }

    func testEmptyInputThrows() throws {
        let dsl = try diary()
        let entity = try XCTUnwrap(dsl.entity("diary"))
        let c = try XCTUnwrap(dsl.screen("diary_form")?.components.first { $0.id == "ai_summary" })
        XCTAssertThrowsError(try AIRequestBuilder.build(c, entity: entity, values: ["body": "  "])) {
            XCTAssertEqual($0 as? AIRequestBuilder.BuildError, .emptyInput)
        }
    }

    func testWholeFormInputSkipsOutputAndPlaceholders() throws {
        let dsl = try diary()
        let entity = try XCTUnwrap(dsl.entity("diary"))
        let c = try component(#"{"type":"aiButton","task":"suggest","output":"summary","prompt":"気分は{{mood}}。{{input}} に一言"}"#)
        let req = try AIRequestBuilder.build(c, entity: entity, values: ["body": "散歩した", "mood": "ふつう", "summary": "古い要約"])
        XCTAssertTrue(req.prompt.hasPrefix("気分はふつう。"), req.prompt)
        XCTAssertTrue(req.prompt.contains("本文: 散歩した"))
        XCTAssertFalse(req.prompt.contains("古い要約"), "output 自身は入力に含めない")
    }

    func testEntityInputUsesRecentRecords() throws {
        let dsl = try diary()
        let entity = try XCTUnwrap(dsl.entity("diary"))
        let c = try component(#"{"type":"aiButton","task":"suggest","entity":"diary","limit":2,"output":"summary"}"#)
        let records = [
            Record(id: "1", data: ["body": "古い日"], createdAt: "2026-10-01T00:00:00Z"),
            Record(id: "2", data: ["body": "昨日"], createdAt: "2026-10-23T00:00:00Z"),
            Record(id: "3", data: ["body": "今日"], createdAt: "2026-10-24T00:00:00Z"),
        ]
        let req = try AIRequestBuilder.build(c, entity: entity, values: [:], sourceEntity: entity, records: records)
        XCTAssertTrue(req.prompt.contains("本文: 今日"))
        XCTAssertTrue(req.prompt.contains("本文: 昨日"))
        XCTAssertFalse(req.prompt.contains("古い日"), "limit 件だけ(新しい順)")
    }

    func testClassifyWithoutCandidatesThrows() throws {
        let dsl = try diary()
        let entity = try XCTUnwrap(dsl.entity("diary"))
        let c = try component(#"{"type":"aiButton","task":"classify","input":"body","output":"summary"}"#)
        XCTAssertThrowsError(try AIRequestBuilder.build(c, entity: entity, values: ["body": "x"])) {
            XCTAssertEqual($0 as? AIRequestBuilder.BuildError, .noOptions)
        }
    }
}
