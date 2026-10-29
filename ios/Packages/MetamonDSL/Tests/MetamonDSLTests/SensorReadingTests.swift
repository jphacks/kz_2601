import XCTest
@testable import MetamonDSL

/// `sensorValue`(#419): 単位変換・整形・間引き・一定間隔の記録・しきい値(サンプルを注入して確かめる)。
final class SensorReadingTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    func testUnitConversion() {
        XCTAssertEqual(SensorMath.degrees(.pi / 2), 90, accuracy: 1e-9)
        XCTAssertEqual(SensorMath.hectopascals(kilopascals: 101.325), 1013.25, accuracy: 1e-9)
        // 平置き 0° / 立てる 90° / 45°(表裏は区別しない)
        XCTAssertEqual(SensorMath.tiltDegrees(gravity: Vector3(0, 0, -1)), 0, accuracy: 1e-9)
        XCTAssertEqual(SensorMath.tiltDegrees(gravity: Vector3(0, -1, 0)), 90, accuracy: 1e-9)
        XCTAssertEqual(SensorMath.tiltDegrees(gravity: Vector3(0, -0.7071, 0.7071)), 45, accuracy: 0.01)
        XCTAssertEqual(SensorMath.tiltDegrees(gravity: Vector3(0, 0, 0)), 0)

        let m = MotionSample(pitch: .pi / 6, roll: -.pi / 4, gravity: Vector3(0, -1, 0),
                             userAcceleration: Vector3(0.3, 0.4, 0), rotationRate: Vector3(0, 0, .pi))
        XCTAssertEqual(SensorKind.pitch.value(from: m)!, 30, accuracy: 1e-9)
        XCTAssertEqual(SensorKind.roll.value(from: m)!, -45, accuracy: 1e-9)
        XCTAssertEqual(SensorKind.tilt.value(from: m)!, 90, accuracy: 1e-9)
        XCTAssertEqual(SensorKind.acceleration.value(from: m)!, 0.5, accuracy: 1e-9)
        XCTAssertEqual(SensorKind.rotation.value(from: m)!, 180, accuracy: 1e-9)
        XCTAssertNil(SensorKind.pressure.value(from: m))

