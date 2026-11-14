import XCTest
import MapKit
import SwiftUI
import MetamonDSL
@testable import MetamonRenderer

/// `mapView`(#1346): "lat,lng" の読み取り・route の範囲合わせ・Mac の地図の操作。
final class MapViewLeafTests: XCTestCase {
    func testCoordinateParsing() {
        let c = MapViewLeaf.coordinate(.string("36.578056, 136.648611"))
        XCTAssertEqual(c?.latitude ?? 0, 36.578056, accuracy: 1e-9)
        XCTAssertEqual(c?.longitude ?? 0, 136.648611, accuracy: 1e-9)
        XCTAssertNil(MapViewLeaf.coordinate(.string("91,0")), "範囲外")
        XCTAssertNil(MapViewLeaf.coordinate(.string("36.5")), "片方だけ")
        XCTAssertNil(MapViewLeaf.coordinate(.string("")))
        XCTAssertNil(MapViewLeaf.coordinate(.null))
    }

    func testFitWithoutPointsUsesMinSpan() {
        let region = MapViewLeaf.fit([], minSpan: 500).region
        XCTAssertNotNil(region, "点が無くても東京駅を中心に出す")
        let expected = MKCoordinateRegion(center: region!.center, latitudinalMeters: 500, longitudinalMeters: 500)
        XCTAssertEqual(region!.span.latitudeDelta, expected.span.latitudeDelta, accuracy: 1e-9)
    }

    func testFitCentersAndCoversAllPoints() {
        let a = CLLocationCoordinate2D(latitude: 36.56, longitude: 136.64)
        let b = CLLocationCoordinate2D(latitude: 36.60, longitude: 136.70)
        guard let region = MapViewLeaf.fit([a, b], minSpan: 500).region else { return XCTFail("region") }
        XCTAssertEqual(region.center.latitude, 36.58, accuracy: 1e-9)
        XCTAssertEqual(region.center.longitude, 136.67, accuracy: 1e-9)
        XCTAssertGreaterThanOrEqual(region.span.latitudeDelta, 0.04, "両端が入る")
    }

    func testFitNeverNarrowerThanMinSpan() {
        let p = CLLocationCoordinate2D(latitude: 36.56, longitude: 136.64)
        guard let one = MapViewLeaf.fit([p], minSpan: 1_000).region else { return XCTFail("region") }
        let expected = MKCoordinateRegion(center: p, latitudinalMeters: 1_000, longitudinalMeters: 1_000)
        XCTAssertEqual(one.span.latitudeDelta, expected.span.latitudeDelta, accuracy: 1e-9)
    }

    func testInlineModesOnMacKeepScrollForThePage() {
        #if os(macOS)
        XCTAssertFalse(PlatformMap.inlineModes(.all).contains(.pan), "Mac はスクロールを画面の送りに残す")
        XCTAssertTrue(PlatformMap.inlineModes(.all).contains(.zoom), "拡大(ピンチ・+/-)は残す")
        XCTAssertEqual(PlatformMap.inlineModes([.pan, .zoom]), [.zoom])
        #else
        XCTAssertEqual(PlatformMap.inlineModes(.all), .all, "iOS は変えない")
        XCTAssertEqual(PlatformMap.inlineModes([.pan, .zoom]), [.pan, .zoom])
        #endif
    }
}
