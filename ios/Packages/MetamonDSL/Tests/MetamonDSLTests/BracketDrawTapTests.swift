import XCTest
@testable import MetamonDSL

/// トーナメントの次の試合が決まっている時の押し方(`BracketDraw.nextDecided`・`tap` の `TapResult`)。
final class BracketDrawTapTests: XCTestCase {
    private func p(_ id: String, wins: Int?, at day: Int) -> Record {
        var data: [String: JSONValue] = ["name": .string(id)]
        if let wins { data["wins"] = .number(Double(wins)) }
        return Record(id: id, data: data, createdAt: String(format: "2026-11-%02dT00:00:00Z", day))
    }

    /// 出場者を登録順に並べた表(シードなし)。4 人: 1 回戦は a-d・b-c。8 人: a-h・d-e・b-g・c-f。
    private func draw(_ ids: [String], _ wins: [String: Int] = [:], pending: [String: Int] = [:]) -> BracketDraw {
        let rs = ids.enumerated().map { i, id in p(id, wins: wins[id], at: i + 1) }
        return BracketDraw(records: rs, winsField: "wins", seedField: nil, pending: pending)
    }

    private let four = ["a", "b", "c", "d"]
    private let eight = ["a", "b", "c", "d", "e", "f", "g", "h"]

    // MARK: - nextDecided

    func testNextDecidedTable() {
        let cases: [(name: String, wins: [String: Int], round: Int, index: Int, want: Bool)] = [
            ("結果なし", [:], 1, 0, false),
            ("1 回戦だけ決まった(先の相手が未定)", ["a": 1], 1, 0, false),
            ("2 回戦の 2 人がそろったが未定", ["a": 1, "b": 1], 1, 0, false),
            ("2 回戦の 2 人がそろったが未定(下の山)", ["a": 1, "b": 1], 1, 1, false),
            ("決勝が決まった(上の山)", ["a": 2, "b": 1], 1, 0, true),
            ("決勝が決まった(下の山・負けた側の山でも)", ["a": 2, "b": 1], 1, 1, true),
            ("決勝は先が無い", ["a": 2, "b": 1], 2, 0, false),
        ]
        for c in cases {
            let d = draw(four, c.wins)
            XCTAssertEqual(d.nextDecided(d.rounds[c.round - 1][c.index]), c.want, c.name)
        }
    }

    /// 先の試合は index / 2 の試合。8 人の下の山だけ決まっていれば、上の山の試合は動かせる。
    func testNextDecidedFollowsTheHalfIndex() {
        let d = draw(eight, ["b": 2, "c": 1]) // b-g は b・c-f は c・2 回戦 b-c は b
        XCTAssertEqual(d.rounds[1][1].winner, .a)
        XCTAssertEqual(d.rounds[0].map { d.nextDecided($0) }, [false, false, true, true])
        XCTAssertEqual(d.rounds[1].map { d.nextDecided($0) }, [false, false], "決勝の相手が未定")
        XCTAssertFalse(d.nextDecided(d.rounds[2][0]))
    }

    func testNextDecidedUsesPendingWins() {
        let saved = ["a": 1, "b": 1]
        XCTAssertFalse(draw(four, saved).nextDecided(draw(four, saved).rounds[0][0]))
        let d = draw(four, saved, pending: ["a": 2])
        XCTAssertTrue(d.nextDecided(d.rounds[0][0]), "保存待ちで決勝が決まったら先は決まり")
        XCTAssertEqual(d.tap(d.rounds[0][0], side: .b, winsField: "wins", pending: ["a": 2]), .locked)
    }

    // MARK: - tap

    /// 先が決まっていれば勝った側・負けた側どちらを押しても locked。決まっていなければ update。
    func testTapLockedOnlyWhenNextDecided() {
        let cases: [(name: String, wins: [String: Int], side: BracketDraw.Side, want: BracketDraw.TapResult)] = [
            ("先が未定・勝者を押す=取り消し", ["a": 1], .a, .update(["a": 0])),
            ("先が未定・負けた側を押す=入れ替え", ["a": 1], .b, .update(["d": 1, "a": 0])),
            ("先が未定(2 人そろって未定)・勝者", ["a": 1, "b": 1], .a, .update(["a": 0])),
            ("先が決まった・勝者", ["a": 2, "b": 1], .a, .locked),
            ("先が決まった・負けた側", ["a": 2, "b": 1], .b, .locked),
            ("決勝で負けた人の山でも先が決まれば", ["a": 1, "b": 2], .a, .locked),
        ]
        for c in cases {
            let d = draw(four, c.wins)
            XCTAssertEqual(d.tap(d.rounds[0][0], side: c.side, winsField: "wins"), c.want, c.name)
        }
    }

