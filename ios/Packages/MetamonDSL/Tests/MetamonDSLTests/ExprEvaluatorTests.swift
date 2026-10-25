import XCTest
@testable import MetamonDSL

final class ExprEvaluatorTests: XCTestCase {
    private func expr(_ json: String) throws -> Expr { try JSONDecoder().decode(Expr.self, from: Data(json.utf8)) }

    private func eval(_ json: String, row: ExprEvaluator.Row? = nil, records: [String: [Record]] = [:], entities: [Entity] = [], now: Date = Date()) throws -> ExprValue? {
        try ExprEvaluator(entities: entities, records: { records[$0] ?? [] }, now: now).evaluate(expr(json), row: row)
    }

    private func num(_ json: String) throws -> Double? { try eval(json)?.number }

    // MARK: - 算術

    func testArithmetic() throws {
        XCTAssertEqual(try num(#"{"op":"add","args":[1,2,3]}"#), 6)
        XCTAssertEqual(try num(#"{"op":"sub","args":[10,4]}"#), 6)
        XCTAssertEqual(try num(#"{"op":"mul","args":[2,3,4]}"#), 24)
        XCTAssertEqual(try num(#"{"op":"div","args":[9,3]}"#), 3)
        XCTAssertEqual(try num(#"{"op":"mod","args":[10,3]}"#), 1)
        XCTAssertEqual(try num(#"{"op":"neg","args":[5]}"#), -5)
        XCTAssertEqual(try num(#"{"op":"abs","args":[-5]}"#), 5)
        XCTAssertEqual(try num(#"{"op":"sqrt","args":[16]}"#), 4)
        XCTAssertEqual(try num(#"{"op":"pow","args":[2,10]}"#), 1024)
        XCTAssertEqual(try num(#"{"op":"round","args":[2.5]}"#), 3)
        XCTAssertEqual(try num(#"{"op":"round","args":[3.14159,2]}"#), 3.14)
        XCTAssertEqual(try num(#"{"op":"floor","args":[2.9]}"#), 2)
        XCTAssertEqual(try num(#"{"op":"ceil","args":[2.1]}"#), 3)
        XCTAssertEqual(try num(#"{"op":"min","args":[3,1,2]}"#), 1)
        XCTAssertEqual(try num(#"{"op":"max","args":[3,1,2]}"#), 3)
        XCTAssertEqual(try num(#"{"op":"clamp","args":[15,0,10]}"#), 10)
        XCTAssertEqual(try num(#"{"op":"clamp","args":[-1,0,10]}"#), 0)
        XCTAssertEqual(try num(#"{"op":"add","args":["1,200",300]}"#), 1500, "数字文字列は数値として扱う")
    }

    func testUnavailableIsNil() throws {
        XCTAssertNil(try eval(#"{"op":"div","args":[1,0]}"#), "ゼロ除算")
        XCTAssertNil(try eval(#"{"op":"mod","args":[1,0]}"#))
        XCTAssertNil(try eval(#"{"op":"sqrt","args":[-1]}"#))
        XCTAssertNil(try eval(#"{"op":"add","args":[1,"abc"]}"#), "型不一致")
        XCTAssertNil(try eval(#"{"op":"add","args":[1,{"ref":"missing"}]}"#, row: .init(entity: nil, record: Record(id: "r", data: [:]))), "参照なし")
        XCTAssertNil(try eval(#"{"ref":"x"}"#), "行コンテキスト無しの ref")
        XCTAssertNil(try eval(#"{"op":"nope","args":[1]}"#), "未知 op")
        XCTAssertNil(try eval(#"{"op":"mul","args":[{"op":"div","args":[1,0]},2]}"#), "nil は伝播する")
    }

    // MARK: - 比較・論理・if

    func testComparisonLogicAndIf() throws {
        XCTAssertEqual(try eval(#"{"op":"eq","args":[1,1]}"#), .bool(true))
        XCTAssertEqual(try eval(#"{"op":"eq","args":["a","a"]}"#), .bool(true))
        XCTAssertEqual(try eval(#"{"op":"neq","args":[1,2]}"#), .bool(true))
        XCTAssertEqual(try eval(#"{"op":"gt","args":[2,1]}"#), .bool(true))
        XCTAssertEqual(try eval(#"{"op":"gte","args":[2,2]}"#), .bool(true))
        XCTAssertEqual(try eval(#"{"op":"lt","args":[1,2]}"#), .bool(true))
        XCTAssertEqual(try eval(#"{"op":"lte","args":[3,2]}"#), .bool(false))
        XCTAssertEqual(try eval(#"{"op":"and","args":[true,{"op":"gt","args":[2,1]}]}"#), .bool(true))
        XCTAssertEqual(try eval(#"{"op":"or","args":[false,false]}"#), .bool(false))
        XCTAssertEqual(try eval(#"{"op":"not","args":[false]}"#), .bool(true))
        XCTAssertEqual(try eval(#"{"if":{"op":"gt","args":[5,3]},"then":"大","else":"小"}"#), .string("大"))
        XCTAssertEqual(try eval(#"{"if":{"op":"div","args":[1,0]},"then":"x","else":"y"}"#), .string("y"), "nil 条件は false")
    }

    func testStringsAndConversion() throws {
        XCTAssertEqual(try eval(#"{"op":"concat","args":["A","→",12]}"#), .string("A→12"))
        XCTAssertEqual(try num(#"{"op":"len","args":["割り勘"]}"#), 3)
        XCTAssertEqual(try num(#"{"op":"toNumber","args":["42"]}"#), 42)
        XCTAssertEqual(try eval(#"{"op":"toString","args":[42]}"#), .string("42"))
        XCTAssertEqual(try num(#"{"op":"coalesce","args":[{"op":"div","args":[1,0]},7]}"#), 7)
    }

    // MARK: - 日付

    func testDates() throws {
        let now = FieldCoding.dateFormatter.date(from: "2026-10-24")!
        func d(_ json: String) throws -> ExprValue? { try eval(json, now: now) }
        XCTAssertEqual(try d(#"{"op":"daysBetween","args":["2026-10-01","2026-10-24"]}"#)?.number, 23)
        XCTAssertEqual(try d(#"{"op":"daysUntil","args":["2026-12-24"]}"#)?.number, 61)
        XCTAssertEqual(try d(#"{"op":"toString","args":[{"op":"addDays","args":[{"op":"today"},7]}]}"#), .string("2026-10-31"))
        XCTAssertEqual(try d(#"{"op":"year","args":["2026-10-24"]}"#)?.number, 2026)
        XCTAssertEqual(try d(#"{"op":"month","args":["2026-10-24"]}"#)?.number, 10)
        XCTAssertEqual(try d(#"{"op":"day","args":["2026-10-24"]}"#)?.number, 24)
        XCTAssertEqual(try d(#"{"op":"weekday","args":["2026-10-24"]}"#)?.number, 7, "Calendar 準拠: 日=1 … 土=7")
        XCTAssertEqual(try d(#"{"op":"gt","args":[{"op":"now"},"2026-01-01"]}"#), nil, "date と文字列の比較は型不一致")
        XCTAssertEqual(try d(#"{"op":"gt","args":[{"op":"now"},{"op":"addDays","args":["2026-01-01",0]}]}"#), .bool(true))
    }

    func testDeepNesting() throws {
        var json = "1"
        for _ in 0..<30 { json = #"{"op":"add","args":[\#(json),1]}"# }
        XCTAssertEqual(try num(json), 31)
        var tooDeep = "1"
        for _ in 0..<(ExprEvaluator.maxDepth + 5) { tooDeep = #"{"op":"neg","args":[\#(tooDeep)]}"# }
        XCTAssertNil(try num(tooDeep), "上限を超えた入れ子は nil(スタックを守る)")
    }

    // MARK: - agg + 行コンテキスト(割り勘)

    private func warikan() throws -> (AppDSL, [String: [Record]]) {
        let dsl = try ExamplesFixture.load("warikan")
        let records: [String: [Record]] = [
            "members": [
                Record(id: "m1", data: ["name": "たろう"]),
                Record(id: "m2", data: ["name": "はなこ"]),
                Record(id: "m3", data: ["name": "じろう"]),
            ],
            "expenses": [
                Record(id: "e1", data: ["title": "宿", "amount": 9000, "payer": "m1"]),
                Record(id: "e2", data: ["title": "夕食", "amount": 4500, "payer": "m2"]),
                Record(id: "e3", data: ["title": "ガソリン", "amount": 1500, "payer": "m1"]),
            ],
        ]
        return (dsl, records)
    }

    func testWarikanComputedFields() throws {
        let (dsl, records) = try warikan()
        let ev = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        let members = ev.viewRecords(for: "members")
        let byName = Dictionary(uniqueKeysWithValues: members.map { ($0["name"].stringValue!, $0) })
        // 合計 15000 / 3 人 = 5000
        XCTAssertEqual(byName["たろう"]?["paid"].doubleValue, 10500)
        XCTAssertEqual(byName["はなこ"]?["paid"].doubleValue, 4500)
        XCTAssertEqual(byName["じろう"]?["paid"].doubleValue, 0)
        XCTAssertEqual(byName["たろう"]?["share"].doubleValue, 5000)
        XCTAssertEqual(byName["たろう"]?["balance"].doubleValue, 5500)
        XCTAssertEqual(byName["はなこ"]?["balance"].doubleValue, -500)
        XCTAssertEqual(byName["じろう"]?["balance"].doubleValue, -5000)
        XCTAssertNil(records["members"]?[0].data["balance"], "computed は保存レコードに入れない")

        // agg の field に computed を指定: balance の合計は 0
        let sum = ev.evaluate(.agg(AggExpr(op: "sum", entity: "members", field: "balance")))
        XCTAssertEqual(sum?.number, 0)

        let t = Settlement.transfers(members.map { ($0["name"].stringValue!, $0["balance"].doubleValue!) })
        XCTAssertEqual(t, [.init(from: "じろう", to: "たろう", amount: 5000), .init(from: "はなこ", to: "たろう", amount: 500)])
    }

    /// 正典 warikan.json のホーム: stat 合計(aggregate)¥15,000 / stat 1人あたり(value)¥5,000 / 支出の per_person。
    func testWarikanHomeStatsAndPerPerson() throws {
        let (dsl, records) = try warikan()
        let ev = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        let stats = try XCTUnwrap(dsl.screen("home")?.components.first?.children)
        XCTAssertEqual(stats.map(\.label), ["合計", "1人あたり"])
        let total = Aggregator.aggregate(stats[0].aggregate, field: stats[0].field, over: ev.viewRecords(for: "expenses"))
        XCTAssertEqual(total, 15000)
        XCTAssertEqual(Aggregator.format(total, as: stats[0].format), "¥15,000")
        XCTAssertNil(stats[1].aggregate)
        let perPerson = try XCTUnwrap(ev.evaluate(try XCTUnwrap(stats[1].valueExpr))?.number)
        XCTAssertEqual(perPerson, 5000)
        XCTAssertEqual(Aggregator.format(perPerson, as: stats[1].format), "¥5,000")
        let expenses = Dictionary(uniqueKeysWithValues: ev.viewRecords(for: "expenses").map { ($0.id, $0) })
        XCTAssertEqual(expenses["e1"]?["per_person"].doubleValue, 3000)
        XCTAssertEqual(expenses["e2"]?["per_person"].doubleValue, 1500)
        // 支出フォームの payer は picker(ref)、computed に束縛した入力部品は無い
        let form = try XCTUnwrap(dsl.screen("expense_form")?.components.first?.children)
        XCTAssertEqual(form.first { $0.field == "payer" }?.type, .picker)
        XCTAssertFalse(form.contains { $0.field == "per_person" })
    }

    /// 正典 bmi.json: 体重 65kg / 身長 170cm → 22.5(小数 1 桁)。
    func testBMIExample() throws {
        let dsl = try ExamplesFixture.load("bmi")
        let records = ["body": [
            Record(id: "b1", data: ["date": "2026-10-24", "weight": 65, "height": 170]),
            Record(id: "b2", data: ["date": "2026-10-23", "weight": 70, "height": 175]),
        ]]
        let ev = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        let rows = Dictionary(uniqueKeysWithValues: ev.viewRecords(for: "body").map { ($0.id, $0) })
        XCTAssertEqual(rows["b1"]?["bmi"].doubleValue, 22.5)
        XCTAssertEqual(rows["b2"]?["bmi"].doubleValue, 22.9)
        let avgStat = try XCTUnwrap(dsl.screen("home")?.components.first)
        XCTAssertEqual(Aggregator.aggregate(avgStat.aggregate, field: avgStat.field, over: ev.viewRecords(for: "body")), 22.7, accuracy: 1e-9)
    }

    func testAggFilterArrayAndEmpty() throws {
        let (dsl, records) = try warikan()
        let ev = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        let e = try expr(#"{"agg":"sum","entity":"expenses","field":"amount","filter":[{"field":"payer","op":"eq","value":"m1"},{"field":"amount","op":"gt","value":2000}]}"#)
        XCTAssertEqual(ev.evaluate(e)?.number, 9000)
        XCTAssertNil(ev.evaluate(try expr(#"{"agg":"avg","entity":"nothing","field":"x"}"#)), "空の avg は nil")
        XCTAssertEqual(ev.evaluate(try expr(#"{"agg":"count","entity":"nothing"}"#))?.number, 0)
    }

    func testZeroMembersShareIsNil() throws {
        let (dsl, _) = try warikan()
        let ev = ExprEvaluator(dsl: dsl, records: { _ in [] })
        let row = ExprEvaluator.Row(entity: dsl.entity("members"), record: Record(id: "x", data: [:]), memoizable: false)
        XCTAssertNil(ev.fieldValue("share", row: row), "0 人で割る = —")
        XCTAssertEqual(FieldCoding.display(.null, field: dsl.entity("members")?.field("share")), "—")
    }

    func testCycleIsNil() throws {
        let json = #"{"id":"e","name":"E","fields":[{"id":"a","name":"a","type":"number","computed":{"ref":"b"}},{"id":"b","name":"b","type":"number","computed":{"op":"add","args":[{"ref":"a"},1]}}]}"#
        let entity = try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
        let ev = ExprEvaluator(entities: [entity], records: { _ in [Record(id: "1", data: [:])] })
        XCTAssertNil(ev.viewRecords(for: "e")[0].data["a"] ?? nil)
    }

    // MARK: - Codable / 後方互換

    func testFilterBackwardCompatAndExprValue() throws {
        let legacy = try JSONDecoder().decode(Filter.self, from: Data(#"{"field":"category","op":"eq","value":"食費"}"#.utf8))
        XCTAssertEqual(legacy.value, "食費")
        XCTAssertNil(legacy.valueExpr)
        let modern = try JSONDecoder().decode(Filter.self, from: Data(#"{"field":"payer","op":"eq","value":{"ref":"id"}}"#.utf8))
        XCTAssertNil(modern.value)
        XCTAssertEqual(modern.valueExpr, .ref("id"))
        let round = try JSONDecoder().decode(Filter.self, from: JSONEncoder().encode(modern))
        XCTAssertEqual(round, modern)
    }

    func testExprRoundTrip() throws {
        let json = #"{"if":{"op":"gt","args":[{"ref":"balance"},0]},"then":{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"payer","op":"eq","value":{"ref":"id"}}},"else":null}"#
        let e = try expr(json)
        XCTAssertEqual(try JSONDecoder().decode(Expr.self, from: JSONEncoder().encode(e)), e)
        XCTAssertEqual(e.referencedEntities, ["expenses"])
        XCTAssertThrowsError(try expr(#"{"foo":1}"#))
    }

    func testSettlementEdgeCases() {
        XCTAssertEqual(Settlement.transfers([("A", 0), ("B", 0)]), [])
        XCTAssertEqual(Settlement.transfers([("A", 0.3), ("B", -0.3)]), [], "1 円未満は精算不要")
        // 1000 円を 3 人: 端数があっても転送は円に丸める
        let t = Settlement.transfers([("A", 666.67), ("B", -333.33), ("C", -333.34)])
        XCTAssertEqual(t.map(\.amount).reduce(0, +), 666)
        XCTAssertEqual(Set(t.map(\.to)), ["A"])
    }
}
