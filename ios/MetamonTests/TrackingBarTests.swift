#if !APP_STORE_REVIEW
import XCTest
import SwiftUI
import MetamonRenderer
@testable import Metamon

/// アプリ全体の「記録中」バー(#559)。出す条件・経過時間の表示・停止が記録の停止に届くか。
@MainActor
final class TrackingBarTests: XCTestCase {
    private let start = Date(timeIntervalSince1970: 1_800_000_000)

    func testVisibleOnlyWhileTracking() {
        XCTAssertFalse(TrackingBarModel.isVisible(.idle))
        XCTAssertTrue(TrackingBarModel.isVisible(.init(isTracking: true, isRequesting: true)))
        XCTAssertTrue(TrackingBarModel.isVisible(.init(isTracking: true, startedAt: start, appName: "散歩", entity: "points")))
    }

    func testElapsedClock() {
        let s = LocationTrackingStatus(isTracking: true, startedAt: start)
        XCTAssertEqual(TrackingBarModel.clock(TrackingBarModel.elapsed(s, now: start.addingTimeInterval(754))), "12:34")
        XCTAssertEqual(TrackingBarModel.clock(TrackingBarModel.elapsed(s, now: start.addingTimeInterval(3723))), "1:02:03")
        // 許可待ち(startedAt なし)・時計の巻き戻りは 0
        XCTAssertEqual(TrackingBarModel.elapsed(.init(isTracking: true, isRequesting: true), now: start), 0)
        XCTAssertEqual(TrackingBarModel.elapsed(s, now: start.addingTimeInterval(-5)), 0)
    }

    func testSpokenDurationIsLocalized() {
        XCTAssertEqual(TrackingBarModel.spokenDuration(754, locale: Locale(identifier: "en_US")), "12 minutes, 34 seconds")
        XCTAssertTrue(TrackingBarModel.spokenDuration(754, locale: Locale(identifier: "ja_JP")).contains("12分"))
    }

    func testStopButtonCallsThroughToHostStop() {
        var stops = 0
        let host = TrackingBarHost(status: { .init(isTracking: true, startedAt: self.start) }, stop: { stops += 1 })
        let bar = TrackingBar(status: host.status(), onStop: host.stop)
        bar.onStop()
        XCTAssertEqual(stops, 1)
    }

    func testDefaultHostStopsSharedTracking() {
        TrackingBarHost().stop()
        XCTAssertEqual(TrackingBarHost().status(), .idle)
        XCTAssertEqual(LocationTrackingControl.status, .idle)
    }
}
#endif
