import XCTest
@testable import MetamonRenderer

/// 到着までの時間をまとめて求める口(#2230)。同時の数の絞り・失敗した場所は nil・並びを確かめる。
final class MapExtras2230Tests: XCTestCase {
    private actor Counter {
        var inFlight = 0
        var peak = 0
        var calls = 0
        func enter() { inFlight += 1; calls += 1; peak = max(peak, inFlight) }
        func leave() { inFlight -= 1 }
    }

    private struct FakeCalculator: MapETACalculating {
        let counter: Counter
        /// この緯度の場所は失敗する
        var failing: Set<Double> = []

        func eta(from: MapPoint?, to: MapPoint, transport: MapTransport) async throws -> MapETA {
            await counter.enter()
            try? await Task.sleep(for: .milliseconds(20))
            await counter.leave()
            if failing.contains(to.latitude) { throw URLError(.timedOut) }
            return MapETA(travelTime: to.latitude * 60, distance: to.latitude * 100, expectedArrival: Date(timeIntervalSince1970: to.latitude))
        }
    }

    private func points(_ n: Int) -> [MapPoint] {
        (1...n).map { MapPoint(latitude: Double($0), longitude: 136.6) }
    }

    func testConcurrencyIsLimited() async {
        let counter = Counter()
        let got = await MapExtras.etas(to: points(10), transport: .walking, maxConcurrent: 3, calculator: FakeCalculator(counter: counter))
        let peak = await counter.peak
        let calls = await counter.calls
        XCTAssertEqual(got.count, 10)
        XCTAssertLessThanOrEqual(peak, 3)
        XCTAssertGreaterThan(peak, 1) // 本当に並行している
        XCTAssertEqual(calls, 10)
    }

    func testDefaultLimit() async {
        let counter = Counter()
        _ = await MapExtras.etas(to: points(8), transport: .automobile, calculator: FakeCalculator(counter: counter))
        let peak = await counter.peak
        XCTAssertLessThanOrEqual(peak, MapExtras.maxConcurrent)
    }

    func testFailedPlaceIsNilAndOrderKept() async {
        let counter = Counter()
        let got = await MapExtras.etas(to: points(4), transport: .walking, calculator: FakeCalculator(counter: counter, failing: [2]))
        XCTAssertEqual(got.map { $0?.travelTime }, [60, nil, 180, 240])
    }

    func testInvalidCoordinateIsNilWithoutCall() async {
        let counter = Counter()
        let bad = [MapPoint(latitude: 200, longitude: 0), MapPoint(latitude: 0, longitude: 0), MapPoint(latitude: 3, longitude: 136.6)]
        let got = await MapExtras.etas(to: bad, transport: .transit, calculator: FakeCalculator(counter: counter))
        let calls = await counter.calls
        XCTAssertNil(got[0])
        XCTAssertNil(got[1])
        XCTAssertEqual(got[2]?.distance, 300)
        XCTAssertEqual(calls, 1)
    }

    func testEmptyAndZeroLimit() async {
        let counter = Counter()
        let empty = await MapExtras.etas(to: [], transport: .walking, calculator: FakeCalculator(counter: counter))
        XCTAssertTrue(empty.isEmpty)
        let one = await MapExtras.etas(to: points(2), transport: .walking, maxConcurrent: 0, calculator: FakeCalculator(counter: counter))
        XCTAssertEqual(one.compactMap { $0 }.count, 2)
        let peak = await counter.peak
        XCTAssertEqual(peak, 1)
    }

    func testTransportMapping() {
        XCTAssertEqual(MapTransport.walking.mapKit, .walking)
        XCTAssertEqual(MapTransport.automobile.mapKit, .automobile)
        XCTAssertEqual(MapTransport.transit.mapKit, .transit)
    }

    func testLookAroundInvalidPointIsNil() async {
        let scene = await MapExtras.lookAroundScene(at: MapPoint(latitude: 91, longitude: 0))
        XCTAssertNil(scene)
    }
}
