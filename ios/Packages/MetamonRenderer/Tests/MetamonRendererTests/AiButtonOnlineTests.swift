import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// aiButton の要約・分類のオンライン版: 依頼の組み立て・答えを output に入れる・ホストが断った時(Pro でない等)はドラフトを変えない。
@MainActor
final class AiButtonOnlineTests: XCTestCase {
    private final class FakeHost: AITextHost {
        var requests: [AITextRequest] = []
        var answer: Result<AITextResult, AITextError> = .success(AITextResult(shape: .line, text: "公園で遊んだ日"))
        var supportsOnlineAIText: Bool { true }
        func generateAIText(_ request: AITextRequest) async throws -> AITextResult {
            requests.append(request)
            return try answer.get()
        }
        func recordAIButton(_ event: AIButtonMetric.Event) {}
    }

    private func diary() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/diary_ai.json"))
    }

    private func button(_ id: String) throws -> (Component, Entity) {
        let dsl = try diary()
        let c = try XCTUnwrap(dsl.screen("diary_form")?.components.first { $0.id == id })
        return (c, try XCTUnwrap(dsl.entity("diary")))
    }

    func testSummarizeRequestSendsInputSeparately() throws {
        let (c, entity) = try button("ai_summary")
        let r = try AIRequestBuilder.online(c, entity: entity, values: ["body": "公園で友達と遊んだ。"], locale: "ja")
        XCTAssertEqual(r.task, "summarize")
        XCTAssertEqual(r.input, "公園で友達と遊んだ。")
        XCTAssertEqual(r.instruction, "日記の本文を 30 字以内の 1 文に要約して。", "本文は指示に混ぜない")
        XCTAssertEqual(r.shape, .line)
        XCTAssertNil(r.options)
    }

    func testClassifyTakesEnumOptionsAndInputIsClipped() throws {
        let (c, entity) = try button("ai_mood")
        let body = String(repeating: "あ", count: 3000) // 9000 バイト
        let r = try AIRequestBuilder.online(c, entity: entity, values: ["body": .string(body)], locale: "ja")
        XCTAssertEqual(r.options, ["うれしい", "ふつう", "かなしい", "イライラ", "つかれた"])
        XCTAssertLessThanOrEqual(r.input?.utf8.count ?? 0, AITextLimits.maxInputBytes)
        XCTAssertThrowsError(try AIRequestBuilder.online(c, entity: entity, values: ["body": ""], locale: "ja")) {
            XCTAssertEqual($0 as? AIRequestBuilder.BuildError, .emptyInput)
        }
    }

    func testSpacedInputPlaceholderBecomesMarker() throws {
        let (_, entity) = try button("ai_summary")
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiButton","task":"summarize","input":"body","output":"summary","prompt":"{{ input }} を{{  mood  }}な調子で要約。{{in put}}"}"#.utf8))
        let r = try AIRequestBuilder.online(c, entity: entity, values: ["body": "公園で遊んだ。", "mood": "うれしい"], locale: "ja")
        XCTAssertEqual(r.instruction, "下の材料 をうれしいな調子で要約。{{in put}}")
        XCTAssertEqual(r.input, "公園で遊んだ。")
    }

    func testFillWritesAnswerIntoOutput() async throws {
        let (c, entity) = try button("ai_summary")
        let form = FormDraft(entity: entity)
        form.values["body"] = "公園で友達と遊んだ。"
        let host = FakeHost()
        try await AIButtonOnline.fill(c, form: form, values: form.values, sourceEntity: nil, records: [], host: host, locale: "ja")
        XCTAssertEqual(form.values["summary"], .string("公園で遊んだ日"))
        XCTAssertEqual(host.requests.count, 1)
    }

    func testNotProLeavesDraftUntouched() async throws {
        let (c, entity) = try button("ai_summary")
        let form = FormDraft(entity: entity)
        form.values["body"] = "公園で友達と遊んだ。"
        form.values["summary"] = "自分で書いた"
        let host = FakeHost()
        host.answer = .failure(.needsPro)
        do {
            try await AIButtonOnline.fill(c, form: form, values: form.values, sourceEntity: nil, records: [], host: host, locale: "ja")
            XCTFail("needsPro を投げる")
        } catch {
            XCTAssertEqual(error as? AITextError, .needsPro)
            XCTAssertTrue(error.localizedDescription.contains("Pro") || error.localizedDescription.contains("AI"))
        }
        XCTAssertEqual(form.values["summary"], .string("自分で書いた"))
    }

    func testRouteFallsBackOnlyWhenHostHasOnline() {
        let off = AIAvailability.unavailable(.deviceNotEligible)
        XCTAssertEqual(AIGenerateRoute.choose(availability: off, hostOnline: true, mock: false), .online)
        if case .unavailable = AIGenerateRoute.choose(availability: off, hostOnline: false, mock: false) {} else {
            XCTFail("審査用ビルド(オンライン版なし)は無効のまま")
        }
    }
}
