import XCTest
@testable import MetamonDSL

/// Siri / ショートカット「記録を追加」(#80)のパラメータ解決。
final class QuickRecordTests: XCTestCase {
    func testParse() {
        XCTAssertEqual(QuickRecord.parse("ランチ 800円"), .init(title: "ランチ", number: 800, unit: "円"))
        XCTAssertEqual(QuickRecord.parse("ランチ８００円"), .init(title: "ランチ", number: 800, unit: "円"))
        XCTAssertEqual(QuickRecord.parse("ランチを1,200円"), .init(title: "ランチ", number: 1200, unit: "円"))
        XCTAssertEqual(QuickRecord.parse("￥980 コンビニ"), .init(title: "コンビニ", number: 980, unit: "円"))
        XCTAssertEqual(QuickRecord.parse("¥980 コンビニ"), .init(title: "コンビニ", number: 980, unit: "円"))
        XCTAssertEqual(QuickRecord.parse("体重 62.5kg"), .init(title: "体重", number: 62.5, unit: "kg"))
        XCTAssertEqual(QuickRecord.parse("スクワット20回"), .init(title: "スクワット", number: 20, unit: "回"))
        XCTAssertEqual(QuickRecord.parse("800円のランチ"), .init(title: "ランチ", number: 800, unit: "円"))
        XCTAssertEqual(QuickRecord.parse("散歩した"), .init(title: "散歩した", number: nil))
        XCTAssertEqual(QuickRecord.parse("  "), .init(title: nil, number: nil))
    }

    func testKakeibo() throws {
        let entity = try XCTUnwrap(ExamplesFixture.load("kakeibo").entity("expense"))
        XCTAssertEqual(QuickRecord.titleField(of: entity)?.id, "memo")
        XCTAssertEqual(QuickRecord.numberField(of: entity)?.id, "amount")

        let draft = QuickRecord.build(entity: entity, text: "ランチ 800円")
        XCTAssertEqual(draft.data["memo"], "ランチ")
        XCTAssertEqual(draft.data["amount"], 800)
        XCTAssertEqual(draft.data["category"], "食費")           // enum の既定値
        XCTAssertEqual(draft.data["date"]?.stringValue?.count, 10) // date の now → YYYY-MM-DD
        XCTAssertFalse(draft.data.keys.contains("receipt"))  // 未入力の photo は送らない
        XCTAssertEqual(draft.missing, [])
        XCTAssertEqual(draft.summary, "ランチ ¥800")
    }

    func testEnumMatchedFromText() throws {
        let entity = try XCTUnwrap(ExamplesFixture.load("kakeibo").entity("expense"))
        let option = try XCTUnwrap(entity.field("category")?.options?.last)
        let draft = QuickRecord.build(entity: entity, text: "\(option) 300円")
        XCTAssertEqual(draft.data["category"]?.stringValue, option)
        XCTAssertEqual(draft.data["amount"], 300)
    }

    func testExplicitNumberWinsAndMissingRequired() throws {
        let entity = try XCTUnwrap(ExamplesFixture.load("kakeibo").entity("expense"))
        XCTAssertEqual(QuickRecord.build(entity: entity, text: "ランチ 800円", number: 650).data["amount"], 650)
        // 金額(required)が無ければ missing に名前が出る(Intent はここで聞き返す)
        let noAmount = QuickRecord.build(entity: entity, text: "ランチ")
        XCTAssertEqual(noAmount.missing, [entity.field("amount")!.name])
    }

    func testTrainingEnumTitle() throws {
        let entity = try XCTUnwrap(ExamplesFixture.load("training").entity("workout"))
        XCTAssertEqual(QuickRecord.titleField(of: entity)?.id, "exercise")
        XCTAssertEqual(QuickRecord.numberField(of: entity)?.id, "reps")
        let option = try XCTUnwrap(entity.field("exercise")?.options?.first)
        let draft = QuickRecord.build(entity: entity, text: "\(option) 20回")
        XCTAssertEqual(draft.data["exercise"]?.stringValue, option)
        XCTAssertEqual(draft.data["reps"], 20)
        XCTAssertEqual(draft.missing, [])
    }

    func testRecordableEntitiesPreferFormScreens() throws {
        let dsl = try ExamplesFixture.load("kakeibo")
        XCTAssertEqual(QuickRecord.recordableEntities(in: dsl).first?.id, "expense")
        XCTAssertTrue(QuickRecord.recordableEntities(in: try ExamplesFixture.load("initial")).isEmpty)
    }

