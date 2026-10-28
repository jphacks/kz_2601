#if !APP_STORE_REVIEW
import XCTest
import SwiftUI
import MetamonDSL
@testable import MetamonRenderer

/// `sensorValue`(#419): 保存する 1 件・gauge の範囲・モックの波形・各 style の描画。
final class SensorValueLeafTests: XCTestCase {
    private func sensors() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/sensors.json"))
    }

    private func component(_ dsl: AppDSL, _ id: String) throws -> Component {
        try XCTUnwrap(dsl.screens.flatMap(\.components).first { $0.id == id })
    }

    @MainActor func testRecordPayload() throws {
        let dsl = try sensors()
        let c = try component(dsl, "pressure")
        let at = Date(timeIntervalSince1970: 1_800_000_000)
        let data = try XCTUnwrap(SensorValueLeaf.record(kind: .pressure, value: 1013.27, component: c, entity: dsl.entity("reading"), at: at))
        XCTAssertEqual(data["hpa"], .number(1013.3), "表示と同じ桁で保存")
        XCTAssertEqual(data["at"], .string(FieldCoding.datetimeFormatter.string(from: at)))
        XCTAssertEqual(data.count, 2)

        // 保存先の無い部品・computed の field には保存しない
        XCTAssertNil(SensorValueLeaf.record(kind: .tilt, value: 3, component: try component(dsl, "level"), entity: dsl.entity("reading"), at: at))
        XCTAssertNil(SensorValueLeaf.record(kind: .pressure, value: 1, component: c, entity: nil, at: at))
    }

    func testGaugeRange() {
        XCTAssertEqual(SensorValueLeaf.range(.acceleration, min: nil, max: nil), 0...2)
        XCTAssertEqual(SensorValueLeaf.range(.tilt, min: 0, max: 45), 0...45)
        XCTAssertEqual(SensorValueLeaf.range(.tilt, min: 50, max: 10), 0...90, "壊れた範囲は既定に戻す")
    }

    func testMockWaveCrossesThreshold() {
        XCTAssertEqual(MockSensorData.frame(.tilt, at: 3).value, 40, accuracy: 0.5, "12 秒周期の山で 40°")
        XCTAssertEqual(MockSensorData.frame(.tilt, at: 9).value, 0, accuracy: 0.5)
        XCTAssertNotNil(MockSensorData.frame(.tilt, at: 3).motion)
        XCTAssertEqual(MockSensorData.frame(.pressure, at: 0).value, 1013.2, accuracy: 0.1)
        XCTAssertNil(MockSensorData.frame(.altitude, at: 0).motion)
    }

    @MainActor func testUnavailableAndMockSources() async {
        XCTAssertEqual(UnavailableSensorData().availability(.tilt), .unavailable)
        var n = 0
        for await _ in UnavailableSensorData().frames(.tilt) { n += 1 }
        XCTAssertEqual(n, 0)

        let mock = MockSensorData()
        XCTAssertEqual(mock.availability(.pressure), .available)
        var got: [Double] = []
        for await f in mock.frames(.pressure) {
            got.append(f.value)
            if got.count == 2 { break } // break で購読を取り消す(センサー停止の経路)
        }
        XCTAssertEqual(got.count, 2)
    }

    /// 各 style と「使えません」が落ちずに描ける(ImageRenderer では .task は走らないので初期値で描く)。
    @MainActor func testRendersEveryState() throws {
        let dsl = try sensors()
        let ctx = RenderContext(route: Route(screen: "home"))
        let level = try component(dsl, "level")
        let pressure = try component(dsl, "pressure")
        let tiltFrame = MockSensorData.frame(.tilt, at: 2)
        var gauge = pressure
        gauge.style = "gauge"
        let cases: [(String, SensorValueLeaf)] = [
            ("level", SensorValueLeaf(c: level, ctx: ctx, initial: tiltFrame, availability: .available)),
            ("number+save", SensorValueLeaf(c: pressure, ctx: ctx, initial: SensorFrame(value: 1013.2), availability: .available)),
            ("gauge", SensorValueLeaf(c: gauge, ctx: ctx, initial: SensorFrame(value: 1013.2), availability: .available)),
            ("loading", SensorValueLeaf(c: pressure, ctx: ctx)),
            ("unavailable", SensorValueLeaf(c: level, ctx: ctx, availability: .unavailable)),
            ("denied", SensorValueLeaf(c: pressure, ctx: ctx, availability: .denied)),
        ]
        for (name, view) in cases {
            let r = ImageRenderer(content: view.frame(width: 360, height: 240))
            r.scale = 1
            let img = try XCTUnwrap(r.uiImage, name)
            XCTAssertEqual(img.size.width, 360, name)
        }
    }

    func testRegisteredInStandardRegistry() {
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "sensorValue"))
    }
}
#endif
