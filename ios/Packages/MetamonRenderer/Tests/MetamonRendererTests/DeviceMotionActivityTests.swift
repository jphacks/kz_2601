import XCTest
@testable import MetamonRenderer

#if !APP_STORE_REVIEW
/// 今の動きを読む口(#2225)。`CMMotionActivity` は自分で作れないので、
/// フラグだけを持つ `DeviceMotionActivityFlags` から種類を決める純関数を確かめる。
final class DeviceMotionActivityTests: XCTestCase {
    private let now = ISO8601DateFormatter().date(from: "2026-11-13T09:30:00+09:00")!

    private func at(_ minutes: Int) -> Date { now.addingTimeInterval(TimeInterval(minutes) * 60) }

    private func flags(stationary: Bool = false, walking: Bool = false, running: Bool = false,
                       automotive: Bool = false, cycling: Bool = false, unknown: Bool = false,
                       confidence: DeviceMotionConfidence = .high, minutes: Int = 0) -> DeviceMotionActivityFlags {
        DeviceMotionActivityFlags(stationary: stationary, walking: walking, running: running,
                                  automotive: automotive, cycling: cycling, unknown: unknown,
                                  confidence: confidence, start: at(minutes))
    }

    // MARK: 1 つだけ立つ

    func testEachFlagMapsToItsOwnKind() {
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(walking: true)), .walking)
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(running: true)), .running)
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(cycling: true)), .cycling)
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(automotive: true)), .automotive)
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(stationary: true)), .stationary)
    }

    func testNothingSetIsUnknown() {
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags()), .unknown)
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(unknown: true)), .unknown)
    }

    /// OS の unknown が立っていても、体の動きが一緒に立っているならそちらを取る。
    func testUnknownFlagDoesNotHideARealActivity() {
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(walking: true, unknown: true)), .walking)
    }

    // MARK: 複数が同時に立つ時の優先

    /// 電車の中で歩くと walking と automotive が両方立つ。体の動きを先に取る。
    func testWalkingWinsOverAutomotive() {
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(walking: true, automotive: true)), .walking)
    }

    /// 信号待ちで automotive と stationary が両方立つ。乗り物を先に取る。
    func testAutomotiveWinsOverStationary() {
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(stationary: true, automotive: true)), .automotive)
    }

    func testRunningWinsOverWalking() {
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(walking: true, running: true)), .running)
    }

    func testCyclingWinsOverRunningAndWalking() {
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(walking: true, running: true, cycling: true)), .cycling)
    }

    func testBodyMotionWinsWhenEveryFlagIsSet() {
        let all = flags(stationary: true, walking: true, running: true, automotive: true, cycling: true)
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: all), .cycling)
    }

    /// 優先の並びどおりに、前の方が必ず勝つ。
    func testPriorityOrderIsTotal() {
        let order = DeviceMotionActivityMapping.priority
        XCTAssertEqual(order, [.cycling, .running, .walking, .automotive, .stationary])
        for (i, strong) in order.enumerated() {
            for weak in order[(i + 1)...] {
                var pair = flags()
                for kind in [strong, weak] {
                    switch kind {
                    case .cycling: pair.cycling = true
                    case .running: pair.running = true
                    case .walking: pair.walking = true
                    case .automotive: pair.automotive = true
                    case .stationary: pair.stationary = true
                    case .unknown: break
                    }
                }
                XCTAssertEqual(DeviceMotionActivityMapping.kind(from: pair), strong,
                               "\(strong) が \(weak) に勝つはず")
            }
        }
    }

    // MARK: 信頼度 low

    /// low は当てにならない(止まっていても walking が立つ)ので DSL には unknown を渡す。
    func testLowConfidenceBecomesUnknown() {
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(walking: true, confidence: .low)), .unknown)
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(automotive: true, confidence: .low)), .unknown)
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(stationary: true, confidence: .low)), .unknown)
    }

    func testMediumAndHighConfidenceAreKept() {
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(walking: true, confidence: .medium)), .walking)
        XCTAssertEqual(DeviceMotionActivityMapping.kind(from: flags(walking: true, confidence: .high)), .walking)
    }

    /// low でも、元のフラグで決めた種類は `rawKind` に残す(実機での確認用)。
    func testLowConfidenceKeepsRawKind() {
        let reading = DeviceMotionActivityMapping.reading(from: flags(walking: true, automotive: true, confidence: .low, minutes: -5))
        XCTAssertEqual(reading.kind, .unknown)
        XCTAssertEqual(reading.rawKind, .walking)
        XCTAssertEqual(reading.confidence, .low)
        XCTAssertEqual(reading.start, at(-5))
    }

    func testConfidenceIsOrdered() {
        XCTAssertTrue(DeviceMotionConfidence.low < .medium)
        XCTAssertTrue(DeviceMotionConfidence.medium < .high)
        XCTAssertEqual(DeviceMotionConfidence.allCases.sorted(), [.low, .medium, .high])
    }

    // MARK: 期間と件数の上限

    func testClampKeepsRangeInsideTheLastSevenDays() {
        let range = DeviceMotionActivityLimits.clamp(from: now.addingTimeInterval(-60 * 86_400),
                                                     to: now.addingTimeInterval(86_400), now: now)
        let lower = Calendar.current.date(byAdding: .day, value: -DeviceMotionActivityLimits.daysBack, to: now)!
        XCTAssertEqual(range?.start, lower)
        XCTAssertEqual(range?.end, now)
    }

    func testClampReturnsNilForAnEmptyRange() {
        XCTAssertNil(DeviceMotionActivityLimits.clamp(from: now, to: now, now: now))
        XCTAssertNil(DeviceMotionActivityLimits.clamp(from: at(10), to: at(20), now: now))
    }

    func testHistorySortsByStartAndCutsAtTheLimit() {
        let readings = (0..<(DeviceMotionActivityLimits.maxItems + 50)).reversed().map {
            DeviceMotionActivityMapping.reading(from: flags(walking: true, minutes: -$0 - 1))
        }
        let range = DateInterval(start: at(-10_000), end: now)
        let cut = DeviceMotionActivityMapping.history(readings, in: range)
        XCTAssertEqual(cut.count, DeviceMotionActivityLimits.maxItems)
        XCTAssertEqual(cut, cut.sorted { $0.start < $1.start })
        XCTAssertEqual(cut.first?.start, at(-(DeviceMotionActivityLimits.maxItems + 50)))
    }

    func testHistoryDropsReadingsOutsideTheRange() {
        let readings = [-30, -10, 10].map { DeviceMotionActivityMapping.reading(from: flags(walking: true, minutes: $0)) }
        let cut = DeviceMotionActivityMapping.history(readings, in: DateInterval(start: at(-20), end: now))
        XCTAssertEqual(cut.map(\.start), [at(-10)])
    }

    // MARK: 偽物

    private func fake(access: DeviceMotionActivityAccess = .authorized,
                      isAvailable: Bool = true,
                      all: [DeviceMotionActivityFlags] = []) -> FakeDeviceMotionActivity {
        let now = self.now
        return FakeDeviceMotionActivity(isAvailable: isAvailable, access: access, all: all, now: { now })
    }

    func testFakeReturnsTheNewestReadingAsCurrent() async throws {
        let source = fake(all: [flags(stationary: true, minutes: -30), flags(walking: true, minutes: -1)])
        let reading = try await source.current()
        XCTAssertEqual(reading?.kind, .walking)
        XCTAssertEqual(reading?.start, at(-1))
    }

    func testFakeWithoutPermissionThrows() async {
        do {
            _ = try await fake(access: .denied).current()
            XCTFail("許可が無ければ投げるはず")
        } catch {
            XCTAssertEqual(error as? DeviceMotionActivityError, .notAuthorized(.denied))
        }
    }

    func testFakeWithoutTheSensorThrows() async {
        do {
            _ = try await fake(isAvailable: false).current()
            XCTFail("端末に無ければ投げるはず")
        } catch {
            XCTAssertEqual(error as? DeviceMotionActivityError, .unavailable)
        }
    }

    func testFakeAsksOnlyWhenUndecided() async {
        let undecided = fake(access: .notDetermined)
        let asked = await undecided.authorization(requesting: true)
        XCTAssertEqual(asked, .authorized)
        let notAsked = await undecided.authorization(requesting: false)
        XCTAssertEqual(notAsked, .notDetermined)
        let denied = await fake(access: .denied).authorization(requesting: true)
        XCTAssertEqual(denied, .denied)
    }

    func testFakeHistoryUsesTheMappingAndTheRange() async throws {
        let source = fake(all: [flags(walking: true, minutes: -60 * 24 * 30),
                                flags(walking: true, automotive: true, minutes: -120),
                                flags(stationary: true, confidence: .low, minutes: -30)])
        let readings = try await source.history(from: now.addingTimeInterval(-86_400 * 60), to: now)
        XCTAssertEqual(readings.map(\.kind), [.walking, .unknown])
        XCTAssertEqual(readings.map(\.rawKind), [.walking, .stationary])
    }

    func testFakeUpdatesStopWhenNotAllowed() async {
        var seen: [DeviceMotionActivityKind] = []
        for await reading in fake(all: [flags(walking: true, minutes: -2)]).updates() { seen.append(reading.kind) }
        XCTAssertEqual(seen, [.walking])

        var blocked: [DeviceMotionActivityKind] = []
        for await reading in fake(access: .denied, all: [flags(walking: true)]).updates() { blocked.append(reading.kind) }
        XCTAssertTrue(blocked.isEmpty)
    }

    #if os(iOS)
    func testAuthorizationStatusMapsEveryCase() {
        XCTAssertEqual(DeviceMotionActivityAccess(.notDetermined), .notDetermined)
        XCTAssertEqual(DeviceMotionActivityAccess(.denied), .denied)
        XCTAssertEqual(DeviceMotionActivityAccess(.restricted), .restricted)
        XCTAssertEqual(DeviceMotionActivityAccess(.authorized), .authorized)
        XCTAssertTrue(DeviceMotionActivityAccess.authorized.canRead)
        XCTAssertFalse(DeviceMotionActivityAccess.notDetermined.canRead)
    }

    func testUsageDescriptionMustNotBeEmpty() {
        XCTAssertFalse(LiveDeviceMotionActivity.hasUsageDescription(in: nil))
        XCTAssertFalse(LiveDeviceMotionActivity.hasUsageDescription(in: [:]))
        XCTAssertFalse(LiveDeviceMotionActivity.hasUsageDescription(in: ["NSMotionUsageDescription": ""]))
        XCTAssertTrue(LiveDeviceMotionActivity.hasUsageDescription(in: ["NSMotionUsageDescription": "歩数を読みます"]))
    }
    #endif
}
#endif
