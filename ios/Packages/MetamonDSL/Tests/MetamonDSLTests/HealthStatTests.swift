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
    }
}
