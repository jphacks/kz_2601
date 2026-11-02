#if !APP_STORE_REVIEW
import XCTest
@testable import MetamonRenderer

/// 位置の記録の状態の公開(#559)。ホストの「記録中」バーが読む値。
@MainActor
final class LocationTrackingStatusTests: XCTestCase {
    private let target = LocationTracking.Target(appName: "散歩", entity: "points", locationField: "where", dateField: "at")
    private let start = Date(timeIntervalSince1970: 1_800_000_000)

    func testRecordingExposesStartAndTarget() {
        let s = LocationTracking.status(phase: .recording, target: target, startedAt: start)
        XCTAssertEqual(s, LocationTrackingStatus(isTracking: true, startedAt: start, appName: "散歩", entity: "points"))
    }

    func testRequestingIsTrackingWithoutStart() {
        // 前回の startedAt が残っていても、許可待ちの間は出さない
        let s = LocationTracking.status(phase: .requesting, target: target, startedAt: start)
        XCTAssertTrue(s.isTracking)
        XCTAssertTrue(s.isRequesting)
        XCTAssertNil(s.startedAt)
        XCTAssertEqual(s.appName, "散歩")
    }

    func testStoppedStatesAreIdle() {
        for phase: LocationTracking.Phase in [.idle, .denied, .failed("x")] {
            XCTAssertEqual(LocationTracking.status(phase: phase, target: target, startedAt: start), .idle)
        }
    }

    func testControlReadsSharedSessionAndStopKeepsIdle() {
        XCTAssertEqual(LocationTrackingControl.status, LocationTracking.shared.status)
        LocationTrackingControl.stop()
        XCTAssertEqual(LocationTracking.shared.phase, .idle)
        XCTAssertEqual(LocationTrackingControl.status, .idle)
    }
}
#endif
