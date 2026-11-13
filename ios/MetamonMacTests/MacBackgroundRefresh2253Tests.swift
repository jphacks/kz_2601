import XCTest
@testable import Metamon

/// #2253: ウインドウが閉じていても時々取り直す(NSBackgroundActivityScheduler・決まりは iOS と共有の BackgroundRefreshCycle)。
@MainActor
final class MacBackgroundRefresh2253Tests: XCTestCase {
    private var calls: [String] = []

    private func make(signedIn: Bool, online: Bool) -> MacBackgroundRefresh {
        MacBackgroundRefresh(isSignedIn: { signedIn }, isOnline: { online },
                             drain: { [weak self] in self?.calls.append("drain") },
                             refetch: { [weak self] in self?.calls.append("refetch") })
    }

    func testSignedOutDoesNothing() async {
        let refresh = make(signedIn: false, online: true)
        let outcome = await refresh.run()
        XCTAssertEqual(outcome, .skippedSignedOut)
        XCTAssertEqual(calls, [])
        XCTAssertEqual(refresh.refreshCount, 0)
    }

    func testOfflineDoesNothing() async {
        let refresh = make(signedIn: true, online: false)
        let outcome = await refresh.run()
        XCTAssertEqual(outcome, .skippedOffline)
        XCTAssertEqual(calls, [])
    }

    func testDrainsQueueThenRefetches() async {
        let refresh = make(signedIn: true, online: true)
        let outcome = await refresh.run()
        XCTAssertEqual(outcome, .refreshed)
        XCTAssertEqual(calls, ["drain", "refetch"])
        XCTAssertEqual(refresh.refreshCount, 1)
    }

    func testSignedOutWinsOverOffline() {
        XCTAssertEqual(BackgroundRefreshCycle.skipReason(signedIn: false, online: false), .skippedSignedOut)
        XCTAssertNil(BackgroundRefreshCycle.skipReason(signedIn: true, online: true))
    }

    /// 間隔は iOS の refresh と同じ目安(15 分)
    func testIntervalMatchesIOS() {
        XCTAssertEqual(BackgroundRefreshCycle.interval, 15 * 60)
        XCTAssertEqual(MacBackgroundRefresh.identifier, "jp.kz2601.metamon.refresh")
    }

    func testInstallTwiceIsHarmless() {
        let refresh = make(signedIn: false, online: false)
        refresh.install()
        refresh.install()
        refresh.uninstall()
    }
}
