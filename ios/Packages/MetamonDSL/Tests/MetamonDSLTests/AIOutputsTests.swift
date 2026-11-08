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

    func testKanjiMultipliers() throws {
        let score = try field("score"), rate = try field("rate")
        XCTAssertEqual(AIOutputs.value("3万円", field: rate), .number(30000), "「3万円」を 3 にしない")
        XCTAssertEqual(AIOutputs.value("約 1.5万人", field: rate), .number(15000))
        XCTAssertEqual(AIOutputs.value("３万５０００円", field: rate), .number(35000))
        XCTAssertEqual(AIOutputs.value("1億2000万", field: rate), .number(120_000_000))
        XCTAssertEqual(AIOutputs.value("3千万円", field: rate), .number(30_000_000))
        XCTAssertEqual(AIOutputs.value("5千", field: score), .number(5000))
        XCTAssertNil(AIOutputs.value("3万〜5万円", field: rate), "範囲は読まない")
        XCTAssertNil(AIOutputs.value("3万円以上", field: rate), "残りが曖昧なら空")
        XCTAssertNil(AIOutputs.value("千円札", field: rate))
        var thousand = try field("rate")
        thousand.unit = "千円"
        XCTAssertEqual(AIOutputs.value("3千円", field: thousand), .number(3), "単位が千円なら単位どおり")
    }

    func testYearlessDatesRollToNextYear() throws {
        let day = try field("day"), start = try field("start")
        let dec = ISO8601DateFormatter().date(from: "2026-12-20T03:00:00Z")!
        XCTAssertEqual(AIOutputs.value("1/10", field: day, now: dec), .string("2027-01-10"), "12 月の「1/10」は来年")
        XCTAssertEqual(AIOutputs.value("1月10日", field: day, now: dec), .string("2027-01-10"))
        XCTAssertEqual(AIOutputs.value("12/1", field: day, now: dec), .string("2026-12-01"), "少し前は今年のまま")
        XCTAssertEqual(AIOutputs.value("2026/1/10", field: day, now: dec), .string("2026-01-10"), "年があればそのまま")
        let jan = try XCTUnwrap(AIOutputs.value("1/10 18:00", field: start, now: dec)?.stringValue.flatMap { FieldCoding.date(from: .string($0)) })
        XCTAssertEqual(Calendar(identifier: .gregorian).dateComponents(in: FieldCoding.dateFormatter.timeZone, from: jan).year, 2027)
    }

    func testDateFieldKeepsGivenOffsetDay() throws {
        let day = try field("day")
        XCTAssertEqual(AIOutputs.value("2026-11-20T23:30:00-05:00", field: day, now: now), .string("2026-11-20"), "TZ で日をずらさない")
        XCTAssertEqual(AIOutputs.value("2026-11-20T00:30:00+09:00", field: day, now: now), .string("2026-11-20"))
        XCTAssertNil(AIOutputs.value("2026-02-30T10:00:00Z", field: day, now: now))
    }

    func testApplyDoesNotOverwrite() throws {
        let e = try entity()
        let fills = AIOutputs.fills([AITextField(name: "題名", value: "秋の音楽祭"), AITextField(name: "点数", value: "85"),
                                     AITextField(name: "種類", value: "展示"), AITextField(name: "日時", value: "不明")],
                                    outputs: ["題名": "title", "点数": "score", "種類": "kind", "日時": "start"], entity: e, now: now)
        var values: [String: JSONValue] = ["title": .string("自分で書いた題"), "score": .null, "kind": .string("講演"), "start": .string("2026-11-20T09:00:00Z")]
        let r = AIOutputs.apply(fills, to: &values) { id, _ in id == "kind" }
        XCTAssertEqual(r.applied.map(\.field), ["score", "kind"])
        XCTAssertEqual(r.kept.map(\.fieldName), ["題名"])
        XCTAssertEqual(values["title"], .string("自分で書いた題"), "入力済みは上書きしない")
        XCTAssertEqual(values["score"], .number(85))
        XCTAssertEqual(values["kind"], .string("展示"), "入れ替えてよい値(初期値・前回の AI)は入れる")
        XCTAssertEqual(values["start"], .string("2026-11-20T09:00:00Z"), "読めない値は触らない")
        var same: [String: JSONValue] = ["title": .string("秋の音楽祭")]
        XCTAssertEqual(AIOutputs.apply(Array(fills.prefix(1)), to: &same) { _, _ in false }.kept, [], "同じ値は入れなかった扱いにしない")
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

    func testFormatsAndHint() throws {
        let e = try entity()
        XCTAssertNil(AIOutputs.formats(items: ["題名"], outputs: ["題名": "title"], entity: e))
        let f = AIOutputs.formats(items: ["点数", "日", "種類", "題名"], outputs: ["点数": "score", "日": "day", "種類": "kind", "題名": "title"], entity: e)
        XCTAssertEqual(f, [AITextFormat(item: "点数", kind: "number"), AITextFormat(item: "日", kind: "date"),
                           AITextFormat(item: "種類", kind: "enum", options: ["講演", "展示", "Live"])])
        XCTAssertEqual(AIOutputs.hint(Array(f!.prefix(2)), english: true),
                       "Format: 点数=number only / 日=YYYY/M/D (M/D if no year) (unknown: -)")
        XCTAssertEqual(AIOutputs.hint([f![2]], english: false), "書き方: 種類=講演/展示/Live のどれか(分からなければ -)")
        XCTAssertNil(AIOutputs.hint(nil, english: false))
    }
}
