import XCTest
import MapKit
import MetamonDSL
@testable import MetamonRenderer

/// 地図の Look Around と到着時間の読む口(#2230)。偽物で、同時に投げる数の絞り・取れなかった場所の扱いを確かめる。
final class MapExtrasTests: XCTestCase {
    private let here = RouteGuidance.Coord(lat: 36.578056, lng: 136.648611) // 金沢駅

    private func coord(_ i: Int) -> RouteGuidance.Coord {
        RouteGuidance.Coord(lat: 36.5 + Double(i) / 1_000, lng: 136.6)
    }

    private func eta(_ minutes: Double) -> MapETA {
        MapETA(travelTime: minutes * 60, distance: minutes * 80,
               expectedArrival: Date(timeIntervalSince1970: 1_800_000_000 + minutes * 60))
    }

    func testETAsKeepOrderAndReturnNilForFailures() async {
        let destinations = (0..<5).map(coord)
        var table: [String: MapETA] = [:]
        for i in [0, 2, 4] { table[FakeMapExtras.key(coord(i))] = eta(Double(i + 1)) }
        let fake = FakeMapExtras(etas: table, delay: .milliseconds(5))
        let results = await MapExtras.etas(from: here, to: destinations, mode: .walking, source: fake)
        XCTAssertEqual(results.count, destinations.count)
        XCTAssertEqual(results.map { $0?.travelTime }, [60, nil, 180, nil, 300], "順は渡した通り・取れなかった場所は nil")
    }

    func testETAsLimitConcurrentRequests() async {
        let destinations = (0..<12).map(coord)
        let fake = FakeMapExtras(delay: .milliseconds(10))
        _ = await MapExtras.etas(from: here, to: destinations, mode: .automobile, source: fake)
        let peak = await fake.inFlight.peak
        let current = await fake.inFlight.current
        XCTAssertLessThanOrEqual(peak, MapExtras.maxConcurrentRequests, "同時に投げる数を絞る")
        XCTAssertEqual(peak, MapExtras.maxConcurrentRequests, "絞りの数までは同時に投げる")
        XCTAssertEqual(current, 0, "全部終わっている")
    }

    func testETAsWithFewerDestinationsThanLimit() async {
        let fake = FakeMapExtras(etas: [FakeMapExtras.key(coord(0)): eta(3)], delay: .milliseconds(5))
        let results = await MapExtras.etas(from: here, to: [coord(0)], mode: .transit, source: fake)
        XCTAssertEqual(results.map { $0?.travelTime }, [180])
        let peak = await fake.inFlight.peak
        XCTAssertEqual(peak, 1)
    }

    func testEmptyDestinations() async {
        let fake = FakeMapExtras()
        let results = await MapExtras.etas(from: here, to: [], mode: .walking, source: fake)
        XCTAssertTrue(results.isEmpty)
        let peak = await fake.inFlight.peak
        XCTAssertEqual(peak, 0)
    }

    func testLookAroundReturnsNilWithoutThrowing() async {
        let scene = await FakeMapExtras().lookAroundScene(at: coord(0))
        XCTAssertNil(scene, "Look Around が無い場所は nil")
    }

    func testTravelModesMapToMapKit() {
        XCTAssertEqual(MapTravelMode.walking.transportType, .walking)
        XCTAssertEqual(MapTravelMode.automobile.transportType, .automobile)
        XCTAssertEqual(MapTravelMode.transit.transportType, .transit)
        XCTAssertEqual(MapTravelMode.allCases.count, 3)
    }
}
