import XCTest
import MetamonDSL
@testable import MetamonServices

/// #417 aiButton generate のオンライン版: 送ってよいか(同意 → Pro)・モックの決まった文・サーバーのエラーの文言。
final class AITextGateTests: XCTestCase {
    private var savedLanguage: String?
    override func setUp() { savedLanguage = setAppLanguage("ja") } // 文言は日本語で確かめる(シミュレータの言語に依らない)
    override func tearDown() { setAppLanguage(savedLanguage) }

    func testGateOrder() {
        XCTAssertEqual(AITextGate.decide(consentGranted: false, isPro: true, enforced: true), .needsConsent, "同意が先")
        XCTAssertEqual(AITextGate.decide(consentGranted: true, isPro: false, enforced: true), .needsPro)
        XCTAssertEqual(AITextGate.decide(consentGranted: true, isPro: true, enforced: true), .send)
        XCTAssertEqual(AITextGate.decide(consentGranted: true, isPro: false, enforced: false), .send, "課金の制限の無効化中(Debug の既定)は通す")
    }

    /// 共有のアプリの中では Pro でなくてもサーバーに判定させる(参加者は作成者の Pro で通る)。同意は先に要る
    func testSharedAppDefersProToServer() {
        XCTAssertEqual(AITextGate.decide(consentGranted: true, isPro: false, enforced: true, sharedApp: true), .send)
        XCTAssertEqual(AITextGate.decide(consentGranted: false, isPro: false, enforced: true, sharedApp: true), .needsConsent)
        XCTAssertEqual(AITextGate.decide(consentGranted: true, isPro: false, enforced: true, sharedApp: false), .needsPro, "自分だけのアプリは今まで通り")
    }

    /// 参加者への 402(creator_needs_pro)は課金画面ではなく案内。作成者・参加者でない人は今まで通り課金画面(needsPro)
    func testCreatorNeedsProIsNeutral() {
        let participant = HTTPAIClient.textError(.server(status: 402, code: "pro_required", message: "this needs Tailor It Pro: onlineAI",
                                                         details: ["reason": .string("creator_needs_pro")]))
        XCTAssertEqual(participant as? AITextError, .creatorNeedsPro)
        XCTAssertEqual(participant.localizedDescription, "この機能は今は使えません。アプリを作った人に知らせてください")
        XCTAssertFalse(participant.localizedDescription.contains("Pro"), "参加者には課金の案内を出さない")
        let creator = HTTPAIClient.textError(.server(status: 402, code: "pro_required", message: "x", details: [:]))
        XCTAssertEqual(creator as? AITextError, .needsPro)
    }

    /// appId はオンライン版の本文に入る(付けなければ送らない)
    func testRequestCarriesAppId() throws {
        let base = AITextRequest(instruction: "励まし", shape: .line)
        let plain = try JSONSerialization.jsonObject(with: JSONEncoder().encode(base)) as? [String: Any]
        XCTAssertNil(plain?["appId"])
        let scoped = base.inApp("app-123")
        XCTAssertEqual(scoped.appId, "app-123")
        let body = try JSONSerialization.jsonObject(with: JSONEncoder().encode(scoped)) as? [String: Any]
        XCTAssertEqual(body?["appId"] as? String, "app-123")
        XCTAssertEqual(body?["instruction"] as? String, "励まし")
        XCTAssertNil(base.inApp("").appId, "空の appId は付けない")
    }

    /// 長い文の依頼にも appId が付き、length は残る(作成者の Pro で判定される)
    func testLongRequestCarriesAppId() throws {
        let long = AITextRequest(instruction: "手紙", shape: .line, length: "long").inApp("app-123")
        XCTAssertTrue(long.isLong)
        let body = try JSONSerialization.jsonObject(with: JSONEncoder().encode(long)) as? [String: Any]
        XCTAssertEqual(body?["length"] as? String, "long")
        XCTAssertEqual(body?["appId"] as? String, "app-123")
    }

