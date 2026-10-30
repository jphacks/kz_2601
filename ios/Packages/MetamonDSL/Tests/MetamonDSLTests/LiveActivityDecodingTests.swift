import XCTest
@testable import MetamonDSL

/// #423: timerDisplay.liveActivity と startActivity / endActivity のデコード(例 ramen_timer)。
final class LiveActivityDecodingTests: XCTestCase {
    func testRamenTimerExample() throws {
        let dsl = try ExamplesFixture.load("ramen_timer")
        let all = dsl.screens.flatMap(\.allComponents)
        let timer = try XCTUnwrap(all.first { $0.type == .timerDisplay })
        XCTAssertEqual(timer.liveActivity, true)

        let start = try XCTUnwrap(all.first { $0.id == "queue" }?.action)
        XCTAssertEqual(start.type, .startActivity)
        XCTAssertEqual(start.activity, ActivitySpec(title: "行列に並び中", icon: "person.3", minutes: 30, stat: "month"))
        XCTAssertEqual(all.first { $0.id == "seated" }?.action?.type, .endActivity)
    }

    /// title の無い activity は捨てる(寛容)。未知の action は unknown のまま。
    func testLenient() throws {
        let json = #"{"type":"startActivity","activity":{"minutes":3}}"#
        let a = try JSONDecoder().decode(Action.self, from: Data(json.utf8))
        XCTAssertEqual(a.type, .startActivity)
        XCTAssertNil(a.activity)
        let timer = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"timerDisplay","mode":"stopwatch"}"#.utf8))
        XCTAssertNil(timer.liveActivity)
    }
}
