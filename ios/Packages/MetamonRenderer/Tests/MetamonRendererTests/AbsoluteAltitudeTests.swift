#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
#if os(iOS)
import CoreMotion
#endif
@testable import MetamonRenderer

/// 標高の口(#2226): 使えない端末では nil・精度の判定。
final class AbsoluteAltitudeTests: XCTestCase {
    func testQualityByWorstError() {
        XCTAssertEqual(AbsoluteAltitudeRule.quality(accuracy: 0, precision: 0), .good)
        XCTAssertEqual(AbsoluteAltitudeRule.quality(accuracy: 10, precision: 3), .good)
        XCTAssertEqual(AbsoluteAltitudeRule.quality(accuracy: 10.5, precision: 3), .fair)
        XCTAssertEqual(AbsoluteAltitudeRule.quality(accuracy: 3, precision: 50), .fair)
        XCTAssertEqual(AbsoluteAltitudeRule.quality(accuracy: 3, precision: 50.1), .poor, "悪い方で決める")
        XCTAssertEqual(AbsoluteAltitudeRule.quality(accuracy: 120, precision: 1), .poor)
    }

    func testQualityRejectsBrokenNumbers() {
        XCTAssertEqual(AbsoluteAltitudeRule.quality(accuracy: -1, precision: 1), .poor, "負の誤差は不明扱い")
        XCTAssertEqual(AbsoluteAltitudeRule.quality(accuracy: .nan, precision: 1), .poor)
        XCTAssertEqual(AbsoluteAltitudeRule.quality(accuracy: .infinity, precision: 1), .poor)
        let brokenAltitude = AbsoluteAltitudeSample(altitude: .nan, accuracy: 1, precision: 1)
        XCTAssertEqual(AbsoluteAltitudeRule.quality(of: brokenAltitude), .poor)
    }

    func testUsableAltitude() {
        let good = AbsoluteAltitudeSample(altitude: 42.5, accuracy: 4, precision: 2)
        let rough = AbsoluteAltitudeSample(altitude: 42.5, accuracy: 400, precision: 2)
        XCTAssertTrue(AbsoluteAltitudeRule.isUsable(good))
        XCTAssertFalse(AbsoluteAltitudeRule.isUsable(rough))
        XCTAssertEqual(AbsoluteAltitudeRule.usableAltitude(good), 42.5)
        XCTAssertNil(AbsoluteAltitudeRule.usableAltitude(rough), "精度が悪い値は捨てる")
        XCTAssertNil(AbsoluteAltitudeRule.usableAltitude(nil))
    }

    @MainActor func testUnavailableSourceReturnsNil() async {
        let source = UnavailableSensorData()
        XCTAssertEqual(source.absoluteAltitudeAvailability(), .unavailable)
        let sample = await source.absoluteAltitude(timeout: .milliseconds(10))
        XCTAssertNil(sample)
        var count = 0
        for await _ in source.absoluteAltitudes() { count += 1 }
        XCTAssertEqual(count, 0, "使えない端末では流れない")
    }

    /// mac(= 気圧計の無い端末と同じ道筋)では読めない。1 回読みは待たずに nil を返す。
    @MainActor func testLiveSourceOnDeviceWithoutAltimeter() async {
        let source = LiveSensorData()
        #if os(iOS)
        if CMAltimeter.isAbsoluteAltitudeAvailable() { return }
        #endif
        XCTAssertEqual(source.absoluteAltitudeAvailability(), .unavailable)
        let started = ContinuousClock.now
        let sample = await source.absoluteAltitude(timeout: .seconds(5))
        XCTAssertNil(sample)
        XCTAssertLessThan(started.duration(to: .now), .seconds(1), "使えない端末では待たずに諦める")
    }

    @MainActor func testMockSourceGivesUsableAltitude() async throws {
        let source = MockSensorData()
        XCTAssertEqual(source.absoluteAltitudeAvailability(), .available)
        let read = await source.absoluteAltitude(timeout: .seconds(1))
        let sample = try XCTUnwrap(read)
        XCTAssertEqual(AbsoluteAltitudeRule.quality(of: sample), .good)
        XCTAssertNotNil(AbsoluteAltitudeRule.usableAltitude(sample))

        var first: AbsoluteAltitudeSample?
        for await s in source.absoluteAltitudes() {
            first = s
            break
        }
        XCTAssertNotNil(first, "続けて読む口も値を出す")
    }
}
#endif
