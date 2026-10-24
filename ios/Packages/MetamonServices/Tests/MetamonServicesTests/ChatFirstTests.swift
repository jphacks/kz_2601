import XCTest
@testable import MetamonServices

/// チャット基本化(#184 会話の取り決め / #186 iOS)。answer / proposal のデコード・リクエストの形・kind ごとの表示・自動実行の判定・モック。
final class ChatFirstTests: XCTestCase {
    // MARK: - 契約(デコード / リクエスト)

    func testDecodesAnswer() throws {
        let json = #"{"kind":"answer","text":"いいえ、WebView ではありません。","turn":1,"locale":"ja","source":"claude"}"#
        let t = try JSONDecoder().decode(ChatTurn.self, from: Data(json.utf8))
        XCTAssertEqual(t.kind, .answer)
        XCTAssertNil(t.options)
        XCTAssertEqual(ClarifyFlow.offer(for: t), .none)
    }

    func testDecodesProposal() throws {
        let json = #"{"kind":"proposal","text":"こんな案はどうでしょう","turn":1,"options":[{"id":"a","title":"数字を大きく","detail":"金額を太字に","instruction":"金額を大きな太字にして"},{"id":"b","title":"カード","detail":"","instruction":"一覧をカードにして"}]}"#
        let t = try JSONDecoder().decode(ChatTurn.self, from: Data(json.utf8))
        XCTAssertEqual(t.kind, .proposal)
        XCTAssertEqual(t.options?.map(\.id), ["a", "b"])
        XCTAssertEqual(t.options?.first?.instruction, "金額を大きな太字にして")
    }

    func testOptionMissingFieldsFallBack() throws {
        let json = #"{"kind":"proposal","text":"?","turn":1,"options":[{"title":"数字を大きく"}]}"#
        let o = try XCTUnwrap(JSONDecoder().decode(ChatTurn.self, from: Data(json.utf8)).options?.first)
        XCTAssertEqual(o.id, "数字を大きく")
        XCTAssertEqual(o.detail, "")
        XCTAssertEqual(o.instruction, "数字を大きく", "instruction が無ければ title を指示文にする")
    }

    func testUnknownKindIsTreatedAsAnswer() throws {
        let json = #"{"kind":"something_new","text":"新しい種類","turn":1}"#
        let t = try JSONDecoder().decode(ChatTurn.self, from: Data(json.utf8))
        XCTAssertEqual(t.kind, .answer, "知らない kind で落ちず、吹き出しだけ出す")
    }

