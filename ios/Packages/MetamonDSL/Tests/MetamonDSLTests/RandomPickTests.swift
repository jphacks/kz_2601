import XCTest
@testable import MetamonDSL

/// `randomPicker`(#263): 例のデコード・選び方(直前と同じものを続けて出さない・空・1 件)・演出のコマ。
final class RandomPickTests: XCTestCase {
    /// 再現できる乱数源(SplitMix64)。
    private struct Seeded: RandomNumberGenerator {
        var state: UInt64
        mutating func next() -> UInt64 {
            state &+= 0x9E37_79B9_7F4A_7C15
            var z = state
            z = (z ^ (z >> 30)) &* 0xBF58_476D_1CE4_E5B9
            z = (z ^ (z >> 27)) &* 0x94D0_49BB_1331_11EB
            return z ^ (z >> 31)
        }
    }

    private func records(_ n: Int) -> [Record] {
        (1...n).map { Record(id: "r\($0)", data: ["name": .string("料理\($0)")]) }
    }

    func testExampleDecodes() throws {
        let dsl = try ExamplesFixture.load("dinner")
        let picker = try XCTUnwrap(dsl.screens.flatMap(\.allComponents).first { $0.type == .randomPicker })
        XCTAssertEqual(picker.entity, "dish")
        XCTAssertEqual(picker.titleField, "name")
        XCTAssertEqual(picker.subtitleField, "genre")
        XCTAssertEqual(picker.label, "今日の晩ごはんを決める")
        XCTAssertEqual(picker.onItemTap?.type, .navigate)
        XCTAssertTrue(DSLExamples.names.contains("dinner"))
    }

    func testEmptyAndSingle() {
        var g = Seeded(state: 1)
        XCTAssertNil(RandomPick.pick(from: [], excluding: nil, using: &g))
        let one = records(1)
        // 1 件しか無ければ直前と同じでも選ぶ(押しても何も出ないよりよい)
        XCTAssertEqual(RandomPick.pick(from: one, excluding: "r1", using: &g)?.id, "r1")
        XCTAssertEqual(RandomPick.reel(from: one, ending: one[0], frames: 9, using: &g), [])
    }

    func testNeverRepeatsPreviousAndCoversAll() {
        var g = Seeded(state: 42)
        let rs = records(4)
        var previous: String?
        var seen = Set<String>()
        for _ in 0..<200 {
            let r = RandomPick.pick(from: rs, excluding: previous, using: &g)
            XCTAssertNotNil(r)
            XCTAssertNotEqual(r?.id, previous, "直前と同じ 1 件が続いた")
            previous = r?.id
            seen.insert(r?.id ?? "")
        }
        XCTAssertEqual(seen, ["r1", "r2", "r3", "r4"], "選ばれない候補がある")
    }

    func testTwoCandidatesAlternate() {
        var g = Seeded(state: 7)
        let rs = records(2)
        var previous = "r1"
        for _ in 0..<10 {
            let r = RandomPick.pick(from: rs, excluding: previous, using: &g)
            XCTAssertEqual(r?.id, previous == "r1" ? "r2" : "r1")
            previous = r?.id ?? ""
        }
    }

    func testReelEndsBeforeWinnerWithoutStalling() {
        var g = Seeded(state: 3)
        let rs = records(3)
        for _ in 0..<50 {
            let winner = rs[Int.random(in: 0..<3, using: &g)]
            let frames = RandomPick.reel(from: rs, ending: winner, frames: 9, using: &g)
            XCTAssertEqual(frames.count, 9)
            XCTAssertNotEqual(frames.last?.id, winner.id, "最後のコマが当選と同じだと止まって見える")
            for (a, b) in zip(frames, frames.dropFirst()) {
                XCTAssertNotEqual(a.id, b.id, "同じコマが続いた")
            }
        }
    }
}
