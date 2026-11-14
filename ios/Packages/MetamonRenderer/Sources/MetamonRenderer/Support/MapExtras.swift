import Foundation
import CoreLocation
import MapKit
import MetamonDSL

/// 地図の Look Around と到着までの時間を読む口(#2230・親 #1832)。部品・AI の決まりはまだ無い(決定待ち)。
/// `DeviceCalendarSource` と同じ形: protocol + 実体(`LiveMapExtras`)+ 偽物(`FakeMapExtras`)。
/// 問い合わせは端末から Apple のマップへ直接(本アプリのサーバーは通らない)。座標はログに出さない。
public protocol MapExtrasSource: Sendable {
    /// その座標の Look Around。見られない場所・取れない時は nil(投げない)。
    func lookAroundScene(at coordinate: RouteGuidance.Coord) async -> MKLookAroundScene?
    /// 1 地点への到着までの時間。経路が無い・公共交通が無い地域などは nil(投げない)。
    func eta(from origin: RouteGuidance.Coord, to destination: RouteGuidance.Coord, mode: MapTravelMode) async -> MapETA?
}

/// 到着時間を求める移動の手段。道案内(`RouteGuidance.Mode`)に無い公共交通を足したもの。
public enum MapTravelMode: String, Sendable, CaseIterable {
    case walking, automobile, transit

    var transportType: MKDirectionsTransportType {
        switch self {
        case .walking: .walking
        case .automobile: .automobile
        case .transit: .transit
        }
    }
}

public struct MapETA: Sendable, Equatable {
    public var travelTime: TimeInterval
    public var distance: CLLocationDistance
    public var expectedArrival: Date

    public init(travelTime: TimeInterval, distance: CLLocationDistance, expectedArrival: Date) {
        self.travelTime = travelTime
        self.distance = distance
        self.expectedArrival = expectedArrival
    }
}

public enum MapExtras {
    /// 読む口の実体。アプリ起動時に 1 回だけ差し替える。
    @MainActor public static var source: any MapExtrasSource = LiveMapExtras()

    /// まとめて求める時に同時に投げる数(MapKit の回数の上限に当たらないよう絞る)。
    public static let maxConcurrentRequests = 3

    @MainActor
    public static func lookAroundScene(at coordinate: RouteGuidance.Coord) async -> MKLookAroundScene? {
        await source.lookAroundScene(at: coordinate)
    }

    @MainActor
    public static func etas(from origin: RouteGuidance.Coord, to destinations: [RouteGuidance.Coord],
                            mode: MapTravelMode) async -> [MapETA?] {
        await etas(from: origin, to: destinations, mode: mode, source: source)
    }

    /// 今の場所から複数の場所への到着時間。順は `destinations` のまま、取れなかった場所は nil。
    /// 同時に投げるのは `maxConcurrentRequests` 件までで、1 件返るたびに次を足す。
    public static func etas(from origin: RouteGuidance.Coord, to destinations: [RouteGuidance.Coord],
                            mode: MapTravelMode, source: any MapExtrasSource) async -> [MapETA?] {
        guard !destinations.isEmpty else { return [] }
        var results = [MapETA?](repeating: nil, count: destinations.count)
        await withTaskGroup(of: (index: Int, eta: MapETA?).self) { group in
            var next = 0
            func request() {
                let index = next
                let destination = destinations[index]
                group.addTask { (index, await source.eta(from: origin, to: destination, mode: mode)) }
                next += 1
            }
            while next < min(maxConcurrentRequests, destinations.count) { request() }
            while let done = await group.next() {
                results[done.index] = done.eta
                if Task.isCancelled { break }
                if next < destinations.count { request() }
            }
            group.cancelAll()
        }
        return results
    }
}

// MARK: - 偽物

/// `METAMON_MOCK=1`・テスト・プレビュー用。返す値と、同時にいくつ動いたかを外から見られる。
public struct FakeMapExtras: MapExtrasSource {
    /// 到着時間を返す場所("lat,lng")。載っていない場所は nil
    public var etas: [String: MapETA]
    public var delay: Duration
    public let inFlight = InFlightCounter()

    public init(etas: [String: MapETA] = [:], delay: Duration = .milliseconds(20)) {
        self.etas = etas
        self.delay = delay
    }

    public func lookAroundScene(at coordinate: RouteGuidance.Coord) async -> MKLookAroundScene? { nil }

    public func eta(from origin: RouteGuidance.Coord, to destination: RouteGuidance.Coord, mode: MapTravelMode) async -> MapETA? {
        await inFlight.enter()
        try? await Task.sleep(for: delay)
        let found = etas[FakeMapExtras.key(destination)]
        await inFlight.leave()
        return found
    }

    public static func key(_ c: RouteGuidance.Coord) -> String { String(format: "%.6f,%.6f", c.lat, c.lng) }

    /// 同時に動いている数と、その最大。
    public actor InFlightCounter {
        public private(set) var current = 0
        public private(set) var peak = 0

        func enter() {
            current += 1
            peak = max(peak, current)
        }

        func leave() { current -= 1 }
    }
}

// MARK: - 実体

/// MapKit から読む。Look Around(`MKLookAroundSceneRequest`)も到着時間(`MKDirections.calculateETA`)も許可は要らない。
public struct LiveMapExtras: MapExtrasSource {
    public init() {}

    public func lookAroundScene(at coordinate: RouteGuidance.Coord) async -> MKLookAroundScene? {
        let request = MKLookAroundSceneRequest(coordinate: CLLocationCoordinate2D(latitude: coordinate.lat, longitude: coordinate.lng))
        return try? await request.scene
    }

    public func eta(from origin: RouteGuidance.Coord, to destination: RouteGuidance.Coord, mode: MapTravelMode) async -> MapETA? {
        let request = MKDirections.Request()
        request.source = Self.mapItem(origin)
        request.destination = Self.mapItem(destination)
        request.transportType = mode.transportType
        guard let response = try? await MKDirections(request: request).calculateETA() else { return nil }
        return MapETA(travelTime: response.expectedTravelTime, distance: response.distance,
                      expectedArrival: response.expectedArrivalDate)
    }

    /// MKPlacemark は iOS 26 で非推奨。iOS 26 以降は init(location:address:) を使う。
    private static func mapItem(_ c: RouteGuidance.Coord) -> MKMapItem {
        if #available(iOS 26.0, macOS 26.0, *) {
            return MKMapItem(location: CLLocation(latitude: c.lat, longitude: c.lng), address: nil)
        }
        return MKMapItem(placemark: MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: c.lat, longitude: c.lng)))
    }
}
