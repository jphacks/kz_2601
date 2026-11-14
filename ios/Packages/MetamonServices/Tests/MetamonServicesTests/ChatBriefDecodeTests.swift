import XCTest
@testable import MetamonServices

/// 会話の要約(ChatBrief)と、アプリが受けられる応答の種類(ChatCapability)。openapi-ai.yaml の `ChatBrief` は 5 つの鍵が必須。
final class ChatBriefDecodeTests: XCTestCase {
    private func decode(_ json: String) throws -> ChatBrief {
        try JSONDecoder().decode(ChatBrief.self, from: Data(json.utf8))
    }

    func testDecodesAllKeys() throws {
        let b = try decode(#"{"name":"家計簿","purpose":"お金の記録","entities":["金額","カテゴリ"],"features":["月の合計"],"native":["camera","notification"]}"#)
        XCTAssertEqual(b, ChatBrief(name: "家計簿", purpose: "お金の記録", entities: ["金額", "カテゴリ"], features: ["月の合計"], native: ["camera", "notification"]))
    }

    func testEmptyArraysAreAccepted() throws {
        let b = try decode(#"{"name":"x","purpose":"y","entities":[],"features":[],"native":[]}"#)
        XCTAssertTrue(b.entities.isEmpty)
        XCTAssertTrue(b.features.isEmpty)
        XCTAssertTrue(b.native.isEmpty, "ネイティブ機能が無い時は空配列")
    }

    func testUnknownNativeKeywordsAreKeptAsIs() throws {
        let b = try decode(#"{"name":"x","purpose":"y","entities":[],"features":[],"native":["hologram"]}"#)
        XCTAssertEqual(b.native, ["hologram"], "知らない語でも落とさない")
    }

    func testUnknownExtraKeysAreIgnored() throws {
        let b = try decode(#"{"name":"x","purpose":"y","entities":[],"features":[],"native":[],"futureKey":1}"#)
        XCTAssertEqual(b.name, "x")
    }

    func testMissingKeyThrows() {
        let base: [String: String] = [
            "name": #""x""#, "purpose": #""y""#, "entities": "[]", "features": "[]", "native": "[]",
        ]
        for missing in base.keys.sorted() {
            let body = base.filter { $0.key != missing }.sorted { $0.key < $1.key }
                .map { #""\#($0.key)":\#($0.value)"# }.joined(separator: ",")
            XCTAssertThrowsError(try decode("{\(body)}"), "\(missing) が無いと読めない(契約の必須)") { error in
                guard case DecodingError.keyNotFound(let key, _) = error else { return XCTFail("keyNotFound のはず: \(error)") }
                XCTAssertEqual(key.stringValue, missing)
            }
        }
    }

    func testWrongTypeThrows() {
        XCTAssertThrowsError(try decode(#"{"name":"x","purpose":"y","entities":"金額","features":[],"native":[]}"#))
    }

    func testInitDefaultsAndRoundTrip() throws {
        let b = ChatBrief(name: "n", purpose: "p")
        XCTAssertEqual(b.entities, [])
        XCTAssertEqual(b.features, [])
        XCTAssertEqual(b.native, [])
        let again = try JSONDecoder().decode(ChatBrief.self, from: try JSONEncoder().encode(b))
        XCTAssertEqual(again, b)
    }

    func testChatTurnCarriesBrief() throws {
        let json = #"{"kind":"ready","text":"作ります","turn":2,"locale":"ja","source":"claude","transcript":"t","brief":{"name":"家計簿","purpose":"p","entities":[],"features":[],"native":[]}}"#
        let turn = try JSONDecoder().decode(ChatTurn.self, from: Data(json.utf8))
        XCTAssertEqual(turn.brief?.name, "家計簿")
    }

    func testCapabilitiesListOnlyWhatAppCanShow() {
        XCTAssertEqual(ChatCapability.all, ["answer", "proposal"])
        XCTAssertFalse(ChatCapability.all.contains("question"), "question / ready は常に出せるので載せない")
        XCTAssertFalse(ChatCapability.all.contains("ready"))
    }
}
