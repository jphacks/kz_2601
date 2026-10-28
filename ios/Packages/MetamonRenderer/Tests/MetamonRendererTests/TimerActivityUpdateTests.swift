import XCTest
@testable import MetamonRenderer

/// #423: timerDisplay の状態 → ロック画面に出す段階(走行中 / 一時停止 / 終了 / 消す)。
final class TimerActivityUpdateTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_000_000)

    private func phase(total: Int?, _ run: TimerRun) -> TimerActivityUpdate.Phase {
        TimerActivityUpdate(key: "k", label: "麺", total: total, run: run).phase
    }

    func testCountdownRunningEndsAtTotal() {
        XCTAssertEqual(phase(total: 180, TimerRun(base: 0, startedAt: t0)), .running(start: t0, end: t0 + 180))
        // 60 秒で止めて再開: 見かけの開始は 60 秒前、終わりは再開から 120 秒後(TimerRun.endDate と一致)
        let resumed = TimerRun(base: 60, startedAt: t0)
        XCTAssertEqual(phase(total: 180, resumed), .running(start: t0 - 60, end: t0 + 120))
        XCTAssertEqual(resumed.endDate(total: 180), t0 + 120)
    }

    func testStopwatchRunningHasNoEnd() {
        XCTAssertEqual(phase(total: nil, TimerRun(base: 15, startedAt: t0)), .running(start: t0 - 15, end: nil))
    }

    func testPausedShowsRemainingOrElapsed() {
        XCTAssertEqual(phase(total: 180, TimerRun(base: 50)), .paused(shown: 130))
        XCTAssertEqual(phase(total: nil, TimerRun(base: 50)), .paused(shown: 50))
    }

    func testFinishedAndReset() {
        XCTAssertEqual(phase(total: 180, TimerRun(base: 180)), .finished)
        XCTAssertEqual(phase(total: 180, TimerRun()), .ended)
        XCTAssertEqual(phase(total: nil, TimerRun()), .ended)
    }
}
