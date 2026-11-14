import XCTest
@testable import MetamonDSL

/// `aiChat.inputEntity`: DSL の読み取りと、参考の記録を付けた依頼(会話は残りの 16 KB に収める)。
final class AIChatInputTests: XCTestCase {
    func testDecodesInputEntityAndLimit() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"""
            {"type":"aiChat","entity":"turn","textField":"text","roleField":"role","persona":"p","inputEntity":"diary","limit":5}
            """#.utf8))
        XCTAssertEqual(c.inputEntity, "diary")
        XCTAssertEqual(c.limit, 5)
    }

    func testMaterialGoesWithTheRequestAndShrinksTheConversation() throws {
        let material = AITextMaterial(entity: "diary", name: "日記", text: String(repeating: "a", count: 8 * 1024))
        let history = (0..<10).map { AIChat.Turn(id: "t\($0)", role: $0.isMultiple(of: 2) ? .user : .assistant,
                                                 text: String(repeating: "b", count: 1000)) }
        let r = AIChat.request(persona: "p", history: history, next: "質問", long: false, locale: "ja", material: material)
        XCTAssertEqual(r.material, material)
        let used = (r.messages ?? []).reduce(0) { $0 + $1.text.utf8.count } + material.text.utf8.count
        XCTAssertLessThanOrEqual(used, AIChat.maxBytes, "会話と参考の記録で合わせて 16 KB まで")
        XCTAssertEqual(r.messages?.last?.text, "質問")
        let plain = AIChat.request(persona: "p", history: history, next: "質問", long: false, locale: "ja")
        XCTAssertNil(plain.material)
        XCTAssertGreaterThan(plain.messages?.count ?? 0, r.messages?.count ?? 0)

        let json = String(decoding: try JSONEncoder().encode(r), as: UTF8.self)
        XCTAssertTrue(json.contains(#""material":{"#))
        XCTAssertFalse(String(decoding: try JSONEncoder().encode(plain), as: UTF8.self).contains("material"))
    }
}