        let a = AltitudeSample(pressureKPa: 100.5, relativeAltitude: 2.34)
        XCTAssertEqual(SensorKind.pressure.value(from: a)!, 1005, accuracy: 1e-9)
        XCTAssertEqual(SensorKind.altitude.value(from: a)!, 2.34, accuracy: 1e-9)
        XCTAssertNil(SensorKind.tilt.value(from: a))
        XCTAssertTrue(SensorKind.pressure.usesAltimeter && SensorKind.altitude.usesAltimeter && !SensorKind.tilt.usesAltimeter)
    }

    func testFormatting() {
        XCTAssertEqual(SensorKind.tilt.format(29.6), "30")
        XCTAssertEqual(SensorKind.tilt.format(-0.2), "0", "-0 にしない")
        XCTAssertEqual(SensorKind.acceleration.format(0.347), "0.35")
        XCTAssertEqual(SensorKind.pressure.format(1013.27), "1013.3", "桁区切りなし・1 桁")
        XCTAssertEqual(SensorKind.altitude.format(-1.04), "-1.0")
        XCTAssertEqual(SensorKind.rotation.rounded(12.4), 12)
        XCTAssertEqual(SensorKind.acceleration.rounded(0.123), 0.12)
        XCTAssertEqual(SensorKind.tilt.unit, "°")
        XCTAssertEqual(SensorKind.pressure.unit, "hPa")
        XCTAssertEqual(SensorKind.rotation.unit, "°/s")
    }

    func testBubbleMovesUphillAndClamps() {
        XCTAssertEqual(SensorMath.bubble(gravity: Vector3(0, 0, -1)).x, 0, accuracy: 1e-9)
        let right = SensorMath.bubble(gravity: Vector3(0.5, 0, -0.866)) // 右が下がる → 泡は左
        XCTAssertLessThan(right.x, 0)
        let edge = SensorMath.bubble(gravity: Vector3(1, 1, 0))
        XCTAssertEqual(abs(edge.x), 1, accuracy: 1e-9)
        XCTAssertEqual(abs(edge.y), 1, accuracy: 1e-9)
    }

    func testThrottleCapsAt10Hz() {
        var th = SensorThrottle(hz: 60) // 上限は 10 Hz
        XCTAssertEqual(th.interval, 0.1, accuracy: 1e-9)
        var emitted = 0
        for i in 0..<100 where th.shouldEmit(at: t0.addingTimeInterval(Double(i) / 50)) { emitted += 1 } // 50 Hz を 2 秒
        XCTAssertEqual(emitted, 20)
    }

    func testLogSessionIntervalAndLimit() {
        XCTAssertEqual(SensorLogSession(seconds: 0, limit: nil).interval, 1, "1 秒未満は 1 秒")
        XCTAssertEqual(SensorLogSession(seconds: 99_999, limit: nil).interval, 3600)
        XCTAssertEqual(SensorLogSession(seconds: 5, limit: nil).limit, 60)
        XCTAssertEqual(SensorLogSession(seconds: 5, limit: 50_000).limit, 1000)

        var s = SensorLogSession(seconds: 2, limit: 3)
        var at: [Double] = []
        for i in 0...100 { // 0.1 秒刻みで 10 秒
            let t = Double(i) / 10
            if s.tick(at: t0.addingTimeInterval(t)) { at.append(t) }
        }
        XCTAssertEqual(at, [0, 2, 4], "開始直後 + 2 秒ごと、3 件で止まる")
        XCTAssertTrue(s.isFinished)

        var late = SensorLogSession(seconds: 1, limit: 10)
        XCTAssertTrue(late.tick(at: t0))
        XCTAssertTrue(late.tick(at: t0.addingTimeInterval(30)), "背面から戻った等の遅れ")
        XCTAssertFalse(late.tick(at: t0.addingTimeInterval(30.5)), "遅れた分を一気に保存しない")
        XCTAssertTrue(late.tick(at: t0.addingTimeInterval(31)))
    }

    func testThresholdFiresOnEnterOnly() {
        var tr = SensorThresholdTrigger(above: 30, below: nil, cooldown: 3)
        let values: [Double] = [10, 31, 35, 40, 20, 33, 10, 10, 10, 32]
        var fired: [Int] = []
        for (i, v) in values.enumerated() where tr.update(v, at: t0.addingTimeInterval(Double(i))) { fired.append(i) }
        // 1 で入る → 4 で出る → 5 で入るが前回から 4 秒 → 発火、9 で再び入る(5 から 4 秒)→ 発火
        XCTAssertEqual(fired, [1, 5, 9])

        var cool = SensorThresholdTrigger(above: 30, below: nil, cooldown: 3)
        XCTAssertTrue(cool.update(31, at: t0))
        XCTAssertFalse(cool.update(10, at: t0.addingTimeInterval(1)))
        XCTAssertFalse(cool.update(31, at: t0.addingTimeInterval(2)), "3 秒以内は再発火しない")

        let band = SensorThresholdTrigger(above: 20, below: -20)
        XCTAssertTrue(band.matches(-25) && band.matches(25) && !band.matches(0))
    }

    func testPipelineEvents() {
        var p = SensorPipeline(threshold: SensorThreshold(above: 30, save: true), canSave: true)
        p.startLog(seconds: 1, limit: 2)
        var events: [SensorPipeline.Event] = []
        var shown = 0
        // 20 Hz で 3 秒: 0 秒に 10°、1 秒から 40°
        for i in 0..<60 {
            let t = Double(i) / 20
            if let e = p.ingest(t < 1 ? 10 : 40, at: t0.addingTimeInterval(t)) {
                shown += 1
                events += e
            }
        }
        XCTAssertEqual(shown, 30, "表示は 10 Hz に間引く")
        XCTAssertEqual(events, [.save(10), .fired(40), .save(40), .logFinished(2)],
                       "0 秒の記録・1 秒でしきい値(保存は記録と 1 件にまとめる)・2 件で記録が止まる")
        XCTAssertFalse(p.isLogging)
        XCTAssertTrue(p.isAlerting)

        var noSave = SensorPipeline(threshold: SensorThreshold(above: 1, save: true), canSave: false)
        XCTAssertEqual(noSave.ingest(5, at: t0), [.fired(5)], "保存先が無ければ保存しない")
        var none = SensorPipeline(threshold: SensorThreshold(message: "x"), canSave: true)
        XCTAssertEqual(none.ingest(5, at: t0), [])
    }

    func testExampleDecodes() throws {
        let dsl = try ExamplesFixture.load("sensors")
        let svs = dsl.screens.flatMap(\.components).filter { $0.type == .sensorValue }
        XCTAssertEqual(svs.compactMap(\.sensor), ["tilt", "pressure", "altitude"])
        let level = try XCTUnwrap(svs.first)
        XCTAssertEqual(level.style, "level")
        XCTAssertEqual(level.threshold?.above, 30)
        XCTAssertEqual(level.threshold?.message, "傾きすぎです")
        let pressure = svs[1]
        XCTAssertEqual(pressure.entity, "reading")
        XCTAssertEqual(pressure.seconds, 60)
        XCTAssertEqual(pressure.limit, 120)
        XCTAssertTrue(ClientCapabilities.reviewExcludedComponents.contains(.sensorValue))
        XCTAssertTrue(DSLExamples.reviewExcluded.contains("sensors"))
    }
}
