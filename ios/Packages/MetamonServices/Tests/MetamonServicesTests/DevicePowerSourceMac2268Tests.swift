#if os(macOS)
import XCTest
import IOKit.ps
@testable import MetamonServices

/// #2268: Mac の電池・充電中(IOKit の電源の情報・ノートの Mac だけ・デスクトップは不明)。
final class DevicePowerSourceMac2268Tests: XCTestCase {
    private func battery(_ current: Int, _ max: Int = 100, ac: Bool, charging: Bool = false, charged: Bool? = nil,
                         present: Bool = true) -> [String: Any] {
        var d: [String: Any] = [
            kIOPSTypeKey: kIOPSInternalBatteryType,
            kIOPSIsPresentKey: present,
            kIOPSCurrentCapacityKey: current,
            kIOPSMaxCapacityKey: max,
            kIOPSPowerSourceStateKey: ac ? kIOPSACPowerValue : kIOPSBatteryPowerValue,
            kIOPSIsChargingKey: charging,
        ]
        if let charged { d[kIOPSIsChargedKey] = charged }
        return d
    }

    func testLaptopOnBattery() {
        let r = MacPowerSources.reading(from: [battery(42, ac: false)])
        XCTAssertEqual(r.rawLevel, 0.42, accuracy: 0.0001)
        XCTAssertEqual(r.charge, .unplugged)
        let state = DevicePowerState.from(rawLevel: r.rawLevel, charge: r.charge, isLowPowerMode: true)
        XCTAssertEqual(state.batteryLevel ?? 0, 0.42, accuracy: 0.0001)
        XCTAssertFalse(state.isCharging)
        XCTAssertTrue(state.isLowPowerMode)
    }

    func testLaptopChargingAndFull() {
        XCTAssertEqual(MacPowerSources.reading(from: [battery(30, ac: true, charging: true)]).charge, .charging)
        XCTAssertEqual(MacPowerSources.reading(from: [battery(100, ac: true, charged: true)]).charge, .full)
        XCTAssertEqual(MacPowerSources.reading(from: [battery(100, ac: true)]).charge, .full)
        // 最適化充電で 80% で止めている: つながっている扱い
        XCTAssertEqual(MacPowerSources.reading(from: [battery(80, ac: true)]).charge, .charging)
        // 容量が割合でない値でも割合にする
        XCTAssertEqual(MacPowerSources.reading(from: [battery(2500, 5000, ac: false)]).rawLevel, 0.5, accuracy: 0.0001)
    }

    func testDesktopWithoutBatteryIsUnknown() {
        let none = MacPowerSources.reading(from: [])
        XCTAssertEqual(DevicePowerState.from(rawLevel: none.rawLevel, charge: none.charge, isLowPowerMode: false), .unknown)
        // UPS だけつながっている
        let ups: [String: Any] = [kIOPSTypeKey: kIOPSUPSType, kIOPSCurrentCapacityKey: 90, kIOPSMaxCapacityKey: 100]
        XCTAssertEqual(MacPowerSources.reading(from: [ups]).charge, .unknown)
        // 電池が外れている・容量が読めない
        XCTAssertEqual(MacPowerSources.reading(from: [battery(50, ac: false, present: false)]).charge, .unknown)
        XCTAssertEqual(MacPowerSources.reading(from: [battery(50, 0, ac: false)]).rawLevel, -1)
    }

    @MainActor
    func testNotifierInstallsOnlyWhileObserved() {
        let source = LiveDevicePowerSource()
        let before = MacPowerSources.notifier.isEnabled
        let obs = source.observe { _ in }
        XCTAssertTrue(MacPowerSources.notifier.isEnabled, "見ている間は電源の変化の通知を張る")
        obs.cancel()
        XCTAssertEqual(MacPowerSources.notifier.isEnabled, before)
        // この Mac の実際の値(ノートなら 0〜1・デスクトップなら nil)
        if let level = source.current.batteryLevel { XCTAssertTrue((0...1).contains(level)) }
    }
}
#endif
