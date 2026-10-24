import XCTest
@testable import MetamonServices

/// 生成前の対話(#75)。`POST /v1/chat` の契約(docs/backend/openapi-ai.yaml `ChatRequest` / `ChatResponse`)とモックの振る舞い。
final class ClarifyChatTests: XCTestCase {
    func testDecodesQuestion() throws {
        // staging(ai-stg)の実応答をそのまま
        let json = #"{"kind":"question","text":"レシートの写真も残しますか?","suggestions":["はい、残す","いいえ、不要","お任せ"],"turn":1,"locale":"ja","source":"claude","latencyMs":{"claude":9371,"total":9371},"meta":{"model":"m","attempts":1}}"#
        let t = try JSONDecoder().decode(ChatTurn.self, from: Data(json.utf8))
        XCTAssertEqual(t.kind, .question)
        XCTAssertEqual(t.suggestions?.count, 3)
        XCTAssertEqual(t.turn, 1)
        XCTAssertNil(t.transcript)
        XCTAssertNil(t.brief)
    }

    func testDecodesReady() throws {
        let json = #"{"kind":"ready","text":"では作りますね。","turn":2,"locale":"ja","source":"shortcut","brief":{"name":"筋トレ記録","purpose":"回数を毎日つける","entities":["トレーニング"],"features":["記録の一覧"],"native":["notification"]},"transcript":"筋トレ記録アプリを作って。"}"#
        let t = try JSONDecoder().decode(ChatTurn.self, from: Data(json.utf8))
        XCTAssertEqual(t.kind, .ready)
        XCTAssertEqual(t.brief?.name, "筋トレ記録")
        XCTAssertEqual(t.brief?.native, ["notification"])
        XCTAssertEqual(t.transcript, "筋トレ記録アプリを作って。")
        XCTAssertEqual(t.source, "shortcut")
    }

    func testChatBodyShape() throws {
        let body = HTTPAIClient.ChatBody(appId: "a", messages: [.init(role: .user, text: "家計簿"), .init(role: .assistant, text: "写真は?")], locale: nil)
        let obj = try JSONSerialization.jsonObject(with: JSONEncoder().encode(body)) as? [String: Any]
        XCTAssertEqual(obj?["appId"] as? String, "a")
        XCTAssertNil(obj?["locale"], "未指定なら省略(サーバーが発話から判定)")
        let messages = obj?["messages"] as? [[String: String]]
        XCTAssertEqual(messages?.map { $0["role"] }, ["user", "assistant"])
        XCTAssertEqual(messages?.first?["text"], "家計簿")
    }

    func testMockAsksOnceThenReady() async throws {
        let ai = MockAIClient(delay: .zero)
        var history = [ChatMessage(role: .user, text: "家計簿")]
        let first = try await ai.chat(appId: "x", messages: history, locale: nil)
        XCTAssertEqual(first.kind, .question)
        history += [.init(role: .assistant, text: first.text), .init(role: .user, text: "写真も残す")]
        let second = try await ai.chat(appId: "x", messages: history, locale: nil)
        XCTAssertEqual(second.kind, .ready)
        XCTAssertEqual(second.transcript, "家計簿。写真も残す")
    }

    func testMockGoAheadIsReadyAndDropsTheGoAheadWord() async throws {
        let ai = MockAIClient(delay: .zero)
        let history: [ChatMessage] = [.init(role: .user, text: "筋トレ"), .init(role: .assistant, text: "?"), .init(role: .user, text: "もう作って")]
        let t = try await ai.chat(appId: "x", messages: history, locale: nil)
        XCTAssertEqual(t.kind, .ready)
        XCTAssertEqual(t.source, "shortcut")
        XCTAssertEqual(t.transcript, "筋トレ")
        XCTAssertEqual(t.brief?.name, "筋トレ記録")
    }

    func testMockLongFirstUtteranceIsReady() async throws {
        let t = try await MockAIClient(delay: .zero).chat(appId: "x", messages: [.init(role: .user, text: "家計簿アプリが欲しい。金額とカテゴリを記録したい")], locale: nil)
        XCTAssertEqual(t.kind, .ready)
    }
}
