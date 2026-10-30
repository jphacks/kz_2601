import XCTest
@testable import MetamonDSL

/// #417 aiButton generate: 出力の形の解釈(backend `parseTextOutput` と同じ規則)と DSL の読み込み。
final class AITextTests: XCTestCase {
    private let line = AITextRequest(instruction: "励まし", shape: .line)
    private let list = AITextRequest(instruction: "罰ゲーム", shape: .list, count: 2)
    private let fields = AITextRequest(instruction: "献立", shape: .fields, items: ["主菜", "汁物"])

    func testLineTakesFirstNonEmptyLine() {
        let r = AITextResult.normalize(text: "\n「今日もよくがんばったね」\n2 行目", items: nil, fields: nil, for: line)
        XCTAssertEqual(r, AITextResult(shape: .line, text: "今日もよくがんばったね"))
        XCTAssertNil(AITextResult.normalize(text: "  ", items: nil, fields: nil, for: line))
    }

    func testListStripsBulletsAndCutsToCount() {
        let r = AITextResult.normalize(text: nil, items: ["- 変顔", "2. 歌を一節", "", "モノマネ"], fields: nil, for: list)
        XCTAssertEqual(r?.items, ["変顔", "歌を一節"])
        XCTAssertEqual(r?.text, "・変顔\n・歌を一節")
        XCTAssertNil(AITextResult.normalize(text: nil, items: [" "], fields: nil, for: list))
        XCTAssertEqual(AITextRequest(instruction: "x", shape: .list).resolvedCount, AITextLimits.defaultCount)
        XCTAssertEqual(AITextRequest(instruction: "x", shape: .list, count: 99).resolvedCount, AITextLimits.maxItems)
    }

    func testFieldsNeedEveryNameInRequestOrder() {
        let r = AITextResult.normalize(text: nil, items: nil, fields: [
            AITextField(name: "汁物", value: "味噌汁"), AITextField(name: "主菜", value: "鮭"), AITextField(name: "おまけ", value: "x"),
        ], for: fields)
        XCTAssertEqual(r?.fields?.map(\.name), ["主菜", "汁物"])
        XCTAssertEqual(r?.text, "主菜: 鮭\n汁物: 味噌汁")
        XCTAssertNil(AITextResult.normalize(text: nil, items: nil, fields: [AITextField(name: "主菜", value: "鮭")], for: fields))
    }

    func testParseServerAndOnDeviceJSON() throws {
        let body = #"{"shape":"list","text":"・a\n・b\n・c","items":["a","b","c"],"locale":"ja","meta":{"model":"m"}}"#
        XCTAssertEqual(AITextResult.parseServer(Data(body.utf8), for: list)?.items, ["a", "b"], "サーバーの答えも count で切る")
        XCTAssertNil(AITextResult.parseServer(Data("{}".utf8), for: list))
        XCTAssertNil(AITextResult.parseServer(Data("oops".utf8), for: list))

        XCTAssertEqual(AITextResult.parseOnDevice(json: #"{"f0":"鮭","f1":"味噌汁"}"#, for: fields)?.text, "主菜: 鮭\n汁物: 味噌汁")
        XCTAssertEqual(AITextResult.parseOnDevice(json: #"{"items":["x"]}"#, for: list)?.items, ["x"])
        XCTAssertEqual(AITextResult.parseOnDevice(json: #"{"text":"ok"}"#, for: line)?.text, "ok")
        XCTAssertNil(AITextResult.parseOnDevice(json: "not json", for: line))
    }

    func testRequestEncodesLikeOpenAPI() throws {
        let data = try JSONEncoder().encode(AITextRequest(instruction: "献立", shape: .fields, items: ["主菜"], input: "- 鮭", locale: "ja"))
        let obj = try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(obj["shape"] as? String, "fields")
        XCTAssertEqual(obj["items"] as? [String], ["主菜"])
        XCTAssertNil(obj["count"], "nil は送らない")
    }

    func testClipKeepsWholeCharacters() {
        let s = String(repeating: "あ", count: 2000) // 6000 バイト
        let clipped = AITextLimits.clip(s)
        XCTAssertLessThanOrEqual(clipped.utf8.count, AITextLimits.maxInputBytes)
        XCTAssertEqual(clipped.utf8.count % 3, 0, "文字の途中で切らない")
        XCTAssertEqual(AITextLimits.clip("short"), "short")
    }

    func testExampleDecodesGenerateKeys() throws {
        let dsl = try ExamplesFixture.load("ai_ideas")
        let menu = try XCTUnwrap(dsl.screen("fridge")?.components.first { $0.id == "menu" })
        XCTAssertEqual(menu.type, .aiButton)
        XCTAssertEqual(menu.task, "generate")
        XCTAssertEqual(menu.shape, "fields")
        XCTAssertEqual(menu.items, ["主菜", "副菜", "汁物"])
        XCTAssertEqual(menu.limit, 20)
        let batsu = try XCTUnwrap(dsl.screen("home")?.components.first { $0.id == "batsu" })
        XCTAssertEqual(batsu.count, 5)
        XCTAssertTrue(DSLExamples.names.contains("ai_ideas"))
    }
}