    /// 表の `tap` は表の `nextDecided` を渡した静的な `tap` と同じ答え(全試合・両側)。
    func testInstanceTapMatchesStaticTapWithNextDecided() {
        let tables: [[String: Int]] = [[:], ["a": 1], ["a": 2, "d": 1, "b": 1], ["b": 2, "c": 1], ["a": 3, "b": 2, "c": 1, "d": 1]]
        for wins in tables {
            let d = draw(eight, wins)
            for m in d.rounds.joined() {
                for side in [BracketDraw.Side.a, .b] {
                    XCTAssertEqual(d.tap(m, side: side, winsField: "wins"),
                                   BracketDraw.tap(m, side: side, winsField: "wins", nextDecided: d.nextDecided(m)),
                                   "wins=\(wins) r\(m.round)-\(m.index) \(side)")
                }
            }
        }
    }

    /// 静的な `tap`: 勝者のいない試合は nextDecided でも止めない(先に結果が付くことはない前提でも、押せば決める)。
    func testStaticTapIgnoresNextDecidedWithoutWinner() {
        let d = draw(four)
        let m = d.rounds[0][0]
        XCTAssertNil(m.winner)
        XCTAssertEqual(BracketDraw.tap(m, side: .a, winsField: "wins", nextDecided: true), .update(["a": 1]))
        let won = draw(four, ["a": 1]).rounds[0][0]
        XCTAssertEqual(BracketDraw.tap(won, side: .a, winsField: "wins", nextDecided: true), .locked)
        XCTAssertEqual(BracketDraw.tap(won, side: .a, winsField: "wins", nextDecided: false), .update(["a": 0]))
    }

    /// 選べない試合(不戦勝・相手が未定)は、先が決まっていても notPlayable(locked より先)。
    func testNotPlayableComesBeforeLocked() {
        let three = draw(["a", "b", "c"], ["a": 2, "b": 1]) // a は不戦勝・b-c は b・決勝 a-b は a
        let bye = three.rounds[0][0]
        XCTAssertTrue(bye.bye)
        XCTAssertTrue(three.nextDecided(bye))
        XCTAssertEqual(three.tap(bye, side: .a, winsField: "wins"), .notPlayable)
        XCTAssertEqual(three.tap(three.rounds[0][1], side: .b, winsField: "wins"), .locked)
    }

    // MARK: - 保存待ちの勝ち数

    func testPendingWinsAreAddedToTheDecision() {
        // 保存値は d の勝ち。保存待ちで d を 0 に戻した後は、d を押すと勝ちにする
        let pending = ["d": 0]
        let d = draw(four, ["d": 1], pending: pending)
        XCTAssertNil(d.rounds[0][0].winner)
        XCTAssertEqual(d.tap(d.rounds[0][0], side: .b, winsField: "wins", pending: pending), .update(["d": 1]))
        // 保存待ちで相手(d)が勝っていれば、a を押すと d を 1 つ下げる
        XCTAssertEqual(draw(four).tap(draw(four).rounds[0][0], side: .a, winsField: "wins", pending: ["d": 1]),
                       .update(["a": 1, "d": 0]))
        // 2 回戦: 相手の保存待ちの勝ち数が 2 なら r-1 = 1 に戻す
        let r2 = ["a": 1, "b": 1]
        let e = draw(four, r2, pending: ["b": 2])
        XCTAssertEqual(e.rounds[1][0].winner, .b)
        XCTAssertEqual(e.tap(e.rounds[1][0], side: .a, winsField: "wins", pending: ["b": 2]), .update(["a": 2, "b": 1]))
        XCTAssertEqual(e.tap(e.rounds[1][0], side: .b, winsField: "wins", pending: ["b": 2]), .update(["b": 1]))
    }
}
