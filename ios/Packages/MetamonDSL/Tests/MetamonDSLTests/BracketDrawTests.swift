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

    /// 標準のシード配置: 1 回戦の 2 人のシード番号の和は枠の数 + 1。不戦勝は上位シードから(1..size-n)。
    func testSeedPositionsAndByesForManySizes() {
        for n in [2, 3, 5, 7, 8, 9, 16, 33, 64] {
            let rs = (1...n).map { p(String(format: "p%02d", $0), at: 1) }
            let d = BracketDraw(records: rs, winsField: "wins", seedField: nil)
            var size = 2
            while size < n { size *= 2 }
            let seed = { (r: Record?) -> Int? in r.map { Int($0.id.dropFirst())! } }
            XCTAssertEqual(d.rounds[0].count, size / 2, "n=\(n)")
            XCTAssertEqual(d.rounds.count, size.trailingZeroBitCount, "n=\(n)")
            var byes: [Int] = []
            for m in d.rounds[0] {
                let a = seed(m.a), b = seed(m.b)
                XCTAssertNotNil(a, "n=\(n): the top slot always has an entrant")
                if let a, let b { XCTAssertEqual(a + b, size + 1, "n=\(n)") } else if let a { byes.append(a) }
            }
            XCTAssertEqual(byes.sorted(), size > n ? Array(1...(size - n)) : [], "n=\(n)")
            // 上位 2 シードは決勝まで当たらない
            guard size >= 4 else { continue }
            XCTAssertTrue(d.rounds[0].prefix(size / 4).contains { seed($0.a) == 1 }, "n=\(n)")
            XCTAssertTrue(d.rounds[0].suffix(size / 4).contains { seed($0.a) == 2 }, "n=\(n)")
        }
    }

    /// 両方が勝ったことになっている(よそで編集)時は値の大きい方、同じなら上の枠。
    func testTiesGoToTheLargerWinsThenTheTopSlot() {
        XCTAssertEqual(four(["a": 1, "d": 1]).rounds[0][0].winner, .a)
        XCTAssertEqual(four(["a": 1, "d": 2]).rounds[0][0].winner, .b)
    }

    // MARK: - タップ

    private func four(_ wins: [String: Int] = [:]) -> BracketDraw {
        let rs = ["a", "b", "c", "d"].enumerated().map { i, id in p(id, wins: wins[id], at: i + 1) }
        return BracketDraw(records: rs, winsField: "wins", seedField: nil)
    }

    func testTapSetsWinnerAndSwapsBack() {
        let m = four().rounds[0][0] // a-d
        XCTAssertEqual(four().tap(m, side: .b, winsField: "wins"), .update(["d": 1]))
        let dd = four(["d": 1]), decided = dd.rounds[0][0]
        XCTAssertEqual(decided.winner, .b)
        // 相手に変える: a を 1・d を 0 に
        XCTAssertEqual(dd.tap(decided, side: .a, winsField: "wins"), .update(["a": 1, "d": 0]))
        // 勝者をもう一度: 取り消し
        XCTAssertEqual(dd.tap(decided, side: .b, winsField: "wins"), .update(["d": 0]))
    }

    func testTapUsesPendingWins() {
        let m = four().rounds[0][0]
        XCTAssertEqual(four().tap(m, side: .a, winsField: "wins", pending: ["d": 1]), .update(["a": 1, "d": 0]))
        let d = BracketDraw(records: four().entrants, winsField: "wins", seedField: nil, pending: ["d": 1])
        XCTAssertEqual(d.rounds[0][0].winner, .b)
    }

    /// 先の回戦に進んだ人の試合は変えられない。不戦勝・相手が未定の試合は選べない。
    func testLockedAndNotPlayable() {
        let d = four(["a": 2, "b": 1])
        XCTAssertEqual(d.tap(d.rounds[0][0], side: .b, winsField: "wins"), .locked)
        XCTAssertEqual(d.tap(d.rounds[0][0], side: .a, winsField: "wins"), .locked)
        XCTAssertEqual(d.tap(d.rounds[1][0], side: .a, winsField: "wins"), .update(["a": 1]))

        let undecided = four().rounds[1][0]
        XCTAssertEqual(four().tap(undecided, side: .a, winsField: "wins"), .notPlayable)
        let rs = ["a", "b", "c"].enumerated().map { i, id in p(id, at: i + 1) }
        let three = BracketDraw(records: rs, winsField: "wins", seedField: nil), bye = three.rounds[0][0]
        XCTAssertTrue(bye.bye)
        XCTAssertEqual(three.tap(bye, side: .a, winsField: "wins"), .notPlayable)
    }

    /// 決勝の結果が付いたら、負けた側でも前の回戦は変えられない(変えると決勝の結果が別の人に移る)。
    func testDecidedNextMatchLocksEvenWhenTheFinalistLost() {
        let d = four(["a": 1, "b": 2]) // a-d は a、b-c は b、決勝 a-b は b
        XCTAssertEqual(d.rounds[1][0].winner, .b)
        XCTAssertEqual(d.tap(d.rounds[0][0], side: .b, winsField: "wins"), .locked)
        XCTAssertEqual(d.tap(d.rounds[0][0], side: .a, winsField: "wins"), .locked)
        // 決勝を取り消せば変えられる
        XCTAssertEqual(d.tap(d.rounds[1][0], side: .b, winsField: "wins"), .update(["b": 1]))
    }

    /// よそで大きすぎる wins を入れられても、先の試合が決まっていなければ取り消せる(決勝は先が無い)。
    func testOverstatedWinsCanStillBeUndone() {
        let d = four(["a": 9, "b": 1])
        XCTAssertEqual(d.champion?.id, "a")
        XCTAssertEqual(d.tap(d.rounds[1][0], side: .a, winsField: "wins"), .update(["a": 1]))
        let e = four(["a": 3]) // 2 回戦の相手(b-c)が未定
        XCTAssertEqual(e.tap(e.rounds[0][0], side: .a, winsField: "wins"), .update(["a": 0]))
        XCTAssertEqual(e.tap(e.rounds[0][0], side: .b, winsField: "wins"), .update(["d": 1, "a": 0]))
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
