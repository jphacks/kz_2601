import XCTest
@testable import MetamonDSL

/// `bracket`(トーナメント表): 組み合わせ(シード順・不戦勝)・勝ち上がり・勝者のタップ。
final class BracketDrawTests: XCTestCase {
    private func p(_ id: String, wins: Int? = nil, seed: Double? = nil, at day: Int) -> Record {
        var data: [String: JSONValue] = ["name": .string(id)]
        if let wins { data["wins"] = .number(Double(wins)) }
        if let seed { data["seed"] = .number(seed) }
        return Record(id: id, data: data, createdAt: String(format: "2026-11-%02dT00:00:00Z", day))
    }

    private func ids(_ m: BracketDraw.Match) -> [String?] { [m.a?.id, m.b?.id] }

    func testSlotSeedsKeepTopSeedsApart() {
        XCTAssertEqual(BracketDraw.slotSeeds(2), [1, 2])
        XCTAssertEqual(BracketDraw.slotSeeds(4), [1, 4, 2, 3])
        XCTAssertEqual(BracketDraw.slotSeeds(8), [1, 8, 4, 5, 2, 7, 3, 6])
    }

    func testFewerThanTwoEntrantsHasNoRounds() {
        XCTAssertTrue(BracketDraw(records: [], winsField: "wins", seedField: nil).rounds.isEmpty)
        let one = BracketDraw(records: [p("a", at: 1)], winsField: "wins", seedField: nil)
        XCTAssertTrue(one.rounds.isEmpty)
        XCTAssertEqual(one.entrants.map(\.id), ["a"])
        XCTAssertNil(one.champion)
    }

    /// 5 人 → 8 枠・3 回戦。空き枠 3 つは上位シード(登録順の 1〜3 番目)の不戦勝。
    func testByesGoToTopSeedsInCreationOrder() {
        let rs = [p("e", at: 5), p("a", at: 1), p("c", at: 3), p("b", at: 2), p("d", at: 4)]
        let d = BracketDraw(records: rs, winsField: "wins", seedField: nil)
        XCTAssertEqual(d.rounds.count, 3)
        XCTAssertEqual(d.rounds[0].map(ids), [["a", nil], ["d", "e"], ["b", nil], ["c", nil]])
        XCTAssertEqual(d.rounds[0].map(\.bye), [true, false, true, true])
        XCTAssertEqual(d.rounds[0].map(\.winner), [.a, nil, .a, .a])
        // 2 回戦: a と(d-e の勝者 = 未定)、b と c
        XCTAssertEqual(d.rounds[1].map(ids), [["a", nil], ["b", "c"]])
        XCTAssertFalse(d.rounds[1][0].isPlayable)
        XCTAssertTrue(d.rounds[1][1].isPlayable)
        XCTAssertNil(d.champion)
    }

    /// seedField の昇順(値の無い人は後ろ・その中は登録順)。
    func testSeedFieldOrdersEntrants() {
        let rs = [p("a", at: 1), p("b", seed: 2, at: 2), p("c", seed: 1, at: 3), p("d", at: 4)]
        XCTAssertEqual(BracketDraw.seeded(rs, seedField: "seed").map(\.id), ["c", "b", "a", "d"])
        let d = BracketDraw(records: rs, winsField: "wins", seedField: "seed")
        XCTAssertEqual(d.rounds[0].map(ids), [["c", "d"], ["b", "a"]])
    }

    /// wins だけで勝ち上がりが決まり、決勝の勝者が優勝。
    func testWinsAdvanceToChampion() {
        let rs = [p("a", wins: 2, at: 1), p("b", wins: 0, at: 2), p("c", wins: 1, at: 3), p("d", at: 4)]
        let d = BracketDraw(records: rs, winsField: "wins", seedField: nil)
        // 1 回戦 a-d / b-c
        XCTAssertEqual(d.rounds[0].map(ids), [["a", "d"], ["b", "c"]])
        XCTAssertEqual(d.rounds[0].map(\.winner), [.a, .b])
        XCTAssertEqual(d.rounds[1].map(ids), [["a", "c"]])
        XCTAssertEqual(d.champion?.id, "a")
        XCTAssertEqual(d.rounds.count, 2)
    }

    func testOnlySixtyFourEntrantsAreDrawn() {
        let rs = (1...70).map { p("p\($0)", at: 1) }
        let d = BracketDraw(records: rs, winsField: "wins", seedField: nil)
        XCTAssertEqual(d.entrants.count, BracketDraw.maxEntrants)
        XCTAssertEqual(d.rounds.count, 6)
    }

    // MARK: - タップ

    private func four(_ wins: [String: Int] = [:]) -> BracketDraw {
        let rs = ["a", "b", "c", "d"].enumerated().map { i, id in p(id, wins: wins[id], at: i + 1) }
        return BracketDraw(records: rs, winsField: "wins", seedField: nil)
    }

    func testTapSetsWinnerAndSwapsBack() {
        let m = four().rounds[0][0] // a-d
        XCTAssertEqual(BracketDraw.tap(m, side: .b, winsField: "wins"), .update(["d": 1]))
        let decided = four(["d": 1]).rounds[0][0]
        XCTAssertEqual(decided.winner, .b)
        // 相手に変える: a を 1・d を 0 に
        XCTAssertEqual(BracketDraw.tap(decided, side: .a, winsField: "wins"), .update(["a": 1, "d": 0]))
        // 勝者をもう一度: 取り消し
        XCTAssertEqual(BracketDraw.tap(decided, side: .b, winsField: "wins"), .update(["d": 0]))
    }

    func testTapUsesPendingWins() {
        let m = four().rounds[0][0]
        XCTAssertEqual(BracketDraw.tap(m, side: .a, winsField: "wins", pending: ["d": 1]), .update(["a": 1, "d": 0]))
        let d = BracketDraw(records: four().entrants, winsField: "wins", seedField: nil, pending: ["d": 1])
        XCTAssertEqual(d.rounds[0][0].winner, .b)
    }

    /// 先の回戦に進んだ人の試合は変えられない。不戦勝・相手が未定の試合は選べない。
    func testLockedAndNotPlayable() {
        let d = four(["a": 2, "b": 1])
        XCTAssertEqual(BracketDraw.tap(d.rounds[0][0], side: .b, winsField: "wins"), .locked)
        XCTAssertEqual(BracketDraw.tap(d.rounds[0][0], side: .a, winsField: "wins"), .locked)
        XCTAssertEqual(BracketDraw.tap(d.rounds[1][0], side: .a, winsField: "wins"), .update(["a": 1]))

        let undecided = four().rounds[1][0]
        XCTAssertEqual(BracketDraw.tap(undecided, side: .a, winsField: "wins"), .notPlayable)
        let rs = ["a", "b", "c"].enumerated().map { i, id in p(id, at: i + 1) }
        let bye = BracketDraw(records: rs, winsField: "wins", seedField: nil).rounds[0][0]
        XCTAssertTrue(bye.bye)
        XCTAssertEqual(BracketDraw.tap(bye, side: .a, winsField: "wins"), .notPlayable)
    }

    func testTournamentExampleIsBundled() throws {
        XCTAssertTrue(DSLExamples.names.contains("tournament"))
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        let dsl = try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/tournament.json"))
        let c = try XCTUnwrap(dsl.screens.flatMap(\.allComponents).first { $0.type == .bracket })
        XCTAssertEqual(c.winsField, "wins")
        XCTAssertEqual(c.seedField, "seed")
        XCTAssertEqual(c.titleField, "name")
    }
}
