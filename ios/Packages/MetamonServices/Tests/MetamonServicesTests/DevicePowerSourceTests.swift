import XCTest
@testable import MetamonServices

/// 端末の電池・充電・省電力モード(#1989)。OS の値は `DevicePowerReadings` で差し替える。
@MainActor
final class DevicePowerSourceTests: XCTestCase {
    /// 差し替え用の端末
    private final class FakeDevice {
        var monitoring = false
        var monitoringSets: [Bool] = []
        var level: Float = 0.5
        var charge: DevicePowerState.Charge = .unplugged
        var lowPower = false

        @MainActor
        var readings: DevicePowerReadings {
            DevicePowerReadings(
                isMonitoringEnabled: { self.monitoring },
                setMonitoringEnabled: { self.monitoring = $0; self.monitoringSets.append($0) },
                // 監視が無効だと UIDevice は -1/unknown を返す
                rawLevel: { self.monitoring ? self.level : -1 },
                charge: { self.monitoring ? self.charge : .unknown },
                isLowPowerMode: { self.lowPower },
                notificationNames: [Self.changed]
            )
        }

        static let changed = Notification.Name("test.devicePower.changed")
    }

    // MARK: - 変換

    func testConversionUnknownChargingFullLowPower() {
        let unplugged = DevicePowerState.from(rawLevel: 0.42, charge: .unplugged, isLowPowerMode: false)
        XCTAssertEqual(unplugged.batteryLevel ?? 0, 0.42, accuracy: 0.0001)
        XCTAssertFalse(unplugged.isCharging)
        XCTAssertFalse(unplugged.isFull)

        let charging = DevicePowerState.from(rawLevel: 0.3, charge: .charging, isLowPowerMode: false)
        XCTAssertTrue(charging.isCharging)
        XCTAssertFalse(charging.isFull)

        let full = DevicePowerState.from(rawLevel: 1, charge: .full, isLowPowerMode: false)
        XCTAssertTrue(full.isCharging, "満充電もつながっている扱い")
        XCTAssertTrue(full.isFull)
        XCTAssertEqual(full.batteryLevel, 1)

        let low = DevicePowerState.from(rawLevel: 0.1, charge: .unplugged, isLowPowerMode: true)
        XCTAssertTrue(low.isLowPowerMode)
    }

    func testSimulatorMinusOneAndUnknownBecomeNil() {
        // シミュレータ・監視無効: -1 と unknown
        XCTAssertEqual(DevicePowerState.from(rawLevel: -1, charge: .unknown, isLowPowerMode: false), .unknown)
        // 充電の状態が unknown なら残りも信用しない
        XCTAssertNil(DevicePowerState.from(rawLevel: 0.5, charge: .unknown, isLowPowerMode: false).batteryLevel)
        // 範囲外の値も不明
        XCTAssertNil(DevicePowerState.from(rawLevel: -1, charge: .unplugged, isLowPowerMode: false).batteryLevel)
        XCTAssertNil(DevicePowerState.from(rawLevel: 1.5, charge: .charging, isLowPowerMode: false).batteryLevel)
        // 省電力モードは電池が不明でも読める
        XCTAssertTrue(DevicePowerState.from(rawLevel: -1, charge: .unknown, isLowPowerMode: true).isLowPowerMode)
    }

    // MARK: - 実体(監視の出し入れ)

    func testObserveThenCancelRestoresMonitoringFlag() {
        for initial in [false, true] {
            let device = FakeDevice()
            device.monitoring = initial
            let source = LiveDevicePowerSource(readings: device.readings, center: NotificationCenter())
            var got: [DevicePowerState] = []
            let a = source.observe { got.append($0) }
            let b = source.observe { _ in }
            XCTAssertTrue(device.monitoring)
            XCTAssertEqual(got.first?.batteryLevel ?? 0, 0.5, accuracy: 0.0001, "登録した時に今の値が届く")
            a.cancel()
            XCTAssertTrue(device.monitoring, "まだ見ている人がいる間は有効のまま")
            b.cancel()
            XCTAssertEqual(device.monitoring, initial, "最後の監視が止まったら元の値に戻る")
            XCTAssertFalse(source.isObserving)
        }
    }

    func testNotificationDeliversOnlyChanges() {
        let device = FakeDevice()
        let center = NotificationCenter()
        let source = LiveDevicePowerSource(readings: device.readings, center: center)
        var got: [DevicePowerState] = []
        let obs = source.observe { got.append($0) }

        center.post(name: FakeDevice.changed, object: nil)
        XCTAssertEqual(got.count, 1, "変わっていなければ届けない")

        device.charge = .charging
        center.post(name: FakeDevice.changed, object: nil)
        device.lowPower = true
        center.post(name: FakeDevice.changed, object: nil)
        XCTAssertEqual(got.map(\.charge), [.unplugged, .charging, .charging])
        XCTAssertEqual(got.last?.isLowPowerMode, true)

        obs.cancel()
        device.level = 0.1
        center.post(name: FakeDevice.changed, object: nil)
        XCTAssertEqual(got.count, 3, "止めた後は届かない")
    }