    /// 単位で数値項目を選ぶ(生成アプリでよくある 重量 kg / 回数 回 の両方を持つ entity)
    func testNumberFieldByUnit() throws {
        let json = #"{"id":"workout","name":"筋トレ","titleField":"exercise","fields":[{"id":"exercise","name":"種目","type":"string","required":true},{"id":"weight","name":"重量","type":"number","unit":"kg"},{"id":"reps","name":"回数","type":"integer","unit":"回"},{"id":"sets","name":"セット数","type":"integer","defaultValue":3}]}"#
        let entity = try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
        XCTAssertEqual(QuickRecord.build(entity: entity, text: "スクワット 20回").data["reps"], 20)
        XCTAssertEqual(QuickRecord.build(entity: entity, text: "ベンチ 60kg").data["weight"], 60)
        XCTAssertEqual(QuickRecord.build(entity: entity, text: "ベンチ 60").data["weight"], 60) // 単位なしは先頭の数値
        XCTAssertEqual(QuickRecord.build(entity: entity, text: "スクワット 20回").data["sets"], 3)
    }

    // MARK: - ref(カテゴリ台帳など、ユーザーが編集する参照先)

    /// 支出(memo / amount / category=ref→categories / payment=enum)と、カテゴリ台帳。
    private func refDSL(categoryDefault: String? = nil) throws -> AppDSL {
        let def = categoryDefault.map { #","defaultValue":"\#($0)""# } ?? ""
        let json = """
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","entities":[
         {"id":"expense","name":"支出","titleField":"memo","fields":[
          {"id":"memo","name":"メモ","type":"string"},
          {"id":"amount","name":"金額","type":"currency","required":true},
          {"id":"category","name":"カテゴリ","type":"ref","refEntity":"categories"\(def)},
          {"id":"payment","name":"支払い","type":"enum","options":["現金","カード"]}]},
         {"id":"categories","name":"カテゴリ","titleField":"name","fields":[{"id":"name","name":"名前","type":"string"}]}],
         "screens":[{"id":"home","title":"t","components":[]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private let categories: [Record] = [
        Record(id: "c1", data: ["name": "食費"]),
        Record(id: "c2", data: ["name": "食費(外食)"]),
        Record(id: "c3", data: ["name": "交通費"]),
    ]

    private func refBuild(_ text: String, categoryDefault: String? = nil, records: [Record]? = nil) throws -> QuickRecord.Draft {
        let dsl = try refDSL(categoryDefault: categoryDefault)
        let entity = try XCTUnwrap(dsl.entity("expense"))
        let choices = QuickRecord.refChoices(for: entity, in: dsl, records: ["categories": records ?? categories])
        return QuickRecord.build(entity: entity, text: text, refChoices: choices)
    }

    /// 1 文字の見出しは語の一部に当たりやすい(「電車」に「車」・「1200円」に「1」)ので、空白で区切った語と一致する時だけ当てる。
    func testRefShortTitleNeedsWholeWord() throws {
        let short = categories + [Record(id: "c4", data: ["name": "車"]), Record(id: "c5", data: ["name": "1"])]
        XCTAssertNil(try refBuild("電車 300円", records: short).data["category"])
        XCTAssertNil(try refBuild("ランチ 1200円", records: short).data["category"])
        XCTAssertEqual(try refBuild("車 ガソリン 3000円", records: short).data["category"], "c4")
        XCTAssertEqual(try refBuild("食費 電車 300円", records: short).data["category"], "c1") // 2 文字以上は従来どおり部分一致
    }

    /// Siri の返事で、どのカテゴリに入れたか分かるようにする(メモに同じ語が残るので要約だけでは当たったか分からない)。
    func testRefMatchIsReported() throws {
        XCTAssertEqual(try refBuild("食費 コンビニ 480円").refs, ["カテゴリ: 食費"])
        XCTAssertEqual(try refBuild("コンビニ 480円").refs, [])
        XCTAssertEqual(try refBuild("コンビニ 480円", categoryDefault: "c3").refs, []) // 既定値は当てたものではない
    }

    func testRefMatchedByRecordTitle() throws {
        let draft = try refBuild("食費 コンビニ 480円")
        XCTAssertEqual(draft.data["category"], "c1") // 見出しでなく id を入れる
        XCTAssertEqual(draft.data["amount"], 480)
        XCTAssertEqual(draft.data["memo"], "食費 コンビニ") // enum と同じく残りの文言はそのまま
        XCTAssertEqual(draft.missing, [])
    }

    func testRefLongestTitleWins() throws {
        XCTAssertEqual(try refBuild("食費(外食) ラーメン 900円").data["category"], "c2")
    }

    func testRefNoMatchLeavesEmptyOrDefault() throws {
        XCTAssertNil(try refBuild("コンビニ 480円").data["category"])
        XCTAssertEqual(try refBuild("コンビニ 480円", categoryDefault: "c3").data["category"], "c3")
        // 候補を渡さない(従来の呼び出し)も空のまま
        let entity = try XCTUnwrap(try refDSL().entity("expense"))
        XCTAssertNil(QuickRecord.build(entity: entity, text: "食費 480円").data["category"])
    }

    func testEnumAndRefBothMatched() throws {
        let draft = try refBuild("交通費 カード 300円")
        XCTAssertEqual(draft.data["category"], "c3")
        XCTAssertEqual(draft.data["payment"], "カード")
        XCTAssertEqual(draft.data["amount"], 300)
    }
}
