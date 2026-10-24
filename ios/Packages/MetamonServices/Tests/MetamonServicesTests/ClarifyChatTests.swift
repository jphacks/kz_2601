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

    // MARK: - #160 体感速度

    func testStartConversationDoesNotCallDeleteWhenServerIsFresh() async throws {
        let ai = SpyChatAI(staleUserTurns: 0)
        let r = try await ClarifyFlow.startConversation(ai: ai, appId: "a", first: .init(role: .user, text: "家計簿"), locale: nil)
        let log = await ai.log
        XCTAssertEqual(log, ["POST:1"], "DELETE を待たずに POST 1 回だけ")
        XCTAssertEqual(r.turn.turn, 1)
        XCTAssertFalse(r.retried)
    }

    func testStartConversationClearsAndResendsWhenAppendedToStaleConversation() async throws {
        // 前の会話(ユーザー 2 発)が残っていて 1 件の POST が追記された → turn=3 → DELETE を待って送り直す
        let ai = SpyChatAI(staleUserTurns: 2)
        let r = try await ClarifyFlow.startConversation(ai: ai, appId: "a", first: .init(role: .user, text: "家計簿"), locale: nil)
        let log = await ai.log
        XCTAssertEqual(log, ["POST:1", "DELETE", "POST:1"])
        XCTAssertEqual(r.turn.turn, 1)
        XCTAssertTrue(r.retried)
    }

    func testAutoBuildOnlyWhenReadyViaVoiceAndEnabled() {
        let ready = ChatTurn(kind: .ready, text: "では作りますね。", turn: 2, transcript: "家計簿")
        let question = ChatTurn(kind: .question, text: "写真は?", turn: 1)
        XCTAssertTrue(ClarifyFlow.shouldAutoBuild(turn: ready, lastInput: .voice, enabled: true))
        XCTAssertFalse(ClarifyFlow.shouldAutoBuild(turn: ready, lastInput: .text, enabled: true), "文字入力はボタンで")
        XCTAssertFalse(ClarifyFlow.shouldAutoBuild(turn: ready, lastInput: .chip, enabled: true), "チップはボタンで")
        XCTAssertFalse(ClarifyFlow.shouldAutoBuild(turn: ready, lastInput: nil, enabled: true))
        XCTAssertFalse(ClarifyFlow.shouldAutoBuild(turn: ready, lastInput: .voice, enabled: false), "設定 OFF")
        XCTAssertFalse(ClarifyFlow.shouldAutoBuild(turn: question, lastInput: .voice, enabled: true), "質問中は進めない")
    }

    func testChatTimingSummary() {
        let t0 = ContinuousClock.now
        var t = ChatTiming(recordStopped: t0, transcribed: t0 + .milliseconds(1200))
        t.postStarted = t0 + .milliseconds(1210)
        t.responded = t0 + .milliseconds(3510)
        t.shown = t0 + .milliseconds(3520)
        XCTAssertEqual(t.summary, "stt=1200ms pre=10ms net=2300ms render=10ms total=3520ms")
        XCTAssertTrue(ChatTiming().summary.hasPrefix("stt=- pre=- net=-"))
    }
}

/// backend の `mergeHistory` を真似る: 1 件の POST はサーバーに残る会話へ追記、DELETE で消える。
private actor SpyChatAI: AIClient {
    private(set) var log: [String] = []
    private var storedUserTurns: Int

    init(staleUserTurns: Int) { storedUserTurns = staleUserTurns }

    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult { throw URLError(.unsupportedURL) }

    func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
        log.append("POST:\(messages.count)")
        let users = messages.filter { $0.role == .user }.count
        storedUserTurns = messages.count > 1 ? users : storedUserTurns + users
        return ChatTurn(kind: .question, text: "?", turn: storedUserTurns)
    }

    func resetChat(appId: String) async {
        log.append("DELETE")
        storedUserTurns = 0
    }
}