    /// 写真つきの依頼にも appId が付き、写真は残る(作成者の Pro で判定される)
    func testPhotoRequestCarriesAppId() throws {
        let photo = AITextRequest(instruction: "薬", shape: .line, task: "extract",
                                  image: AITextImage(jpegBase64: "/9j/AA==")).inApp("app-123")
        XCTAssertEqual(photo.image, AITextImage(jpegBase64: "/9j/AA=="))
        let body = try JSONSerialization.jsonObject(with: JSONEncoder().encode(photo)) as? [String: Any]
        XCTAssertEqual(body?["appId"] as? String, "app-123")
        XCTAssertEqual((body?["image"] as? [String: Any])?["data"] as? String, "/9j/AA==")
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
        // #511: 一時停止は上限と別の文言
        XCTAssertEqual(HTTPAIClient.textError(.server(status: 503, code: "ai_paused", message: "paused")) as? AITextError,
                       .failed("AI を一時的に止めています。しばらくしてからもう一度お試しください"))
        // #316: サーバーも Pro を確かめる。402 は課金画面へ(needsPro)
        XCTAssertEqual(HTTPAIClient.textError(.server(status: 402, code: "pro_required", message: "this needs Tailor It Pro: onlineAI")) as? AITextError, .needsPro)
    }

    /// aiButton inputPhoto: 写真の検証・受け付け停止は写真の文言。写真と関係ない 400 はそのまま。
    func testPhotoErrorsMapToScreenText() {
        let resend = AITextError.failed("写真を AI に送れませんでした。撮り直してからお試しください")
        XCTAssertEqual(HTTPAIClient.textError(.server(status: 413, code: "payload_too_large", message: "image must be at most 409600 base64 characters"), photo: true) as? AITextError, resend)
        XCTAssertEqual(HTTPAIClient.textError(.server(status: 400, code: "bad_request", message: "image must be a JPEG"), photo: true) as? AITextError, resend)
        XCTAssertEqual(HTTPAIClient.textError(.server(status: 403, code: "forbidden", message: "photo input is not enabled"), photo: true) as? AITextError,
                       .failed("写真を AI に見せる機能は、今は使えません"))
        XCTAssertNil(HTTPAIClient.textError(.server(status: 400, code: "bad_request", message: "instruction must be at most 500 characters"), photo: true) as? AITextError)
    }

    /// aiChat: モックは最後の発言に返す・会話が長すぎる 413 は会話の文言・402(作成者が Pro でない)は案内だけ。
    func testChatMockAndErrors() async throws {
        let r = AITextRequest(instruction: "相談相手", shape: .line, locale: "ja", task: "chat",
                              messages: [AITextMessage(role: "user", text: "眠れない")])
        let out = try await MockAIClient(delay: .zero).text(r)
        XCTAssertTrue(out.text.contains("眠れない"))
        XCTAssertEqual(HTTPAIClient.textError(.server(status: 413, code: "payload_too_large", message: "messages must be at most 16384 bytes of text"), chat: true) as? AITextError,
                       .failed("会話が長すぎて AI に送れませんでした。短くしてからお試しください"))
        XCTAssertEqual(HTTPAIClient.textError(.server(status: 402, code: "pro_required", message: "x", details: ["reason": .string("creator_needs_pro")]), chat: true) as? AITextError,
                       .creatorNeedsPro)
    }

    func testRequestEncodesImage() throws {
        let r = AITextRequest(instruction: "", shape: .line, task: "extract", image: AITextImage(jpegBase64: "/9j/AA=="))
        let obj = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(r)) as? [String: Any])
        let image = try XCTUnwrap(obj["image"] as? [String: String])
        XCTAssertEqual(image, ["mediaType": "image/jpeg", "data": "/9j/AA=="])
        XCTAssertNil(obj["input"])
    }
}
