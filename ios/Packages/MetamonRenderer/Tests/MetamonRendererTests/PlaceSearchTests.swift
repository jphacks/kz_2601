import XCTest
import CoreLocation
import MetamonDSL
@testable import MetamonRenderer

/// `placeSearch`(近くの場所)と `locationButton.addressField`(住所): フォームへの書き込み・距離の表示・並び・住所の形。
final class PlaceSearchTests: XCTestCase {
    private let kenrokuen = PickedPlace(name: " 兼六園 ", address: "石川県金沢市兼六町1", latitude: 36.562128, longitude: 136.662652)

    func testApplyFillsNameLocationAndAddress() {
        var values: [String: JSONValue] = ["name": .null, "place": .null, "address": .null]
        PlaceSearch.apply(kenrokuen, to: &values, nameField: "name", locationField: "place", addressField: "address")
        XCTAssertEqual(values["name"]?.stringValue, "兼六園")
        XCTAssertEqual(values["place"]?.stringValue, "36.562128,136.662652", "locationButton / locationPicker と同じ形")
        XCTAssertEqual(values["address"]?.stringValue, "石川県金沢市兼六町1")
    }

    func testApplySkipsMissingAndEmpty() {
        var values: [String: JSONValue] = ["address": .string("手で打った住所")]
        PlaceSearch.apply(PickedPlace(name: nil, address: "  ", latitude: 1, longitude: 2), to: &values,
                          nameField: "name", locationField: nil, addressField: "address")
        XCTAssertNil(values["name"], "名前が無ければ書かない")
        XCTAssertEqual(values["address"]?.stringValue, "手で打った住所", "空の住所で打った値を消さない")
    }

    func testApplySameFieldTwiceKeepsLocation() {
        var values: [String: JSONValue] = [:]
        PlaceSearch.apply(kenrokuen, to: &values, nameField: "place", locationField: "place", addressField: "place")
        XCTAssertEqual(values["place"]?.stringValue, "36.562128,136.662652")
        XCTAssertEqual(values.count, 1)
    }

    func testDistanceText() {
        XCTAssertEqual(PlaceSearch.distanceText(3), "10 m")
        XCTAssertEqual(PlaceSearch.distanceText(347), "350 m")
        XCTAssertEqual(PlaceSearch.distanceText(1_234), "1.2 km")
        XCTAssertEqual(PlaceSearch.distanceText(12_600), "13 km")
    }

    func testNearestFirstSortsAndCaps() {
        let here = PlacePicking.mockHere.coordinate // 金沢駅
        let sorted = PlaceSearch.nearestFirst(PlacePicking.mockResults, from: here)
        XCTAssertEqual(sorted.map(\.place.name), ["近江町市場", "兼六園", "金沢21世紀美術館"].sorted { a, b in
            dist(a, here) < dist(b, here)
        })
        XCTAssertTrue(zip(sorted, sorted.dropFirst()).allSatisfy { ($0.meters ?? 0) <= ($1.meters ?? 0) })
        let many = (0..<40).map { PickedPlace(name: "p\($0)", latitude: 36 + Double($0) / 1000, longitude: 136) }
        XCTAssertEqual(PlaceSearch.nearestFirst(many, from: here).count, PlaceSearch.maxResults)
        XCTAssertNil(PlaceSearch.nearestFirst(many, from: nil).first?.meters, "位置が無ければ距離は出さない")
    }

    private func dist(_ name: String, _ here: CLLocationCoordinate2D) -> CLLocationDistance {
        let p = PlacePicking.mockResults.first { $0.name == name }!
        return CLLocation(latitude: here.latitude, longitude: here.longitude).distance(from: CLLocation(latitude: p.latitude, longitude: p.longitude))
    }

    func testCategoriesMapToMapKit() {
        XCTAssertEqual(PlaceSearch.categories.count, 18)
        XCTAssertEqual(PlaceSearch.category("movieTheater"), .movieTheater)
        XCTAssertEqual(PlaceSearch.category("station"), .publicTransport)
        XCTAssertNil(PlaceSearch.category("zoo"))
        XCTAssertNil(PlaceSearch.category(nil))
    }

    func testAddressText() {
        let jp = PlaceSearch.AddressParts(country: "JP", administrativeArea: "石川県", locality: "金沢市", subLocality: "広坂",
                                         thoroughfare: "広坂1丁目", subThoroughfare: "1-1")
        XCTAssertEqual(PlaceSearch.addressText(jp), "石川県金沢市広坂1丁目1-1")
        let us = PlaceSearch.AddressParts(country: "US", administrativeArea: "CA", locality: "Cupertino", subLocality: nil,
                                         thoroughfare: "Apple Park Way", subThoroughfare: "1")
        XCTAssertEqual(PlaceSearch.addressText(us), "1 Apple Park Way, Cupertino, CA")
        XCTAssertNil(PlaceSearch.addressText(.init()))
    }

    @MainActor func testDecodesAndRegisters() throws {
        let json = #"{"type":"placeSearch","category":"cafe","query":"コワーキング","near":"current","nameField":"name","locationField":"place","addressField":"address","pins":true}"#
        let c = try JSONDecoder().decode(Component.self, from: Data(json.utf8))
        XCTAssertEqual(c.type, .placeSearch)
        XCTAssertEqual(c.category, "cafe")
        XCTAssertEqual(c.query, "コワーキング")
        XCTAssertEqual(c.near, "current")
        XCTAssertEqual(c.addressField, "address")
        XCTAssertEqual(c.pins, true)
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "placeSearch"))
        let button = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"locationButton","field":"place","addressField":"address"}"#.utf8))
        XCTAssertEqual(button.addressField, "address")
    }
}
