import XCTest
@testable import MetamonDSL

/// 位置の記録(#168): 点列の並べ替え・10 分の空きでの分割・距離 / 時間・間引き・表示。
final class TrackTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    private func rec(_ loc: String, _ offset: TimeInterval) -> Record {
        Record(id: UUID().uuidString, data: ["loc": .string(loc), "at": .string(FieldCoding.datetimeFormatter.string(from: t0.addingTimeInterval(offset)))])
    }

    func testPointsSortedAndSegmentedByGap() {
        let records = [
            rec("35.0010,139.0", 60), rec("35.0000,139.0", 0), rec("35.0020,139.0", 120),
            rec("35.1000,139.0", 120 + 3600), rec("35.1010,139.0", 180 + 3600), // 1 時間後の別の散歩
            rec("壊れた値", 30),
        ]
        let pts = Track.points(records, locationField: "loc", by: "at")
        XCTAssertEqual(pts.count, 5)
        XCTAssertEqual(pts.first?.lat, 35.0)
        let segs = Track.segments(pts)
        XCTAssertEqual(segs.map(\.count), [3, 2])
        // 0.001° 緯度 ≒ 111 m: 2 区間 + 1 区間 = 約 333 m(散歩の間の 11 km は数えない)
        XCTAssertEqual(Track.totalDistance(segs), 333, accuracy: 2)
        XCTAssertEqual(Track.totalDuration(segs), 120 + 60, accuracy: 0.5)
        XCTAssertEqual(Track.aggregate("distance", records: records, locationField: "loc", by: "at"), 333, accuracy: 2)
        XCTAssertEqual(Track.aggregate("duration", records: records, locationField: "loc", by: "at"), 180, accuracy: 0.5)
    }

    func testFormat() {
        XCTAssertEqual(Track.formatDistance(850.4), "850 m")
        XCTAssertEqual(Track.formatDistance(1234), "1.23 km")
        XCTAssertEqual(Track.formatDuration(65 * 60), "1:05:00")
        XCTAssertEqual(Track.formatDuration(59), "0:59")
        XCTAssertTrue(Track.isTrackAggregate("distance"))
        XCTAssertFalse(Track.isTrackAggregate("sum"))
    }

    func testShouldKeepThinsPoints() {
        let a = Track.Point(lat: 35, lng: 139, date: t0)
        XCTAssertTrue(Track.shouldKeep(a, accuracy: 10, last: nil))
        XCTAssertFalse(Track.shouldKeep(a, accuracy: 80, last: nil), "精度 50 m 超は捨てる")
        XCTAssertFalse(Track.shouldKeep(a, accuracy: -1, last: nil), "無効な精度")
        let near = Track.Point(lat: 35.0001, lng: 139, date: t0.addingTimeInterval(30)) // 約 11 m
        XCTAssertFalse(Track.shouldKeep(near, accuracy: 10, last: a))
        let far = Track.Point(lat: 35.0003, lng: 139, date: t0.addingTimeInterval(30)) // 約 33 m
        XCTAssertTrue(Track.shouldKeep(far, accuracy: 10, last: a))
        let later = Track.Point(lat: 35.0001, lng: 139, date: t0.addingTimeInterval(301))
        XCTAssertTrue(Track.shouldKeep(later, accuracy: 10, last: a), "止まっていても 5 分に 1 点")
    }

    func testAutoStopClamp() {
        XCTAssertEqual(Track.autoStopSeconds(nil), 3600)
        XCTAssertEqual(Track.autoStopSeconds(10), 300)
        XCTAssertEqual(Track.autoStopSeconds(99_999), 14_400)
    }

    func testMockRouteIsAWalk() {
        let pts = Track.mockRoute.enumerated().map { Track.Point(lat: $1.lat, lng: $1.lng, date: t0.addingTimeInterval(Double($0) * 2)) }
        XCTAssertEqual(Track.totalDistance(Track.segments(pts)), 1300, accuracy: 300)
    }

    func testWalkRouteExampleDecodes() throws {
        let dsl = try ExamplesFixture.load("walk_route")
        let comps = dsl.screens.flatMap(\.components)
        let tracker = try XCTUnwrap(comps.first { $0.type == .locationTracker })
        XCTAssertEqual(tracker.entity, "point")
        XCTAssertEqual(tracker.locationField, "loc")
        XCTAssertEqual(tracker.dateField, "at")
        XCTAssertEqual(tracker.seconds, 3600)
        let map = try XCTUnwrap(comps.first { $0.type == .mapView })
        XCTAssertEqual(map.kind, "route")
        XCTAssertEqual(comps.filter { Track.isTrackAggregate($0.aggregate) }.count, 2)
    }
}
