import Foundation
import MapKit
import MetamonDSL

/// `locationPicker`(#212)で選んだ場所。名前は検索結果から(地図のピン・現在地は nil)。
public struct PickedPlace: Equatable, Sendable, Identifiable {
    public var name: String?
    public var address: String?
    public var latitude: Double
    public var longitude: Double

    public init(name: String?, address: String? = nil, latitude: Double, longitude: Double) {
        self.name = name
        self.address = address
        self.latitude = latitude
        self.longitude = longitude
    }

    public var id: String { "\(name ?? "")@\(locationValue)" }
    public var coordinate: CLLocationCoordinate2D { .init(latitude: latitude, longitude: longitude) }

    /// location field の値。locationButton と同じ "lat,lng"(小数 6 桁)。
    public var locationValue: String { String(format: "%.6f,%.6f", latitude, longitude) }
}

public enum PlacePicking {
    /// 選んだ場所をフォームの値に書く。`nameField` があり場所に名前があれば、そこにも入れる
    /// (ピン・現在地で名前が無い時は、ユーザーが打った名前を消さない)。
    public static func apply(_ place: PickedPlace, to values: inout [String: JSONValue], field: String, nameField: String?) {
        values[field] = .string(place.locationValue)
        if let nameField, !nameField.isEmpty, nameField != field,
           let name = place.name?.trimmingCharacters(in: .whitespacesAndNewlines), !name.isEmpty {
            values[nameField] = .string(name)
        }
    }

    /// "lat,lng" → 座標(範囲外・形違いは nil)。
    public static func coordinate(_ value: JSONValue?) -> CLLocationCoordinate2D? {
        guard let s = value?.stringValue else { return nil }
        let parts = s.split(separator: ",").map { Double($0.trimmingCharacters(in: .whitespaces)) }
        guard parts.count == 2, let lat = parts[0], let lng = parts[1],
              (-90...90).contains(lat), (-180...180).contains(lng) else { return nil }
        return .init(latitude: lat, longitude: lng)
    }

    static var isMock: Bool { ProcessInfo.processInfo.environment["METAMON_MOCK"] == "1" }

    /// モックの現在地(金沢駅)。
    static let mockHere = PickedPlace(name: nil, latitude: 36.578056, longitude: 136.648611)

    /// モック(METAMON_MOCK=1)の固定の検索結果。どの語でも同じ 3 件を返す(UI テスト用)。
    static let mockResults: [PickedPlace] = [
        PickedPlace(name: "兼六園", address: "石川県金沢市兼六町1", latitude: 36.562128, longitude: 136.662652),
        PickedPlace(name: "金沢21世紀美術館", address: "石川県金沢市広坂1-2-1", latitude: 36.560870, longitude: 136.658172),
        PickedPlace(name: "近江町市場", address: "石川県金沢市上近江町50", latitude: 36.571476, longitude: 136.656650),
    ]

    /// 店名・地名・住所で検索(MapKit・API キー不要)。モックは固定結果。
    static func search(_ query: String, near region: MKCoordinateRegion?) async -> [PickedPlace] {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return [] }
        if isMock { return mockResults }
        let request = MKLocalSearch.Request()
        request.naturalLanguageQuery = q
        request.resultTypes = [.pointOfInterest, .address]
        if let region { request.region = region }
        guard let response = try? await MKLocalSearch(request: request).start() else { return [] }
        return response.mapItems.prefix(15).map { item in
            let c = item.placemark.coordinate
            return PickedPlace(name: item.name, address: item.placemark.title, latitude: c.latitude, longitude: c.longitude)
        }
    }
}
