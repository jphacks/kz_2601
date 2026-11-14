import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// `bracket`(トーナメント表): どのビルドにも登録・勝ち数の field の条件・回戦の名前。
final class BracketLeafTests: XCTestCase {
    private func tournament() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/tournament.json"))
    }

    @MainActor func testRegisteredInEveryBuild() throws {
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "bracket"))
        XCTAssertFalse(ClientCapabilities.reviewExcludedComponents.contains(.bracket), "OS の機能も共有も使わないので審査用ビルドにも入れる")
        XCTAssertTrue(ClientCapabilities.allComponents.contains("bracket"))
        XCTAssertTrue(ClientCapabilities.supportedFeatures.contains("component.bracket"))
        let c = try XCTUnwrap(tournament().screens.flatMap(\.allComponents).first { $0.type == .bracket })
        XCTAssertTrue(c.rendersOwnSurface)
    }

    func testWinsFieldMustBeStoredInteger() throws {
        let dsl = try tournament()
        var c = try XCTUnwrap(dsl.screens.flatMap(\.allComponents).first { $0.type == .bracket })
        XCTAssertEqual(BracketLeaf.winsField(c, in: dsl.entity("entrant")), "wins")
        c.winsField = "name"
        XCTAssertNil(BracketLeaf.winsField(c, in: dsl.entity("entrant")), "string の field には書かない(表示だけ)")
        c.winsField = "nope"
        XCTAssertNil(BracketLeaf.winsField(c, in: dsl.entity("entrant")))
    }

    func testStartedFieldMustBeStoredDatetime() throws {
        let dsl = try tournament()
        var c = try XCTUnwrap(dsl.screens.flatMap(\.allComponents).first { $0.type == .bracket })
        XCTAssertEqual(BracketLeaf.startedField(c, in: dsl.entity("entrant")), "started")
        c.startedField = "seed"
        XCTAssertNil(BracketLeaf.startedField(c, in: dsl.entity("entrant")), "datetime でなければ時刻の規則")
        c.startedField = nil
        XCTAssertNil(BracketLeaf.startedField(c, in: dsl.entity("entrant")))
    }

    /// 開始時刻は勝ち数より先に書き(勝ちが残れば必ず開始時刻がある)、消す時は勝ち数を戻してから消す。
    func testWritePlanOrdersStartAroundWins() {
        let now = JSONValue.string("2026-11-09T00:00:00Z")
        let set = BracketLeaf.writePlan(wins: ["a": 1], starts: ["a": now, "b": now], winsField: "wins", startedField: "started")
        XCTAssertEqual(set.map(\.0), ["b", "a"])
        XCTAssertEqual(set[1].1, ["wins": .number(1), "started": now])
        let clear = BracketLeaf.writePlan(wins: ["a": 0], starts: ["a": .null, "b": .null], winsField: "wins", startedField: "started")
        XCTAssertEqual(clear.map(\.0), ["a", "b"])
        XCTAssertEqual(BracketLeaf.writePlan(wins: ["a": 1], starts: [:], winsField: "wins", startedField: nil).map(\.1), [["wins": .number(1)]])
    }

    /// 勝者を入れ替える時は元の勝者を下げる方を先に保存する(途中で失敗しても勝者が 2 人にならない)。
    func testWriteOrderLowersFirst() {
        XCTAssertEqual(BracketLeaf.writeOrder(["me": 2, "other": 1]).map(\.0), ["other", "me"])
        XCTAssertEqual(BracketLeaf.writeOrder(["b": 1, "a": 1]).map(\.0), ["a", "b"])
    }

    func testMatchLabelNamesRoundAndMatch() {
        let rs = ["a", "b", "c", "d"].map { Record(id: $0, data: [:], createdAt: "2026-11-0\($0 == "a" ? 1 : 2)") }
        let m = BracketDraw(records: rs, winsField: "wins", seedField: nil).rounds[0][1]
        XCTAssertEqual(BracketLeaf.matchLabel(m, roundTitle: L("準決勝")), L("\(L("準決勝")) 第 \(2) 試合"))
    }

    func testRoundTitles() {
        XCTAssertEqual((1...4).map { BracketLeaf.roundTitle($0, of: 4) }, [L("\(1) 回戦"), L("準々決勝"), L("準決勝"), L("決勝")])
        XCTAssertEqual(BracketLeaf.roundTitle(1, of: 1), L("決勝"))
    }
}
