import XCTest
@testable import MetamonRenderer

/// `healthStat` の `live`(#3694): 基準値 + 歩数計の増分の計算と、購読の開始・停止。HealthKit は使わない(偽の源)。
final class HealthLiveStepsTests: XCTestCase {
    // MARK: 純粋な計算

    func testValueIsBaselinePlusPedometerDelta() {
        var s = HealthLiveSteps(baseline: 6240)
        XCTAssertEqual(s.value, 6240)
        s.pedometer(cumulative: 12)
        XCTAssertEqual(s.value, 6252)
        s.pedometer(cumulative: 30) // 累計なので置き換え(足さない)
        XCTAssertEqual(s.value, 6270)
    }

    func testRebaseResetsDeltaSoStepsAreNotCountedTwice() {
        var s = HealthLiveSteps(baseline: 100)
        s.pedometer(cumulative: 25)
        s.rebase(125) // ヘルスケアを取り直したら増分は基準値に含まれている
        XCTAssertEqual(s.value, 125)
        XCTAssertEqual(s.delta, 0)
    }

    func testNegativeAndNaNAreTreatedAsZero() {
        var s = HealthLiveSteps(baseline: -5)
        XCTAssertEqual(s.baseline, 0)
        s.pedometer(cumulative: -3)
        XCTAssertEqual(s.value, 0)
        s.pedometer(cumulative: .nan)
        XCTAssertEqual(s.value, 0)
    }

    // MARK: 購読の状態機械

    final class FakeLiveSource: HealthLiveSource, @unchecked Sendable {
        private let lock = NSLock()
        private var _pedometer: AsyncStream<Double>.Continuation?
        private var _observer: AsyncStream<Void>.Continuation?
        private var _starts: [Date] = []
        private var _pedometerEnded = 0
        private var _observerEnded = 0

        var pedometer: AsyncStream<Double>.Continuation? { lock.withLock { _pedometer } }
        var observer: AsyncStream<Void>.Continuation? { lock.withLock { _observer } }
        var starts: [Date] { lock.withLock { _starts } }
        var pedometerEnded: Int { lock.withLock { _pedometerEnded } }
        var observerEnded: Int { lock.withLock { _observerEnded } }

        func pedometerUpdates(from: Date) -> AsyncStream<Double> {
            AsyncStream { cont in
                lock.withLock { _starts.append(from); _pedometer = cont }
                cont.onTermination = { [self] _ in lock.withLock { _pedometerEnded += 1 } }
            }
        }

        func stepCountChanges() -> AsyncStream<Void> {
            AsyncStream { cont in
                lock.withLock { _observer = cont }
                cont.onTermination = { [self] _ in lock.withLock { _observerEnded += 1 } }
            }
        }
    }

    @MainActor
    private func waitUntil(_ what: String, timeout: TimeInterval = 2, _ cond: @escaping () -> Bool) async {
        let deadline = Date().addingTimeInterval(timeout)
        while !cond() {
            if Date() > deadline { XCTFail("timed out: \(what)"); return }
            try? await Task.sleep(for: .milliseconds(10))
        }
    }

    @MainActor
    func testStartSubscribesAndStopUnsubscribes() async {
        let fake = FakeLiveSource()
        let monitor = HealthLiveStepsMonitor(baseline: 100, source: fake) { 999 }
        var seen: [Double] = []
        monitor.onChange = { seen.append($0) }
        XCTAssertFalse(monitor.isRunning)

        monitor.start()
        XCTAssertTrue(monitor.isRunning)
        await waitUntil("subscribed") { fake.pedometer != nil && fake.observer != nil }
        XCTAssertEqual(fake.starts.count, 1)

        fake.pedometer?.yield(0) // 購読直後は 0 歩
        fake.pedometer?.yield(7)
        await waitUntil("delta applied") { seen.last == 107 }
        XCTAssertEqual(seen, [100, 107])

        monitor.start() // 二重に購読しない
        XCTAssertEqual(fake.starts.count, 1)

        monitor.stop()
        XCTAssertFalse(monitor.isRunning)
        await waitUntil("streams ended") { fake.pedometerEnded == 1 && fake.observerEnded == 1 }
        fake.pedometer?.yield(50) // 止めた後の値は捨てる
        try? await Task.sleep(for: .milliseconds(50))
        XCTAssertEqual(seen.last, 107)
    }

    @MainActor
    func testHealthKitChangeRebasesAndRestartsPedometer() async {
        let fake = FakeLiveSource()
        let monitor = HealthLiveStepsMonitor(baseline: 100, source: fake) { 130 }
        var seen: [Double] = []
        monitor.onChange = { seen.append($0) }
        monitor.start()
        await waitUntil("subscribed") { fake.pedometer != nil && fake.observer != nil }
        fake.pedometer?.yield(5)
        await waitUntil("delta") { seen.last == 105 }

        fake.observer?.yield(())
        await waitUntil("rebased") { seen.last == 130 && fake.starts.count == 2 }
        XCTAssertEqual(fake.pedometerEnded, 1, "古い歩数計の購読は止める")
        XCTAssertEqual(monitor.steps, HealthLiveSteps(baseline: 130))

        fake.pedometer?.yield(3) // 新しい購読の累計は取り直しの時点から
        await waitUntil("new delta") { seen.last == 133 }
        monitor.stop()
    }

    @MainActor
    func testDayChangeRebasesInsteadOfAddingYesterdaysDelta() async {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        let fake = FakeLiveSource()
        let monitor = HealthLiveStepsMonitor(baseline: 9000, source: fake, calendar: cal) { 0 }
        var seen: [Double] = []
        monitor.onChange = { seen.append($0) }
        let yesterday = ISO8601DateFormatter().date(from: "2026-11-13T23:59:00+09:00")!
        monitor.start(now: yesterday)
        await waitUntil("subscribed") { fake.pedometer != nil }
        monitor.apply(pedometer: 40, now: yesterday)
        XCTAssertEqual(seen.last, 9040)
        monitor.apply(pedometer: 60, now: ISO8601DateFormatter().date(from: "2026-11-14T00:01:00+09:00")!)
        await waitUntil("rebased to the new day") { seen.last == 0 && fake.starts.count == 2 }
        monitor.stop()
    }

    @MainActor
    func testRefreshFailureKeepsCurrentValue() async {
        struct Boom: Error {}
        let fake = FakeLiveSource()
        let monitor = HealthLiveStepsMonitor(baseline: 100, source: fake) { throw Boom() }
        var seen: [Double] = []
        monitor.onChange = { seen.append($0) }
        monitor.start()
        await waitUntil("subscribed") { fake.observer != nil }
        fake.observer?.yield(())
        try? await Task.sleep(for: .milliseconds(50))
        XCTAssertEqual(seen, [])
        XCTAssertEqual(monitor.steps.value, 100)
        XCTAssertEqual(fake.starts.count, 1, "取り直せない時は歩数計を購読し直さない")
        monitor.stop()
    }

    func testUnavailableSourceEndsImmediately() async {
        let s = UnavailableHealthLiveSource()
        var n = 0
        for await _ in s.pedometerUpdates(from: Date()) { n += 1 }
        for await _ in s.stepCountChanges() { n += 1 }
        XCTAssertEqual(n, 0)
    }
}
