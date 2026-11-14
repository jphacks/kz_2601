import XCTest
@testable import MetamonDSL

/// `aiChat.saveTo`: 依頼に付ける field の形・案の値の確かめ直し・保存する data。
final class AIChatSaveToTests: XCTestCase {
    private func meal() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"meal","name":" 食事 ","fields":[
          {"id":"dish","name":"料理","type":"string","required":true},
          {"id":"date","name":"日付","type":"date","required":true,"defaultValue":"now"},
          {"id":"kind","name":"区分","type":"enum","options":["朝食","昼食","夕食"]},
          {"id":"kcal","name":"カロリー","type":"number","unit":" kcal "},
          {"id":"memo","name":"メモ","type":"text"},
          {"id":"done","name":"済","type":"boolean"}]}
        """#.utf8))
    }

    private let saveTo = AIChatSaveTo(entity: "meal", fields: ["dish", "date", "kind", "kcal"])

    func testDecodesOnlyOnAiChatShape() throws {
        let chat = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiChat","entity":"turn","textField":"text","persona":"p","saveTo":{"entity":"meal","fields":["dish","date"]}}"#.utf8))
        XCTAssertEqual(chat.chatSaveTo, AIChatSaveTo(entity: "meal", fields: ["dish", "date"]))
        XCTAssertNil(chat.saveTo, "aiButton の形(field)ではない")
        let button = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiButton","task":"generate","prompt":"p","shape":"list","saveTo":{"entity":"card","field":"q"}}"#.utf8))
        XCTAssertNil(button.chatSaveTo)
        XCTAssertEqual(button.saveTo, AISaveTo(entity: "card", field: "q"))
    }

    /// backend `chatSaveToOf` と同じ値(名前の前後の空白を落とす・required は defaultValue の無い必須だけ・options は enum だけ)。
    func testRequestMatchesTheBackendShape() throws {
        let today = try XCTUnwrap(FieldCoding.dateFormatter.date(from: "2026-11-08"))
        let r = try XCTUnwrap(saveTo.request(in: try meal(), today: today))
        XCTAssertEqual(r.entity, "meal")
        XCTAssertEqual(r.name, "食事")
        XCTAssertEqual(r.today, "2026-11-08")
        XCTAssertEqual(r.fields, [
            AITextSaveField(id: "dish", name: "料理", type: "string", required: true),
            AITextSaveField(id: "date", name: "日付", type: "date"),
            AITextSaveField(id: "kind", name: "区分", type: "enum", options: ["朝食", "昼食", "夕食"]),
            AITextSaveField(id: "kcal", name: "カロリー", type: "number", unit: "kcal"),
        ])
        let json = String(decoding: try JSONEncoder().encode(AIChat.request(persona: "p", history: [], next: "カレー", long: false, locale: "ja", saveTo: r)), as: UTF8.self)
        XCTAssertTrue(json.contains(#""saveTo":{"#), json)
        XCTAssertNil(AIChatSaveTo(entity: "meal", fields: ["done"]).request(in: try meal()), "boolean は使えない")
        XCTAssertNil(AIChatSaveTo(entity: "other", fields: ["dish"]).request(in: try meal()))
    }

    func testRecordChecksValuesAndFillsDefaults() throws {
        let e = try meal()
        let data = try XCTUnwrap(saveTo.record(["dish": .string(" カレー "), "kind": .string("ディナー"), "kcal": .number(800), "memo": .string("x")], in: e))
        XCTAssertEqual(data["dish"], .string("カレー"))
        XCTAssertNil(data["memo"], "saveTo に無い field の案は使わない")
        XCTAssertEqual(data["kind"], .string("朝食"), "options に無い値は落とし、既定(最初の選択肢)")
        XCTAssertEqual(data["kcal"], .number(800))
        XCTAssertNotNil(data["date"]?.stringValue, "date は既定の今日")
        XCTAssertEqual(data["done"], .bool(false))
        XCTAssertEqual(saveTo.values(["dish": .string("カレー"), "kcal": .number(1e12)], in: e).map(\.field.id), ["dish"])
        XCTAssertNil(saveTo.record(["kind": .string("夕食")], in: e), "required の料理が無い")
        XCTAssertNil(saveTo.record([:], in: e))
        XCTAssertNil(saveTo.record(["date": .string("11/8"), "dish": .string(" ")], in: e))
    }

    func testServerSuggestIsKeptOnlyForChatWithSaveTo() throws {
        let body = Data(#"{"shape":"line","task":"chat","text":"カレーですね","suggest":{"dish":"カレー","kcal":800},"locale":"ja","meta":{}}"#.utf8)
        let field = AITextSaveField(id: "dish", name: "料理", type: "string")
        let withSave = AIChat.request(persona: "p", history: [], next: "カレー", long: false, locale: "ja",
                                      saveTo: AITextSaveTo(entity: "meal", name: nil, today: "2026-11-08", fields: [field]))
        let r = try XCTUnwrap(AITextResult.parseServer(body, for: withSave))
        XCTAssertEqual(r.text, "カレーですね")
        XCTAssertEqual(r.suggest?["dish"], .string("カレー"))
        XCTAssertEqual(r.suggest?["kcal"], .number(800))
        let plain = AIChat.request(persona: "p", history: [], next: "カレー", long: false, locale: "ja")
        XCTAssertNil(try XCTUnwrap(AITextResult.parseServer(body, for: plain)).suggest)
    }
}
