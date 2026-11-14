import XCTest
import MapKit
import MetamonDSL
@testable import MetamonRenderer

/// `locationPicker`(#212): 値の形(locationButton と同じ "lat,lng")と nameField への保存。
final class LocationPickerTests: XCTestCase {
    func testValueShapeMatchesLocationButton() {
        let p = PickedPlace(name: "兼六園", latitude: 36.5621284, longitude: 136.6626519)
        XCTAssertEqual(p.locationValue, "36.562128,136.662652")
        XCTAssertEqual(PickedPlace(name: nil, latitude: -33.8567844, longitude: -73.9855).locationValue, "-33.856784,-73.985500")
    }

    func testApplyWritesLocationAndName() {
        var values: [String: JSONValue] = ["name": .null, "place": .null]
        PlacePicking.apply(PickedPlace(name: " 兼六園 ", latitude: 36.562128, longitude: 136.662652),
                           to: &values, field: "place", nameField: "name")
        XCTAssertEqual(values["place"]?.stringValue, "36.562128,136.662652")
        XCTAssertEqual(values["name"]?.stringValue, "兼六園")
    }

    func testPinWithoutNameKeepsTypedName() {
        var values: [String: JSONValue] = ["name": .string("いつものカフェ")]
        PlacePicking.apply(PickedPlace(name: nil, latitude: 36.578056, longitude: 136.648611),
                           to: &values, field: "place", nameField: "name")
        XCTAssertEqual(values["place"]?.stringValue, "36.578056,136.648611")
        XCTAssertEqual(values["name"]?.stringValue, "いつものカフェ", "ピン・現在地(名前なし)は打った名前を消さない")
    }

    func testApplyWithoutNameField() {
        var values: [String: JSONValue] = [:]
        PlacePicking.apply(PickedPlace(name: "兼六園", latitude: 1, longitude: 2), to: &values, field: "place", nameField: nil)
        XCTAssertEqual(values.count, 1)
        // nameField が field と同じ(誤った DSL)でも location を名前で上書きしない
        PlacePicking.apply(PickedPlace(name: "兼六園", latitude: 1, longitude: 2), to: &values, field: "place", nameField: "place")
        XCTAssertEqual(values["place"]?.stringValue, "1.000000,2.000000")
    }

    func testCoordinateParsing() {
        let c = PlacePicking.coordinate(.string("36.5781, 136.6486"))
        XCTAssertEqual(c?.latitude ?? 0, 36.5781, accuracy: 1e-9)
        XCTAssertEqual(c?.longitude ?? 0, 136.6486, accuracy: 1e-9)
        XCTAssertNil(PlacePicking.coordinate(.string("91,0")))
        XCTAssertNil(PlacePicking.coordinate(.string("兼六園")))
        XCTAssertNil(PlacePicking.coordinate(.null))
        XCTAssertNil(PlacePicking.coordinate(nil))
    }

    @MainActor func testDecodesNameField() throws {
        let json = #"{"type":"locationPicker","id":"place_in","field":"place","nameField":"name","label":"場所を検索"}"#
        let c = try JSONDecoder().decode(Component.self, from: Data(json.utf8))
        XCTAssertEqual(c.type, .locationPicker)
        XCTAssertEqual(c.field, "place")
        XCTAssertEqual(c.nameField, "name")
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "locationPicker"))
    }

    /// Mac で地図をクリックした点を中心にする(#1345): 縮尺は今のまま、無ければ 2km 四方。
    func testRecenteredKeepsSpan() {
        let tapped = CLLocationCoordinate2D(latitude: 36.562128, longitude: 136.662652)
        let now = MKCoordinateRegion(center: .init(latitude: 35, longitude: 139),
                                     span: MKCoordinateSpan(latitudeDelta: 0.05, longitudeDelta: 0.07))
        let r = PlacePicking.recentered(now, on: tapped)
        XCTAssertEqual(r.center.latitude, tapped.latitude, accuracy: 1e-9)
        XCTAssertEqual(r.center.longitude, tapped.longitude, accuracy: 1e-9)
        XCTAssertEqual(r.span.latitudeDelta, 0.05, accuracy: 1e-9)
        XCTAssertEqual(r.span.longitudeDelta, 0.07, accuracy: 1e-9)
        // クリックした点の "lat,lng" は iPhone のピンと同じ形
        XCTAssertEqual(PickedPlace(name: nil, latitude: r.center.latitude, longitude: r.center.longitude).locationValue,
                       "36.562128,136.662652")
    }

    func testRecenteredWithoutRegionUsesDefault() {
        let tapped = CLLocationCoordinate2D(latitude: 35.681236, longitude: 139.767125)
        let r = PlacePicking.recentered(nil, on: tapped)
        XCTAssertEqual(r.center.latitude, tapped.latitude, accuracy: 1e-9)
        XCTAssertGreaterThan(r.span.latitudeDelta, 0)
        XCTAssertEqual(r.span.latitudeDelta, MKCoordinateRegion(center: tapped, latitudinalMeters: 2_000, longitudinalMeters: 2_000).span.latitudeDelta,
                       accuracy: 1e-9)
    }
}
