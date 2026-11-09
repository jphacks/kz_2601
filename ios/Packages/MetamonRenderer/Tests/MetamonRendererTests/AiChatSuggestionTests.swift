#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// `aiChat.saveTo` の記録の案のカード: 合う値だけ行に出し、保存する data を持つ。合わなければ出さない。
final class AiChatSuggestionTests: XCTestCase {
    private func meal() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"meal","name":"食事","fields":[
          {"id":"dish","name":"料理","type":"string","required":true},
          {"id":"kind","name":"区分","type":"enum","options":["朝食","昼食","夕食"]},
          {"id":"kcal","name":"カロリー","type":"number"}]}
        """#.utf8))
    }

    func testMakeKeepsFittingValues() throws {
        let saveTo = AIChatSaveTo(entity: "meal", fields: ["dish", "kind", "kcal"])
        let s = try XCTUnwrap(AiChatSuggestion.make(turnId: "t1", suggest: ["dish": .string("カレー"), "kind": .string("夕食"), "kcal": .string("多い")],
                                                    saveTo: saveTo, entity: try meal(), display: { v, _ in v.displayString }))
        XCTAssertEqual(s.turnId, "t1")
        XCTAssertEqual(s.entity, "meal")
        XCTAssertEqual(s.rows, [.init(name: "料理", value: "カレー"), .init(name: "区分", value: "夕食")])
        XCTAssertEqual(s.data["dish"], .string("カレー"))
        XCTAssertNil(s.data["kcal"], "数でない値は保存しない")
        XCTAssertEqual(s.state, .idle)
    }

    func testNoCardWithoutSuggestOrRequired() throws {
        let saveTo = AIChatSaveTo(entity: "meal", fields: ["dish", "kind"])
        XCTAssertNil(AiChatSuggestion.make(turnId: "t", suggest: nil, saveTo: saveTo, entity: try meal(), display: { v, _ in v.displayString }))
        XCTAssertNil(AiChatSuggestion.make(turnId: "t", suggest: ["kind": .string("夕食")], saveTo: saveTo, entity: try meal(),
                                           display: { v, _ in v.displayString }), "required の料理が無い")
        XCTAssertNil(AiChatSuggestion.make(turnId: "t", suggest: ["dish": .string("カレー")], saveTo: nil, entity: try meal(),
                                           display: { v, _ in v.displayString }))
    }
}
#endif
