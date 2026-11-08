import XCTest
@testable import MetamonDSL

/// aiButton `outputs`: fields の項目を型に直してフォームに入れる(backend `ai_outputs.go`)。
final class AIOutputsTests: XCTestCase {
    private let now = ISO8601DateFormatter().date(from: "2026-11-08T03:00:00Z")!

    private func entity() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
            {"id":"ev","name":"予定","fields":[
             {"id":"title","name":"題名","type":"string"},
             {"id":"start","name":"日時","type":"datetime"},
             {"id":"day","name":"日","type":"date"},
             {"id":"score","name":"点数","type":"integer"},
             {"id":"rate","name":"率","type":"number"},
             {"id":"kind","name":"種類","type":"enum","options":["講演","展示","Live"]},
             {"id":"done","name":"済","type":"boolean"}]}
            """#.utf8))
    }

    private func field(_ id: String) throws -> Field { try XCTUnwrap(entity().field(id)) }

    func testComponentDecodesOutputs() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiButton","task":"generate","prompt":"x","shape":"fields","items":["題名"],"outputs":{"題名":"title"}}"#.utf8))
        XCTAssertEqual(c.outputs, ["題名": "title"])
        let broken = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiButton","task":"generate","prompt":"x","outputs":["title"]}"#.utf8))
        XCTAssertNil(broken.outputs, "読めない outputs は無視(表示だけ)")
    }

    func testNumbers() throws {
        let score = try field("score"), rate = try field("rate")
        XCTAssertEqual(AIOutputs.value("85", field: score), .number(85))
        XCTAssertEqual(AIOutputs.value("85 点", field: score), .number(85))
        XCTAssertEqual(AIOutputs.value("８５．６", field: score), .number(86), "integer は丸める")
        XCTAssertEqual(AIOutputs.value("約 3,000 円", field: rate), .number(3000))
        XCTAssertEqual(AIOutputs.value("0.75", field: rate), .number(0.75))
        XCTAssertNil(AIOutputs.value("85/100", field: score), "数が 2 つは読まない")
        XCTAssertNil(AIOutputs.value("よくできました", field: score))
    }

    func testDates() throws {
        let start = try field("start"), day = try field("day")
        let a = AIOutputs.value("2026/11/20 18:00", field: start, now: now)
        XCTAssertEqual(a, AIOutputs.value("2026-11-20T18:00", field: start, now: now))
        XCTAssertNotNil(a)
        XCTAssertEqual(AIOutputs.value("11/20(金) 18:00〜20:00", field: start, now: now), a, "年なしは今年・範囲は始まり")
        XCTAssertEqual(AIOutputs.value("2026年11月20日(金)18時", field: start, now: now), a)
        XCTAssertEqual(AIOutputs.value("11月20日", field: day, now: now), .string("2026-11-20"))
        XCTAssertNil(AIOutputs.value("来週のどこか", field: day, now: now))
        XCTAssertNil(AIOutputs.value("不明", field: day, now: now))
    }

    func testEnumAndText() throws {
        let kind = try field("kind"), title = try field("title")
        XCTAssertEqual(AIOutputs.value("展示", field: kind), .string("展示"))
        XCTAssertEqual(AIOutputs.value("live", field: kind), .string("Live"))
        XCTAssertNil(AIOutputs.value("ワークショップ", field: kind), "options に無ければ空")
        XCTAssertEqual(AIOutputs.value("  秋の音楽祭 \n", field: title), .string("秋の音楽祭"))
        XCTAssertNil(AIOutputs.value("-", field: title), "分からない答えは空")
        XCTAssertNil(AIOutputs.value("yes", field: try field("done")), "boolean は入れない")
    }

    func testFillsFollowResultOrderAndSkipUnmapped() throws {
        let fields = [AITextField(name: "題名", value: "秋の音楽祭"), AITextField(name: "メモ", value: "x"),
                      AITextField(name: "日時", value: "未定"), AITextField(name: "種類", value: "展示")]
        let fills = AIOutputs.fills(fields, outputs: ["題名": "title", "日時": "start", "種類": "kind", "済": "done"], entity: try entity(), now: now)
        XCTAssertEqual(fills.map(\.field), ["title", "start", "kind"])
        XCTAssertEqual(fills.map(\.fieldName), ["題名", "日時", "種類"])
        XCTAssertEqual(fills[0].value, .string("秋の音楽祭"))
        XCTAssertNil(fills[1].value, "読めない値は入れない")
        XCTAssertEqual(fills[2].value, .string("展示"))
    }

    func testHint() throws {
        let e = try entity()
        XCTAssertNil(AIOutputs.hint(items: ["題名"], outputs: ["題名": "title"], entity: e, english: false))
        XCTAssertEqual(AIOutputs.hint(items: ["点数", "日"], outputs: ["点数": "score", "日": "day"], entity: e, english: true),
                       "Format: 点数=number only / 日=YYYY/M/D (M/D if no year) (unknown: -)")
    }
}
