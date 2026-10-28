import XCTest
@testable import MetamonDSL

/// `chatThread`(#260): 古い順の並び・自分 / 相手の判定・送信者名のまとめ方・送るデータ。
final class ChatThreadTests: XCTestCase {
    private func entity(scope: String?) throws -> Entity {
        var json = #"{"id":"messages","name":"メッセージ","fields":[{"id":"text","name":"本文","type":"text"}]"#
        if let scope { json += #","scope":"\#(scope)""# }
        return try JSONDecoder().decode(Entity.self, from: Data((json + "}").utf8))
    }

    func testDecodesExample() throws {
        let dsl = try ExamplesFixture.load("chat")
        let c = try XCTUnwrap(dsl.screens.first?.components.first)
        XCTAssertEqual(c.type, .chatThread)
        XCTAssertEqual(c.entity, "messages")
        XCTAssertEqual(c.textField, "text")
        XCTAssertEqual(c.photoField, "photo")
        XCTAssertEqual(c.placeholder, "メッセージを入力")
        XCTAssertTrue(dsl.entity("messages")?.isShared ?? false)
    }

    /// iOS が知らない部品名(新しい部品を古いビルドが受け取った時)は落ちずにプレースホルダ扱い。
    func testUnknownTypeStaysLenient() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"chatRoom","entity":"messages","textField":"text"}"#.utf8))
        XCTAssertEqual(c.type, .unknown)
        XCTAssertEqual(c.rawType, "chatRoom")
    }

    func testOrderedOldestFirstWithUnsentLast() {
        let records = [
            Record(id: "b", data: [:], createdAt: "2026-10-26T10:05:00Z"),
            Record(id: "local-1", data: [:], createdAt: "2026-10-26T10:06:30.250Z"), // 未送信(小数秒)
            Record(id: "old", data: [:]),                                              // 時刻なし
            Record(id: "a", data: [:], createdAt: "2026-10-26T10:00:00.500Z"),
            Record(id: "c", data: [:], createdAt: "2026-10-26T10:05:00Z"),             // 同時刻は id 順
        ]
        XCTAssertEqual(ChatThread.ordered(records).map(\.id), ["old", "a", "b", "c", "local-1"])
    }

    func testIsMineComparesAuthorWithUid() throws {
        let shared = try entity(scope: "public")
        XCTAssertTrue(ChatThread.isMine(Record(id: "1", data: [:], author: "me"), in: shared, uid: "me"))
        XCTAssertFalse(ChatThread.isMine(Record(id: "2", data: [:], author: "other"), in: shared, uid: "me"))
        XCTAssertTrue(ChatThread.isMine(Record(id: "3", data: [:]), in: shared, uid: "me"))       // 未送信の自分
        XCTAssertFalse(ChatThread.isMine(Record(id: "4", data: [:], author: "me"), in: shared, uid: nil)) // uid 不明
        XCTAssertTrue(ChatThread.isMine(Record(id: "5", data: [:], author: "other"), in: try entity(scope: nil), uid: "me")) // private は全部自分
    }

    func testSenderNameOnlyOnFirstOfARun() throws {
        let e = try entity(scope: "public")
        let a1 = Record(id: "1", data: [:], author: "sakura")
        let a2 = Record(id: "2", data: [:], author: "sakura")
        let b = Record(id: "3", data: [:], author: "takeshi")
        let me = Record(id: "4", data: [:], author: "me")
        XCTAssertTrue(ChatThread.showsSender(a1, after: nil, in: e, uid: "me"))
        XCTAssertFalse(ChatThread.showsSender(a2, after: a1, in: e, uid: "me"))
        XCTAssertTrue(ChatThread.showsSender(b, after: a2, in: e, uid: "me"))
        XCTAssertTrue(ChatThread.showsSender(a1, after: me, in: e, uid: "me"))
        XCTAssertFalse(ChatThread.showsSender(me, after: b, in: e, uid: "me")) // 自分の分には出さない
        XCTAssertFalse(ChatThread.showsSender(a1, after: nil, in: try entity(scope: nil), uid: "me"))
    }

    func testMessageData() {
        XCTAssertNil(ChatThread.message(text: "  \n ", photo: nil, textField: "text", photoField: "photo"))
        XCTAssertEqual(ChatThread.message(text: " こんにちは ", photo: nil, textField: "text", photoField: "photo", base: ["kind": "msg"]),
                       ["text": "こんにちは", "kind": "msg"])
        XCTAssertEqual(ChatThread.message(text: "", photo: "data:image/jpeg;base64,AA", textField: "text", photoField: "photo"),
                       ["photo": "data:image/jpeg;base64,AA"])
        // photoField の無い部品では写真を捨てる
        XCTAssertNil(ChatThread.message(text: "", photo: "data:image/jpeg;base64,AA", textField: "text", photoField: nil))
    }
}
