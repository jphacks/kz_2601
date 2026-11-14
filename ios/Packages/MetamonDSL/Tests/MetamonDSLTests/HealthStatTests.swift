import XCTest
@testable import MetamonDSL

/// `healthStat` / `Field.health`(#167)のデコード。
final class HealthStatTests: XCTestCase {
    func testExampleDecodesHealthStatAndHealthField() throws {
        let dsl = try ExamplesFixture.load("health")
        let stats = dsl.screens.flatMap(\.components).filter { $0.type == .healthStat }
        XCTAssertEqual(stats.map(\.metric), ["steps", "weight", "steps"])
        XCTAssertEqual(stats.map(\.period), [nil, nil, "week"])
        XCTAssertEqual(stats.first?.target, 8000)
        let weight = try XCTUnwrap(dsl.entity("body")?.field("weight"))
        XCTAssertEqual(weight.health, "weight")
        XCTAssertNil(dsl.entity("body")?.field("date")?.health)
        XCTAssertEqual(stats.map(\.live), [nil, nil, nil], "例は live を持たない(既定のまま)")
    }

    /// `live`(#3694)は書いた時だけ true。省略は nil(開いた時に取り直すだけ)。
    func testLiveDecodesOnlyWhenWritten() throws {
        let live = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"healthStat","metric":"steps","label":"歩数","live":true}"#.utf8))
        XCTAssertEqual(live.live, true)
        let plain = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"healthStat","metric":"steps","label":"歩数"}"#.utf8))
        XCTAssertNil(plain.live)
        let off = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"healthStat","metric":"steps","label":"歩数","live":false}"#.utf8))
        XCTAssertEqual(off.live, false)
    }
}
