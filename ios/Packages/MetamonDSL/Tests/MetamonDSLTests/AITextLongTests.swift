import XCTest
@testable import MetamonDSL

/// aiButton `length: long` の依頼と答え(backend `text_long.go` と同じ規則)。
final class AITextLongTests: XCTestCase {
    func testRequestEncodesLengthOnlyWhenLong() throws {
        let long = AITextRequest(instruction: "手紙", shape: .line, length: "long")
        XCTAssertTrue(long.isLong)
        XCTAssertTrue(String(decoding: try JSONEncoder().encode(long), as: UTF8.self).contains(#""length":"long""#))
        let short = AITextRequest(instruction: "一言", shape: .line, length: "short")
        XCTAssertFalse(short.isLong)
        XCTAssertNil(short.length, "short は送らない(既定)")
        XCTAssertFalse(String(decoding: try JSONEncoder().encode(short), as: UTF8.self).contains("length"))
    }

    func testLongAnswerKeepsParagraphsAndCaps() throws {
        let req = AITextRequest(instruction: "手紙", shape: .line, length: "long")
        let letter = "おばあちゃんへ\n\n元気ですか。\n\nまた行きます。"
        XCTAssertEqual(AITextResult.normalize(text: letter, items: nil, fields: nil, for: req)?.text, letter, "1 行目だけにしない")
        let huge = String(repeating: "あ", count: AITextLimits.maxLongCharacters + 10)
        XCTAssertEqual(AITextResult.normalize(text: huge, items: nil, fields: nil, for: req)?.text.count, AITextLimits.maxLongCharacters)
        XCTAssertNil(AITextResult.normalize(text: "  ", items: nil, fields: nil, for: req))

        let server = Data(#"{"shape":"line","task":"summarize","text":"段落1\n\n段落2","length":"long","locale":"ja","meta":{}}"#.utf8)
        let task = AITextRequest(instruction: "", shape: .line, input: "x", task: "summarize", length: "long")
        XCTAssertEqual(AITextResult.parseServer(server, for: task)?.text, "段落1\n\n段落2")
    }

    func testComponentDecodesLength() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiButton","task":"generate","prompt":"x","length":"long"}"#.utf8))
        XCTAssertEqual(c.length, "long")
        let plain = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiButton","task":"generate","prompt":"x"}"#.utf8))
        XCTAssertNil(plain.length)
    }
}
