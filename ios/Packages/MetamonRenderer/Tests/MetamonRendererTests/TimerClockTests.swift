import XCTest
@testable import MetamonRenderer

/// #38: timerDisplay は保存した開始時刻から残り時間を出し、背面に回った時だけ終了通知を予約する。
final class TimerClockTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_000_000)

    func testRemainingAfterBackgroundGap() {
        let run = TimerRun(base: 0, startedAt: t0)
        // 180 秒タイマーを開始 → 70 秒裏に回って戻った
        XCTAssertEqual(run.remaining(at: t0 + 70, total: 180), 110)
        XCTAssertEqual(run.elapsed(at: t0 + 70, total: 180), 70)
    }

    func testRemainingNeverNegativeAndElapsedCapped() {
        let run = TimerRun(base: 0, startedAt: t0)
        XCTAssertEqual(run.remaining(at: t0 + 999, total: 180), 0)
        XCTAssertEqual(run.elapsed(at: t0 + 999, total: 180), 180)
        XCTAssertEqual(run.elapsed(at: t0 + 999), 999, "stopwatch は頭打ちしない")
    }

    func testResumeKeepsBase() {
        // 30 秒で一時停止 → 再開して 10 秒
        let run = TimerRun(base: 30, startedAt: t0)
        XCTAssertEqual(run.elapsed(at: t0 + 10), 40)
        XCTAssertEqual(run.endDate(total: 60), t0 + 30)
    }

    func testClockSkewBeforeStartIsZero() {
        XCTAssertEqual(TimerRun(base: 5, startedAt: t0).elapsed(at: t0 - 100), 5)
    }

    func testPausedHasNoEndDate() {
        XCTAssertNil(TimerRun(base: 20).endDate(total: 60))
        XCTAssertFalse(TimerRun(base: 20).isRunning)
    }

    func testPlanSchedulesForRunningCountdownRegardlessOfScreen() {
        let run = TimerRun(base: 0, startedAt: t0)
        XCTAssertEqual(TimerNotificationPlan.decide(isCountdown: true, run: run, total: 60, now: t0 + 10),
                       .schedule(at: t0 + 60), "開始した時点で予約する(前面・部品の表示に関係なく)")
        XCTAssertEqual(TimerNotificationPlan.decide(isCountdown: false, run: run, total: 60, now: t0 + 10),
                       .cancel, "stopwatch は終わりが無い")
        XCTAssertEqual(TimerNotificationPlan.decide(isCountdown: true, run: TimerRun(base: 10), total: 60, now: t0),
                       .cancel, "一時停止中は予約しない")
        XCTAssertEqual(TimerNotificationPlan.decide(isCountdown: true, run: run, total: 60, now: t0 + 61),
                       .cancel, "既に終わっていれば予約しない")
    }

    func testStoreRoundTripAndClearOnReset() {
        let store = TimerRunStore()
        let key = "test/\(UUID().uuidString)"
        store.save(TimerRun(base: 3, startedAt: t0), for: key)
        XCTAssertEqual(store.load(key), TimerRun(base: 3, startedAt: t0))
        store.save(TimerRun(), for: key)
        XCTAssertNil(store.load(key), "リセット(初期状態)は保存を消す")
    }

    func testClearAppRemovesOnlyThatAppsTimers() {
        let store = TimerRunStore.shared
        let app = UUID().uuidString.lowercased()
        let other = app + "2" // 前方一致で別アプリを巻き込まない
        let run = TimerRun(base: 5, startedAt: t0)
        store.save(run, for: "\(app)/home/timer")
        store.save(run, for: "\(app)/form/t2")
        store.save(run, for: "\(other)/home/timer")
        defer { store.clear("\(other)/home/timer") }

        store.clearApp(appId: app)

        XCTAssertNil(store.load("\(app)/home/timer"))
        XCTAssertNil(store.load("\(app)/form/t2"))
        XCTAssertEqual(store.load("\(other)/home/timer"), run)
    }
}
