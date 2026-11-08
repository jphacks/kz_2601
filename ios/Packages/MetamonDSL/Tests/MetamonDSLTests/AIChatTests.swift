import XCTest
@testable import MetamonDSL

/// `aiChat`: 発言の並び・役・送る会話(直近 20 発言・16 KB)・保存するデータ。
final class AIChatTests: XCTestCase {
    private func rec(_ id: String, _ text: String, role: String? = nil, at second: Int) -> Record {
        var data: [String: JSONValue] = ["text": .string(text)]
        if let role { data["role"] = .string(role) }
        return Record(id: id, data: data, createdAt: String(format: "2026-11-08T10:00:%02dZ", second))
    }

    func testTurnsUseRoleFieldInTimeOrder() {
        let rows = [rec("b", "いつから?", role: "assistant", at: 2), rec("a", "眠れない", role: "user", at: 1),
                    rec("c", "  ", role: "user", at: 3), rec("d", "先週から", role: "user", at: 4)]
        let turns = AIChat.turns(rows, textField: "text", roleField: "role")
        XCTAssertEqual(turns.map(\.id), ["a", "b", "d"], "古い順・本文の無い行は飛ばす")
        XCTAssertEqual(turns.map(\.role), [.user, .assistant, .user])
    }

    func testOwnKeepsOnlyMyRecords() {
        var mine = rec("a", "自分", role: "user", at: 1); mine.author = "u1"
        let local = rec("b", "未送信", role: "user", at: 2)
        var other = rec("c", "他の人", role: "user", at: 3); other.author = "u2"
        XCTAssertEqual(AIChat.own([mine, local, other], uid: "u1").map(\.id), ["a", "b"])
        XCTAssertEqual(AIChat.own([mine, local, other], uid: nil).map(\.id), ["b"], "uid が分からなければ author の無い分だけ")
    }

    /// speak: true で読む返事は、本文が同じ一番新しい AI の発言(自分の同じ文・古い同じ返事ではない)。
    func testSpokenReplyIsTheNewestMatchingAITurn() {
        let turns = [AIChat.Turn(id: "a", role: .assistant, text: "Hello"), AIChat.Turn(id: "b", role: .user, text: "Hello"),
                     AIChat.Turn(id: "c", role: .assistant, text: "Hello"), AIChat.Turn(id: "d", role: .user, text: "Bye")]
        XCTAssertEqual(AIChat.spokenReply(turns, reply: " Hello\n")?.id, "c")
        XCTAssertNil(AIChat.spokenReply(turns, reply: "Bye"), "自分の発言は読まない")
        XCTAssertNil(AIChat.spokenReply(turns, reply: "  "))
        XCTAssertNil(AIChat.spokenReply([], reply: "Hello"))
    }

    func testSpeakDecodes() throws {
        let on = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiChat","entity":"turn","textField":"text","persona":"p","speak":true}"#.utf8))
        XCTAssertEqual(on.speak, true)
        let off = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiChat","entity":"turn","textField":"text","persona":"p"}"#.utf8))
        XCTAssertNil(off.speak)
    }

    func testTurnsAlternateWithoutRoleField() {
        let rows = [rec("a", "1", at: 1), rec("b", "2", at: 2), rec("c", "3", at: 3)]
        XCTAssertEqual(AIChat.turns(rows, textField: "text", roleField: nil).map(\.role), [.user, .assistant, .user])
    }

    func testRequestEndsWithUserAndKeepsTheNewest() {
        let history = (0..<30).map { AIChat.Turn(id: "\($0)", role: $0.isMultiple(of: 2) ? .user : .assistant, text: "t\($0)") }
        let r = AIChat.request(persona: "  家庭教師  ", history: history, next: "次の問題", long: false, locale: "ja")
        let msgs = r.messages ?? []
        XCTAssertEqual(r.task, "chat")
        XCTAssertEqual(r.instruction, "家庭教師")
        XCTAssertTrue(r.isChat)
        XCTAssertLessThanOrEqual(msgs.count, AIChat.maxTurns)
        XCTAssertEqual(msgs.first?.role, "user", "先頭の AI の発言は落とす")
        XCTAssertEqual(msgs.last, AITextMessage(role: "user", text: "次の問題"))
        XCTAssertEqual(msgs.dropLast().last?.text, "t29", "新しい方を残す")
        XCTAssertNil(r.length)
        XCTAssertEqual(AIChat.request(persona: "p", history: [], next: "x", long: true, locale: "ja").length, "long")
    }

    func testRequestFitsTheByteCap() {
        let big = String(repeating: "あ", count: 3000) // 9000 バイト
        let history = [AIChat.Turn(id: "1", role: .user, text: big), AIChat.Turn(id: "2", role: .assistant, text: big)]
        let r = AIChat.request(persona: "p", history: history, next: big, long: false, locale: "ja")
        let bytes = (r.messages ?? []).reduce(0) { $0 + $1.text.utf8.count }
        XCTAssertLessThanOrEqual(bytes, AIChat.maxBytes)
        XCTAssertEqual(r.messages?.count, 1, "入らない古い発言は落とし、先頭の AI も落とす")
        let huge = AIChat.request(persona: "p", history: [], next: String(repeating: "い", count: 7000), long: false, locale: "ja")
        XCTAssertLessThanOrEqual(huge.messages?.first?.text.utf8.count ?? .max, AIChat.maxBytes)
    }

    func testRequestBody() throws {
        let r = AIChat.request(persona: "p", history: [], next: "hi", long: false, locale: "en")
        let body = try JSONSerialization.jsonObject(with: JSONEncoder().encode(r)) as? [String: Any]
        XCTAssertEqual(body?["task"] as? String, "chat")
        XCTAssertNil(body?["input"])
        let msgs = body?["messages"] as? [[String: String]]
        XCTAssertEqual(msgs ?? [], [["role": "user", "text": "hi"]])
    }

    func testRecordData() {
        XCTAssertEqual(AIChat.record(text: " やあ ", role: .assistant, textField: "text", roleField: "role", base: ["k": .string("v")]),
                       ["text": .string("やあ"), "role": .string("assistant"), "k": .string("v")])
        XCTAssertEqual(AIChat.record(text: "x", role: .user, textField: "text", roleField: nil), ["text": .string("x")])
        XCTAssertNil(AIChat.record(text: "  ", role: .user, textField: "text", roleField: "role"))
    }

    /// 答えの整え方は要約などと同じ(1200 字)、long は 3000 字。
    func testReplyNormalize() {
        let short = AIChat.request(persona: "p", history: [], next: "x", long: false, locale: "ja")
        XCTAssertEqual(AITextResult.normalize(text: String(repeating: "a", count: 2000), items: nil, fields: nil, for: short)?.text.count,
                       AITextLimits.maxTaskCharacters)
        let long = AIChat.request(persona: "p", history: [], next: "x", long: true, locale: "ja")
        XCTAssertEqual(AITextResult.normalize(text: String(repeating: "a", count: 4000), items: nil, fields: nil, for: long)?.text.count,
                       AITextLimits.maxLongCharacters)
    }

    func testExampleDecodes() throws {
        let dsl = try ExamplesFixture.load("ai_counselor")
        let chat = dsl.screens.flatMap(\.allComponents).first { $0.type == .aiChat }
        XCTAssertTrue(DSLExamples.names.contains("ai_counselor"))
        XCTAssertEqual(chat?.roleField, "role")
        XCTAssertEqual(chat?.textField, "text")
        XCTAssertFalse(chat?.persona?.isEmpty ?? true)
    }
}
