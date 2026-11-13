import CoreLocation
import Foundation
import MapKit

/// 地図の場所に Look Around と到着までの時間を出す口(#2230・親 #1832)。部品・式はまだ無い(決定待ち)。
/// 問い合わせは端末から Apple へ直接(本アプリのサーバーは通らない・API キー不要)。
/// MapKit には回数の上限があるので、まとめて求める時は同時に投げる数を `MapExtras.maxConcurrent` に絞る。

/// 移動の手段。
public enum MapTransport: String, CaseIterable, Sendable {
    case walking
    case automobile
    case transit

    var mapKit: MKDirectionsTransportType {
        switch self {
        case .walking: .walking
        case .automobile: .automobile
        case .transit: .transit
        }
    }
}

/// 到着までの時間。
public struct MapETA: Sendable, Equatable {
    /// 秒
    public var travelTime: TimeInterval
    /// メートル
    public var distance: CLLocationDistance
    public var expectedArrival: Date

    public init(travelTime: TimeInterval, distance: CLLocationDistance, expectedArrival: Date) {
        self.travelTime = travelTime
        self.distance = distance
        self.expectedArrival = expectedArrival
    }
}

/// 緯度経度(`CLLocationCoordinate2D` を Sendable な値に)。
public struct MapPoint: Sendable, Equatable, Hashable {
    public var latitude: Double
    public var longitude: Double

    public init(latitude: Double, longitude: Double) {
        self.latitude = latitude
        self.longitude = longitude
    }

    public var coordinate: CLLocationCoordinate2D { CLLocationCoordinate2D(latitude: latitude, longitude: longitude) }
    public var isValid: Bool { CLLocationCoordinate2DIsValid(coordinate) && !(latitude == 0 && longitude == 0) }
}

/// 1 か所の到着時間を求める(実体は `MKDirections.calculateETA`・テストでは偽物)。
public protocol MapETACalculating: Sendable {
    /// `from` が nil なら今の場所から。
    func eta(from: MapPoint?, to: MapPoint, transport: MapTransport) async throws -> MapETA
}

public enum MapExtras {
    /// まとめて求める時に同時に投げる数
    public static let maxConcurrent = 3

    /// 今の場所(`from` が nil)から複数の場所への到着時間。並びは `destinations` と同じ。
    /// 失敗した場所・座標が正しくない場所は nil(全体は失敗にしない)。同時に投げるのは `maxConcurrent` まで。
    public static func etas(from: MapPoint? = nil, to destinations: [MapPoint], transport: MapTransport,
                            maxConcurrent: Int = MapExtras.maxConcurrent,
                            calculator: any MapETACalculating = LiveMapETACalculator()) async -> [MapETA?] {
        var results = [MapETA?](repeating: nil, count: destinations.count)
        let work = destinations.enumerated().filter { $0.element.isValid }
        guard !work.isEmpty else { return results }
        let limit = max(1, maxConcurrent)
        await withTaskGroup(of: (Int, MapETA?).self) { group in
            var next = 0
            var running = 0
            while next < work.count || running > 0 {
                // 空きがある分だけ投げ、1 つ返るのを待つ
                while running < limit && next < work.count {
                    let (index, point) = work[next]
                    next += 1
                    running += 1
                    group.addTask {
                        guard !Task.isCancelled else { return (index, nil) }
                        return (index, try? await calculator.eta(from: from, to: point, transport: transport))
                    }
                }
                guard let (index, eta) = await group.next() else { break }
                running -= 1
                results[index] = eta
            }
        }
        return results
    }

    /// 座標の Look Around。撮影されていない場所・失敗は nil。
    public static func lookAroundScene(at point: MapPoint) async -> MKLookAroundScene? {
        guard point.isValid else { return nil }
        return try? await MKLookAroundSceneRequest(coordinate: point.coordinate).scene
    }
}

/// `MKDirections.calculateETA` で求める。
public struct LiveMapETACalculator: MapETACalculating {
    public init() {}

    public func eta(from: MapPoint?, to: MapPoint, transport: MapTransport) async throws -> MapETA {
        let request = MKDirections.Request()
        request.source = from.map { MKMapItem(placemark: MKPlacemark(coordinate: $0.coordinate)) } ?? .forCurrentLocation()
        request.destination = MKMapItem(placemark: MKPlacemark(coordinate: to.coordinate))
        request.transportType = transport.mapKit
        let r = try await MKDirections(request: request).calculateETA()
        return MapETA(travelTime: r.expectedTravelTime, distance: r.distance, expectedArrival: r.expectedArrivalDate)
    }
}
