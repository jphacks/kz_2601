import XCTest
@testable import MetamonDSL

/// Siri / ショートカット「記録を追加」(#80)のパラメータ解決。
final class QuickRecordTests: XCTestCase {
    func testParse() {
        XCTAssertEqual(QuickRecord.parse("ランチ 800円"), .init(title: "ランチ", number: 800))
        XCTAssertEqual(QuickRecord.parse("ランチ８００円"), .init(title: "ランチ", number: 800))
        XCTAssertEqual(QuickRecord.parse("ランチを1,200円"), .init(title: "ランチ", number: 1200))
        XCTAssertEqual(QuickRecord.parse("￥980 コンビニ"), .init(title: "コンビニ", number: 980))
        XCTAssertEqual(QuickRecord.parse("¥980 コンビニ"), .init(title: "コンビニ", number: 980))
        XCTAssertEqual(QuickRecord.parse("体重 62.5kg"), .init(title: "体重", number: 62.5))
        XCTAssertEqual(QuickRecord.parse("スクワット20回"), .init(title: "スクワット", number: 20))
        XCTAssertEqual(QuickRecord.parse("800円のランチ"), .init(title: "ランチ", number: 800))
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
}
