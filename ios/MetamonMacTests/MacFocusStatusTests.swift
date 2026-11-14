import XCTest
import AppKit
@testable import MetamonServices

/// 集中モード中かを読む口を Mac でも(#2328・#1990 の Mac)。OS の許可の画面は出さない(`requesting: false` だけ)。
@MainActor
final class MacFocusStatusTests: XCTestCase {
    func testSystemReadingsRefreshWhenMacAppBecomesActive() {
        XCTAssertEqual(FocusStatusReadings.system.notificationNames, [NSApplication.didBecomeActiveNotification],
                       "Mac は前に来た時に読み直す")
    }

    func testLiveSourceOnMacDoesNotCrashAndHidesValueWithoutPermission() async {
        let source = LiveFocusStatusSource()
        let auth = await source.authorization(requesting: false)
        let status = source.current
        XCTAssertEqual(status.authorization, auth)
        if auth != .authorized {
            XCTAssertNil(status.isFocused, "許可が無ければ nil")
        }
    }

    func testLiveSourceRereadsOnActivate() {
        var auth = FocusAuthorization.authorized
        var focused: Bool? = false
        let center = NotificationCenter()
        let readings = FocusStatusReadings(authorization: { auth }, requestAuthorization: { $0(auth) },
                                           isFocused: { focused }, notificationNames: [NSApplication.didBecomeActiveNotification])
        let source = LiveFocusStatusSource(readings: readings, center: center)
        var seen: [Bool?] = []
        let observation = source.observe { seen.append($0.isFocused) }
        focused = true // Mac で集中モードを入れた
        center.post(name: NSApplication.didBecomeActiveNotification, object: nil)
        XCTAssertEqual(seen, [false, true], "前に来た時に「集中モード中」が届く")
        auth = .denied
        center.post(name: NSApplication.didBecomeActiveNotification, object: nil)
        XCTAssertEqual(seen, [false, true, nil], "許可を外したら nil")
        observation.cancel()
    }

    func testReviewBuildNeverAsks() async {
        let source = FocusStatusSources.make(reviewBuild: true, environment: [:])
        let auth = await source.authorization(requesting: true)
        XCTAssertEqual(auth, .restricted, "審査用の版は OS の許可を求めない")
        XCTAssertNil(source.isFocused)
    }

    func testMockOnMac() {
        let source = FocusStatusSources.make(reviewBuild: false, environment: ["METAMON_MOCK": "1", "METAMON_FOCUSED": "1"])
        XCTAssertTrue(source is FakeFocusStatusSource)
        XCTAssertEqual(source.isFocused, true)
    }
}