    func testCurrentWithoutObservingReadsAndRestores() {
        let device = FakeDevice()
        let source = LiveDevicePowerSource(readings: device.readings, center: NotificationCenter())
        XCTAssertEqual(source.current.batteryLevel ?? 0, 0.5, accuracy: 0.0001, "読む間だけ有効にする")
        XCTAssertFalse(device.monitoring, "読んだら元に戻す")
    }

    func testSourcesSharingMonitoringDoNotTurnEachOtherOff() {
        for initial in [false, true] {
            let device = FakeDevice()
            device.monitoring = initial
            let count = BatteryMonitoringCount(device.readings)
            let a = LiveDevicePowerSource(readings: device.readings, center: NotificationCenter(), monitoring: count)
            let b = LiveDevicePowerSource(readings: device.readings, center: NotificationCenter(), monitoring: count)
            let obsA = a.observe { _ in }
            XCTAssertEqual(b.current.batteryLevel ?? 0, 0.5, accuracy: 0.0001)
            XCTAssertTrue(device.monitoring, "別の実体が読んでも、見ている実体の監視は消さない")
            let obsB = b.observe { _ in }
            obsA.cancel()
            XCTAssertTrue(device.monitoring, "別の実体がまだ見ている")
            obsB.cancel()
            XCTAssertEqual(device.monitoring, initial, "全部止まったら元の値に戻る")
            XCTAssertEqual(count.count, 0)
            if initial { XCTAssertFalse(device.monitoringSets.contains(false), "元から有効なら一度も無効にしない") }
        }
    }

    func testSystemSourcesShareOneCount() {
        let a = LiveDevicePowerSource()
        let b = LiveDevicePowerSource()
        let before = BatteryMonitoringCount.system.count
        let obs = a.observe { _ in }
        _ = b.current
        XCTAssertEqual(BatteryMonitoringCount.system.count, before + 1, "make() のたびの実体も同じ数を使う")
        obs.cancel()
        XCTAssertEqual(BatteryMonitoringCount.system.count, before)
    }

    func testReleasingSourceWhileObservedCleansUpWhenObservationEnds() async {
        let device = FakeDevice()
        let center = NotificationCenter()
        weak var weakSource: LiveDevicePowerSource?
        var got = 0
        var observation: DevicePowerObservation?
        do {
            let source = LiveDevicePowerSource(readings: device.readings, center: center)
            weakSource = source
            observation = source.observe { _ in got += 1 }
        }
        XCTAssertNotNil(weakSource, "札が実体を握る(先に手放しても通知と監視が宙に浮かない)")
        XCTAssertTrue(device.monitoring)
        device.charge = .charging
        center.post(name: FakeDevice.changed, object: nil)
        XCTAssertEqual(got, 2, "実体を手放しても見ている間は届く")

        observation = nil // 札を捨てる → main で片付ける
        _ = observation
        for _ in 0..<20 where device.monitoring { await Task.yield() }
        XCTAssertFalse(device.monitoring, "札を捨てたら監視を元に戻す")
        for _ in 0..<20 where weakSource != nil { await Task.yield() }
        XCTAssertNil(weakSource, "片付けた後は実体も手放される")
        device.charge = .full
        center.post(name: FakeDevice.changed, object: nil)
        XCTAssertEqual(got, 2, "通知の登録も残らない")
    }

    func testSystemReadingsDoNotCrash() {
        // シミュレータ・Mac では不明になる(落ちないこと)
        let source = LiveDevicePowerSource()
        let state = source.current
        if let level = state.batteryLevel { XCTAssertTrue((0...1).contains(level)) }
        let obs = source.observe { _ in }
        obs.cancel()
    }

    func testUpdatesStreamYieldsCurrentFirst() async {
        let fake = FakeDevicePowerSource(DevicePowerState(batteryLevel: 0.2, charge: .unplugged, isLowPowerMode: false))
        var iterator = fake.updates().makeAsyncIterator()
        let first = await iterator.next()
        XCTAssertEqual(first?.batteryLevel, 0.2)
        fake.state = DevicePowerState(batteryLevel: 0.2, charge: .charging, isLowPowerMode: false)
        let second = await iterator.next()
        XCTAssertEqual(second?.charge, .charging)
    }

    // MARK: - 偽物

    func testFakeFromEnvironment() {
        let fake = FakeDevicePowerSource(environment: [
            "METAMON_BATTERY_LEVEL": "0.15", "METAMON_BATTERY_STATE": "charging", "METAMON_LOW_POWER": "1",
        ])
        XCTAssertEqual(fake.current, DevicePowerState(batteryLevel: 0.15, charge: .charging, isLowPowerMode: true))
        XCTAssertEqual(FakeDevicePowerSource(environment: [:]).current, .unknown)
        XCTAssertTrue(DevicePowerSources.make(environment: ["METAMON_MOCK": "1"]) is FakeDevicePowerSource)
    }

    func testFakeObserveAndCancel() {
        let fake = FakeDevicePowerSource()
        var got: [DevicePowerState] = []
        let obs = fake.observe { got.append($0) }
        fake.state.isLowPowerMode = true
        obs.cancel()
        fake.state.batteryLevel = 0.1
        XCTAssertEqual(got.count, 2)
        XCTAssertEqual(fake.observerCount, 0)
    }
}
