import XCTest
import MetamonDSL
@testable import MetamonServices

/// #417 aiButton generate のオンライン版: 送ってよいか(同意 → Pro)・モックの決まった文・サーバーのエラーの文言。
final class AITextGateTests: XCTestCase {
    func testGateOrder() {
        XCTAssertEqual(AITextGate.decide(consentGranted: false, isPro: true, enforced: true), .needsConsent, "同意が先")
        XCTAssertEqual(AITextGate.decide(consentGranted: true, isPro: false, enforced: true), .needsPro)
        XCTAssertEqual(AITextGate.decide(consentGranted: true, isPro: true, enforced: true), .send)
        XCTAssertEqual(AITextGate.decide(consentGranted: true, isPro: false, enforced: false), .send, "課金の制限の無効化中(Debug の既定)は通す")
    }

    func testMockIsDeterministicPerShape() async throws {
        let mock = MockAIClient(delay: .zero)
        let list = AITextRequest(instruction: "罰ゲーム", shape: .list, count: 4)
        let a = try await mock.text(list)
        let b = try await mock.text(list)
        XCTAssertEqual(a, b, "毎回同じ")
        XCTAssertEqual(a.items?.count, 4)
        let fields = try await mock.text(AITextRequest(instruction: "献立", shape: .fields, items: ["主菜", "汁物"]))
        XCTAssertEqual(fields.fields?.map(\.name), ["主菜", "汁物"])
        let line = try await mock.text(AITextRequest(instruction: "一言", shape: .line))
        XCTAssertFalse(line.text.contains("\n"))
    }

    func testConsentGateBlocksText() async {
        let gated = ConsentGatedAIClient(base: MockAIClient(delay: .zero)) { false }
        do {
            _ = try await gated.text(AITextRequest(instruction: "x", shape: .line))
            XCTFail("同意が無ければ送らない")
        } catch {
            XCTAssertEqual(error as? AIConsentError, .notGranted)
        }
    }

    func testServerErrorsMapToScreenText() {
        XCTAssertEqual(HTTPAIClient.textError(.server(status: 422, code: "refused", message: "the model declined")) as? AITextError, .refused)
        XCTAssertEqual(HTTPAIClient.textError(.server(status: 502, code: "bad_output", message: "missing field")) as? AITextError, .badOutput)
        let limited = HTTPAIClient.textError(.server(status: 429, code: "rate_limited", message: "今日の利用上限(60)に達しました。"))
        XCTAssertEqual(limited.localizedDescription, "今日の利用上限(60)に達しました。", "上限はサーバーの日本語のまま")
    }
}
