import XCTest
import CoreBluetooth
@testable import MetamonRenderer

/// Bluetooth の心拍ベルト(#2237): 測定値(0x2A37)のバイト列の読み取りと、OS の Bluetooth の状態の写し。
final class BLEHeartRate2237Tests: XCTestCase {
    func testSpecExample8BitWithContactAndRR() {
        // flags 0x16 = 接触の検出に対応 + 接触あり + RR 間隔あり / 72 拍 / RR 1024 (=1.0 秒), 512 (=0.5 秒)
        let m = HeartRateMeasurement(Data([0x16, 72, 0x00, 0x04, 0x00, 0x02]))
        XCTAssertEqual(m, HeartRateMeasurement(beatsPerMinute: 72, contact: .detected, rrIntervals: [1.0, 0.5]))
    }

    func test16BitValueAndEnergy() {
        // flags 0x09 = 16bit + 消費エネルギー / 300 拍(0x012C)/ 1000 kJ(0x03E8)
        let m = HeartRateMeasurement(Data([0x09, 0x2C, 0x01, 0xE8, 0x03]))
        XCTAssertEqual(m?.beatsPerMinute, 300)
        XCTAssertEqual(m?.energyExpended, 1000)
        XCTAssertEqual(m?.contact, .unsupported)
        XCTAssertEqual(m?.rrIntervals, [])
    }

    func testContactNotDetected() {
        XCTAssertEqual(HeartRateMeasurement(Data([0x04, 60]))?.contact, .notDetected)
        XCTAssertEqual(HeartRateMeasurement(Data([0x06, 60]))?.contact, .detected)
        XCTAssertEqual(HeartRateMeasurement(Data([0x02, 60]))?.contact, .unsupported) // 対応ビットが無ければ接触ビットは見ない
    }

    func testTruncatedDataIsRejected() {
        XCTAssertNil(HeartRateMeasurement(Data()))
        XCTAssertNil(HeartRateMeasurement(Data([0x00])))
        XCTAssertNil(HeartRateMeasurement(Data([0x01, 0x48]))) // 16bit なのに 1 バイト
        XCTAssertNil(HeartRateMeasurement(Data([0x08, 70, 0x01]))) // 消費エネルギーが欠けている
    }

    func testOddTrailingRRByteIsIgnored() {
        let m = HeartRateMeasurement(Data([0x10, 80, 0x00, 0x04, 0x7F]))
        XCTAssertEqual(m?.rrIntervals, [1.0])
    }

    func testDataSliceWithNonZeroStartIndex() {
        let whole = Data([0xFF, 0xFF, 0x00, 65])
        XCTAssertEqual(HeartRateMeasurement(whole.dropFirst(2))?.beatsPerMinute, 65)
    }

    func testManagerStateMapping() {
        XCTAssertNil(BLEHeartRate.state(for: .poweredOn))
        XCTAssertEqual(BLEHeartRate.state(for: .poweredOff), .poweredOff)
        XCTAssertEqual(BLEHeartRate.state(for: .unauthorized), .unauthorized)
        XCTAssertEqual(BLEHeartRate.state(for: .unsupported), .unavailable)
        XCTAssertEqual(BLEHeartRate.state(for: .unknown), .idle)
    }

    @MainActor
    func testWithoutUsageDescriptionItIsUnavailableAndDoesNotTouchBluetooth() async {
        // テストのバンドルには説明文が無い: CoreBluetooth に触らず「使えません」で終わる
        let reader = BLEHeartRate(bundle: Bundle(for: Self.self))
        var events: [BLEHeartRate.Event] = []
        for await e in reader.start() { events.append(e) }
        XCTAssertEqual(events, [.state(.unavailable)])
    }
}