    func testChatBodyHasModeAndCapabilities() throws {
        let body = HTTPAIClient.ChatBody(appId: "a", messages: [.init(role: .user, text: "見やすくして")], locale: "ja", mode: .edit)
        let obj = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(body)) as? [String: Any])
        XCTAssertEqual(obj["mode"] as? String, "edit")
        XCTAssertEqual(obj["capabilities"] as? [String], ["answer", "proposal"])
        let noMode = HTTPAIClient.ChatBody(appId: "a", messages: [.init(role: .user, text: "x")], locale: nil)
        let obj2 = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(noMode)) as? [String: Any])
        XCTAssertNil(obj2["mode"], "mode 未指定なら省略(backend が appId の DSL 有無で判定)")
    }

    // MARK: - kind ごとの表示

    func testOfferPerKind() {
        XCTAssertEqual(ClarifyFlow.offer(for: ChatTurn(kind: .question, text: "?", suggestions: ["a", "b", "c", "d"], turn: 1)), .chips(["a", "b", "c"]))
        let options = [ChatOption(id: "1", title: "A", instruction: "Aにして"), ChatOption(id: "2", title: "B", instruction: "  ")]
        XCTAssertEqual(ClarifyFlow.offer(for: ChatTurn(kind: .proposal, text: "", turn: 1, options: options)),
                       .proposals([options[0]]), "指示文が空の案は出さない(押しても何もできない)")
        XCTAssertEqual(ClarifyFlow.offer(for: ChatTurn(kind: .proposal, text: "", turn: 1, options: [])), .none)
        XCTAssertEqual(ClarifyFlow.offer(for: ChatTurn(kind: .ready, text: "", turn: 1, transcript: " 金額を大きくして ")),
                       .confirm(transcript: "金額を大きくして"))
        XCTAssertEqual(ClarifyFlow.offer(for: ChatTurn(kind: .ready, text: "", turn: 1)), .confirm(transcript: nil))
    }

    func testShortcutPerModeAndKind() {
        let answer = ChatTurn(kind: .answer, text: "", turn: 1)
        let question = ChatTurn(kind: .question, text: "", turn: 1)
        let proposal = ChatTurn(kind: .proposal, text: "", turn: 1, options: [ChatOption(id: "1", title: "A", instruction: "A")])
        let ready = ChatTurn(kind: .ready, text: "", turn: 1, transcript: "x")
        XCTAssertEqual(ClarifyFlow.shortcut(for: nil, mode: .create), .buildNow)
        XCTAssertEqual(ClarifyFlow.shortcut(for: answer, mode: .create), .buildNow)
        XCTAssertEqual(ClarifyFlow.shortcut(for: ready, mode: .create), .none, "ready は主ボタン「これでいく」だけ")
        XCTAssertEqual(ClarifyFlow.shortcut(for: question, mode: .edit), .applyNow)
        XCTAssertEqual(ClarifyFlow.shortcut(for: answer, mode: .edit), .none, "質問に答えただけの時に改変ボタンを出さない")
        XCTAssertEqual(ClarifyFlow.shortcut(for: proposal, mode: .edit), .none)
        XCTAssertEqual(ClarifyFlow.shortcut(for: ready, mode: .edit), .none)
    }

    func testAutoBuildOnlyCreateReadyByVoice() {
        let ready = ChatTurn(kind: .ready, text: "", turn: 1, transcript: "x")
        XCTAssertTrue(ClarifyFlow.shouldAutoBuild(turn: ready, lastInput: .voice, enabled: true, mode: .create))
        XCTAssertFalse(ClarifyFlow.shouldAutoBuild(turn: ready, lastInput: .voice, enabled: true, mode: .edit), "edit は自動で改変しない")
        let proposal = ChatTurn(kind: .proposal, text: "", turn: 1, options: [ChatOption(id: "1", title: "A", instruction: "A")])
        XCTAssertFalse(ClarifyFlow.shouldAutoBuild(turn: proposal, lastInput: .voice, enabled: true, mode: .create), "案は押すまで変えない")
        XCTAssertFalse(ClarifyFlow.shouldAutoBuild(turn: ChatTurn(kind: .answer, text: "", turn: 1), lastInput: .voice, enabled: true))
    }

    // MARK: - モック(シミュレータ確認用)

    func testMockEditBranches() async throws {
        let ai = MockAIClient(delay: .zero)
        let answer = try await ai.chat(appId: "x", messages: [.init(role: .user, text: "これ現状は WebView でやってるんですか?")], locale: nil, mode: .edit)
        XCTAssertEqual(answer.kind, .answer)
        XCTAssertTrue(answer.text.contains("WebView"))

        let proposal = try await ai.chat(appId: "x", messages: [.init(role: .user, text: "見やすくして")], locale: nil, mode: .edit)
        XCTAssertEqual(proposal.kind, .proposal)
        XCTAssertEqual(proposal.options?.count, 2)

        let ready = try await ai.chat(appId: "x", messages: [.init(role: .user, text: "金額を大きくして")], locale: nil, mode: .edit)
        XCTAssertEqual(ready.kind, .ready)
        XCTAssertEqual(ready.transcript, "金額を大きくして")
    }

    func testMockQuestionThenInstructionKeepsOnlyInstruction() async throws {
        let ai = MockAIClient(delay: .zero)
        let history: [ChatMessage] = [.init(role: .user, text: "もうちょっと大きくすることも可能?"), .init(role: .assistant, text: "できます"),
                                      .init(role: .user, text: "金額を大きくして")]
        let t = try await ai.chat(appId: "x", messages: history, locale: nil, mode: .edit)
        XCTAssertEqual(t.kind, .ready)
        XCTAssertEqual(t.transcript, "金額を大きくして")
    }

    func testMockCreateStillAsksForShortFirstUtterance() async throws {
        let t = try await MockAIClient(delay: .zero).chat(appId: "x", messages: [.init(role: .user, text: "日記")], locale: nil, mode: .create)
        XCTAssertEqual(t.kind, .question)
    }

    func testMockEditSummaryEchoesInstruction() async throws {
        let r = try await MockAIClient(delay: .zero).morph(transcript: "金額を大きくして", appId: "x", currentVersion: 3)
        XCTAssertEqual(r.summary, "「金額を大きくして」を反映しました(モック)")
        XCTAssertEqual(r.document.version, 4)
    }
}
