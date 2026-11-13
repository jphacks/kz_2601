import XCTest
import IOKit.ps
@testable import MetamonServices

/// #2268: Mac の電池・充電中を Mac のホストでも確かめる(本体のケースは package の DevicePowerSourceMac2268Tests)。
final class MacPowerSource2268Tests: XCTestCase {
    func testLaptopReadings() {
        let onBattery: [String: Any] = [
            kIOPSTypeKey: kIOPSInternalBatteryType, kIOPSCurrentCapacityKey: 64, kIOPSMaxCapacityKey: 100,
            kIOPSPowerSourceStateKey: kIOPSBatteryPowerValue, kIOPSIsChargingKey: false,
        ]
        let r = MacPowerSources.reading(from: [onBattery])
        XCTAssertEqual(r.rawLevel, 0.64, accuracy: 0.0001)
        XCTAssertEqual(r.charge, .unplugged)

        var charging = onBattery
        charging[kIOPSPowerSourceStateKey] = kIOPSACPowerValue
        charging[kIOPSIsChargingKey] = true
        XCTAssertEqual(MacPowerSources.reading(from: [charging]).charge, .charging)
    }

    func testDesktopReportsUnknown() {
        let none = MacPowerSources.reading(from: [])
        XCTAssertEqual(DevicePowerState.from(rawLevel: none.rawLevel, charge: none.charge, isLowPowerMode: false), .unknown)
    }

    @MainActor
    func testThisMacDoesNotCrashAndStaysInRange() {
        let source = LiveDevicePowerSource()
        let obs = source.observe { _ in }
        XCTAssertTrue(MacPowerSources.notifier.isEnabled)
        obs.cancel()
        if let level = source.current.batteryLevel { XCTAssertTrue((0...1).contains(level)) }
    }
}
