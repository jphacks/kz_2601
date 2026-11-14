import Foundation
import CoreLocation
import MapKit
import MetamonDSL

/// `placeSearch`: 現在地の近くの場所を Apple のマップで探す(MKLocalSearch / MKLocalPointsOfInterestRequest・API キー不要)。
/// 問い合わせは端末から Apple へ直接(本アプリのサーバーは通らない)。`locationButton.addressField` の逆ジオコーディングもここ。
public enum PlaceSearch {
    /// 探す範囲(現在地からの半径)。
    static let radius: CLLocationDistance = 3_000
    /// 出す件数の上限。
    static let maxResults = 15

    /// DSL の category → MapKit の種類(iOS 17 にあるものだけ・backend `dsl.PlaceCategories` と同じ並び)。
    static let categories: [(String, MKPointOfInterestCategory)] = [
        ("cafe", .cafe), ("restaurant", .restaurant), ("bakery", .bakery), ("store", .store),
        ("station", .publicTransport), ("park", .park), ("hospital", .hospital), ("pharmacy", .pharmacy),
        ("parking", .parking), ("gasStation", .gasStation), ("evCharger", .evCharger), ("hotel", .hotel),
        ("museum", .museum), ("library", .library), ("movieTheater", .movieTheater), ("school", .school),
        ("university", .university), ("fitness", .fitnessCenter),
    ]

    static func category(_ name: String?) -> MKPointOfInterestCategory? {
        guard let name else { return nil }
        return categories.first { $0.0 == name }?.1
    }

    /// 選んだ場所を入力中のフォームに書く。空の名前・住所は書かない(打った値を消さない)。
    /// 同じ field が 2 つに指定されていたら(誤った DSL)、location を優先し、名前で住所を上書きしない。
    public static func apply(_ place: PickedPlace, to values: inout [String: JSONValue],
                             nameField: String?, locationField: String?, addressField: String?) {
        func nonEmpty(_ s: String?) -> String? {
            guard let t = s?.trimmingCharacters(in: .whitespacesAndNewlines), !t.isEmpty else { return nil }
            return t
        }
        var written = Set<String>()
        if let f = nonEmpty(locationField) {
            values[f] = .string(place.locationValue)
            written.insert(f)
        }
        if let f = nonEmpty(nameField), !written.contains(f), let name = nonEmpty(place.name) {
            values[f] = .string(name)
            written.insert(f)
        }
        if let f = nonEmpty(addressField), !written.contains(f), let address = nonEmpty(place.address) {
            values[f] = .string(address)
        }
    }

    /// 現在地からの距離の表示(1 km 未満は 10 m 単位、それ以上は 0.1 km 単位・10 km 以上は km 単位)。
    public static func distanceText(_ meters: CLLocationDistance) -> String {
        if meters < 1_000 { return "\(max(10, Int((meters / 10).rounded()) * 10)) m" }
        if meters < 10_000 { return String(format: "%.1f km", meters / 1_000) }
        return "\(Int((meters / 1_000).rounded())) km"
    }

    /// 近い順に並べて上限で切る。
    static func nearestFirst(_ places: [PickedPlace], from here: CLLocationCoordinate2D?) -> [(place: PickedPlace, meters: CLLocationDistance?)] {
        guard let here else { return places.prefix(maxResults).map { ($0, nil) } }
        let origin = CLLocation(latitude: here.latitude, longitude: here.longitude)
        return places
            .map { ($0, origin.distance(from: CLLocation(latitude: $0.latitude, longitude: $0.longitude))) }
            .sorted { $0.1 < $1.1 }
            .prefix(maxResults)
            .map { ($0.0, Optional($0.1)) }
    }

    /// 近くを探す。`here` が無い(位置を断られた)時は語だけで探す(category だけなら探せない)。モックは固定 3 件。
    static func search(query: String?, category: String?, here: CLLocationCoordinate2D?) async -> [PickedPlace] {
        let q = query?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let cat = Self.category(category)
        if PlacePicking.isMock { return (q.isEmpty && cat == nil) ? [] : PlacePicking.mockResults }
        let response: MKLocalSearch.Response?
        if q.isEmpty {
            guard let cat, let here else { return [] }
            let request = MKLocalPointsOfInterestRequest(center: here, radius: radius)
            request.pointOfInterestFilter = MKPointOfInterestFilter(including: [cat])
            response = try? await MKLocalSearch(request: request).start()
        } else {
            let request = MKLocalSearch.Request()
            request.naturalLanguageQuery = q
            if let cat {
                request.resultTypes = .pointOfInterest
                request.pointOfInterestFilter = MKPointOfInterestFilter(including: [cat])
            } else {
                request.resultTypes = [.pointOfInterest, .address]
            }
            if let here { request.region = MKCoordinateRegion(center: here, latitudinalMeters: radius * 2, longitudinalMeters: radius * 2) }
            response = try? await MKLocalSearch(request: request).start()
        }
        return (response?.mapItems ?? []).map { item in
            let c = item.placemark.coordinate
            return PickedPlace(name: item.name, address: item.placemark.title, latitude: c.latitude, longitude: c.longitude)
        }
    }

    // MARK: - 住所(逆ジオコーディング)

    /// 住所の部品。CLPlacemark から作る(テストでは直接作る)。
    struct AddressParts: Equatable {
        var country: String?
        var administrativeArea: String?
        var locality: String?
        var subLocality: String?
        var thoroughfare: String?
        var subThoroughfare: String?
    }

    /// 日本は「石川県金沢市広坂1丁目1-1」の形(区切りなし)、それ以外は「1-1 Main St, City, State」。
    static func addressText(_ p: AddressParts) -> String? {
        func clean(_ s: String?) -> String? {
            guard let t = s?.trimmingCharacters(in: .whitespaces), !t.isEmpty else { return nil }
            return t
        }
        let street = clean(p.thoroughfare) ?? clean(p.subLocality)
        let text: String
        if p.country == "JP" {
            text = [p.administrativeArea, p.locality, street, p.subThoroughfare].compactMap(clean).joined()
        } else {
            let line = [clean(p.subThoroughfare), street].compactMap { $0 }.joined(separator: " ")
            text = [clean(line), clean(p.locality), clean(p.administrativeArea)].compactMap { $0 }.joined(separator: ", ")
        }
        return text.isEmpty ? nil : text
    }

    /// モックの現在地(金沢駅)の住所。
    static let mockAddress = "石川県金沢市木ノ新保町1-1"

    /// 座標 → 住所(取れなければ nil)。座標は端末から Apple に送られる。
    static func address(of coordinate: CLLocationCoordinate2D) async -> String? {
        if PlacePicking.isMock { return mockAddress }
        let loc = CLLocation(latitude: coordinate.latitude, longitude: coordinate.longitude)
        guard let p = try? await CLGeocoder().reverseGeocodeLocation(loc).first else { return nil }
        return addressText(AddressParts(country: p.isoCountryCode, administrativeArea: p.administrativeArea, locality: p.locality,
                                        subLocality: p.subLocality, thoroughfare: p.thoroughfare, subThoroughfare: p.subThoroughfare))
    }
}
