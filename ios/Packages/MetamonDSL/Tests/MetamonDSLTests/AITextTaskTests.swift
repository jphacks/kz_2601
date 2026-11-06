import XCTest
@testable import MetamonDSL

/// aiButton の要約・分類などのオンライン版(`POST /v1/text` の task): 答えの解釈(backend `parseTaskOutput` と同じ規則)と依頼の形。
final class AITextTaskTests: XCTestCase {
    private let summarize = AITextRequest(instruction: "", shape: .line, input: "日記", task: "summarize")
    private let classify = AITextRequest(instruction: "", shape: .line, input: "会議", task: "classify", options: ["仕事", "趣味"])

    func testTransformKeepsWholeTextAndCaps() {
        let r = AITextResult.normalize(text: "「雨の一日。\n午後は読書。」", items: nil, fields: nil, for: summarize)
        XCTAssertEqual(r?.text, "雨の一日。\n午後は読書。", "1 行目だけにしない")
        let long = String(repeating: "あ", count: AITextLimits.maxTaskCharacters + 10)
        XCTAssertEqual(AITextResult.normalize(text: long, items: nil, fields: nil, for: summarize)?.text.count, AITextLimits.maxTaskCharacters)
        XCTAssertNil(AITextResult.normalize(text: " ", items: nil, fields: nil, for: summarize))
    }

    func testClassifyMustNameOneOption() {
        XCTAssertEqual(AITextResult.normalize(text: "仕事", items: nil, fields: nil, for: classify)?.text, "仕事")
        XCTAssertEqual(AITextResult.normalize(text: "趣味です", items: nil, fields: nil, for: classify)?.text, "趣味")
        XCTAssertNil(AITextResult.normalize(text: "家事", items: nil, fields: nil, for: classify))
        XCTAssertNil(AITextResult.normalize(text: "仕事と趣味", items: nil, fields: nil, for: classify))
    }

    func testServerAnswerWithTask() {
        let body = #"{"shape":"line","task":"summarize","text":"雨。\n読書。","locale":"ja","meta":{"model":"m"}}"#
        XCTAssertEqual(AITextResult.parseServer(Data(body.utf8), for: summarize)?.text, "雨。\n読書。")
    }

    func testRequestEncodesTaskOnlyWhenSet() throws {
        let generate = try JSONSerialization.jsonObject(with: JSONEncoder().encode(AITextRequest(instruction: "x", shape: .line))) as? [String: Any]
        XCTAssertNil(generate?["task"], "generate は task を送らない(旧サーバーとも同じ形)")
        XCTAssertNil(generate?["options"])
        let c = try JSONSerialization.jsonObject(with: JSONEncoder().encode(classify)) as? [String: Any]
        XCTAssertEqual(c?["task"] as? String, "classify")
        XCTAssertEqual(c?["options"] as? [String], ["仕事", "趣味"])
        XCTAssertTrue(classify.isTransform)
        XCTAssertFalse(AITextRequest(instruction: "x", shape: .line, task: "generate").isTransform)
    }
}
