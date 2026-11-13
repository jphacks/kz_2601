import XCTest
#if canImport(Intents)
import Intents
#endif
@testable import MetamonServices

/// 集中モード中か(#1990)。OS の値は `FocusStatusReadings` で差し替える。
@MainActor
final class FocusStatusSourceTests: XCTestCase {
    private final class FakeOS: @unchecked Sendable {
        var auth: FocusAuthorization = .notDetermined
        var grant: FocusAuthorization = .authorized
        var focused: Bool? = true
        var focusReads = 0
        var requests = 0

        @MainActor
        var readings: FocusStatusReadings {
            FocusStatusReadings(
                authorization: { self.auth },
                requestAuthorization: { done in
                    self.requests += 1
                    self.auth = self.grant
                    DispatchQueue.global().async { done(self.grant) }
                },
                isFocused: { self.focusReads += 1; return self.focused },
                notificationNames: [Self.active]
            )
        }

        static let active = Notification.Name("test.focus.active")
    }

    // MARK: - 変換と nil

    #if canImport(Intents)
    func testAuthorizationConversion() {
        XCTAssertEqual(FocusAuthorization(INFocusStatusAuthorizationStatus.notDetermined), .notDetermined)
        XCTAssertEqual(FocusAuthorization(INFocusStatusAuthorizationStatus.denied), .denied)
        XCTAssertEqual(FocusAuthorization(INFocusStatusAuthorizationStatus.restricted), .restricted)
        XCTAssertEqual(FocusAuthorization(INFocusStatusAuthorizationStatus.authorized), .authorized)
    }
    #endif

    func testIsFocusedNilUnlessAuthorized() {
        for auth in FocusAuthorization.allCases where auth != .authorized {
            XCTAssertNil(FocusStatus(authorization: auth, isFocused: true).isFocused, "\(auth)")
            XCTAssertNil(FocusStatus.read(authorization: auth, isFocused: { XCTFail("読まない"); return true }).isFocused)
        }
        XCTAssertEqual(FocusStatus(authorization: .authorized, isFocused: true).isFocused, true)
        XCTAssertEqual(FocusStatus(authorization: .authorized, isFocused: false).isFocused, false)
        // 許可があっても OS が分からなければ nil
        XCTAssertNil(FocusStatus(authorization: .authorized, isFocused: nil).isFocused)
    }

    // MARK: - 実体(差し替えた OS)

    func testLiveDoesNotReadFocusWithoutPermission() {
        let os = FakeOS()
        let source = LiveFocusStatusSource(readings: os.readings, center: NotificationCenter())
        XCTAssertEqual(source.current, FocusStatus(authorization: .notDetermined, isFocused: nil))
        XCTAssertNil(source.isFocused)
        XCTAssertEqual(os.focusReads, 0)
    }

    func testLiveRequestOnlyWhenAskedAndUndetermined() async {
        let os = FakeOS()
        let source = LiveFocusStatusSource(readings: os.readings, center: NotificationCenter())
        let notAsked = await source.authorization(requesting: false)
        XCTAssertEqual(notAsked, .notDetermined)
        XCTAssertEqual(os.requests, 0)

        let asked = await source.authorization(requesting: true)
        XCTAssertEqual(asked, .authorized)
        XCTAssertEqual(os.requests, 1)
        XCTAssertEqual(source.isFocused, true)

        // 決まった後は求め直さない
        _ = await source.authorization(requesting: true)
        XCTAssertEqual(os.requests, 1)
    }

    /// entitlement が無いビルドは restricted が返る想定(未確認・実機では確かめていない)。restricted なら落ちずに nil
    func testLiveRestrictedWithoutEntitlementAssumedUnverified() async {
        let os = FakeOS()
        os.grant = .restricted
        let source = LiveFocusStatusSource(readings: os.readings, center: NotificationCenter())
        let result = await source.authorization(requesting: true)
        XCTAssertEqual(result, .restricted)
        XCTAssertNil(source.isFocused)
    }

    func testLiveRereadsOnForegroundAndNotifiesOnlyOnChange() {
        let os = FakeOS()
        os.auth = .authorized
        os.focused = false
        let center = NotificationCenter()
        let source = LiveFocusStatusSource(readings: os.readings, center: center)
        var seen: [Bool?] = []
        let observation = source.observe { seen.append($0.isFocused) }
        XCTAssertEqual(seen, [false])

        center.post(name: FakeOS.active, object: nil)
        XCTAssertEqual(seen, [false], "変わっていなければ知らせない")

        os.focused = true
        center.post(name: FakeOS.active, object: nil)
        XCTAssertEqual(seen, [false, true])

        observation.cancel()
        XCTAssertFalse(source.isObserving)
        os.focused = false
        center.post(name: FakeOS.active, object: nil)
        XCTAssertEqual(seen, [false, true])
    }

    /// 後から見始めた時、前から見ている側も新しい値を受け取る(取り残されない)
    func testLiveNewObserverRefreshesExistingOnes() {
        let os = FakeOS()
        os.auth = .authorized
        os.focused = false
        let source = LiveFocusStatusSource(readings: os.readings, center: NotificationCenter())
        var first: [Bool?] = []
        var second: [Bool?] = []
        let a = source.observe { first.append($0.isFocused) }
        os.focused = true
        let b = source.observe { second.append($0.isFocused) }
        XCTAssertEqual(first, [false, true])
        XCTAssertEqual(second, [true])
        a.cancel()
        b.cancel()
    }

    // MARK: - 偽物

    func testFakeRequestAndNotify() async {
        let fake = FakeFocusStatusSource(authorization: .notDetermined, focused: true, grantOnRequest: .denied)
        var seen: [FocusStatus] = []
        let observation = fake.observe { seen.append($0) }
        XCTAssertNil(fake.isFocused)

        let result = await fake.authorization(requesting: true)
        XCTAssertEqual(result, .denied)
        XCTAssertEqual(fake.requestCount, 1)
        XCTAssertNil(fake.isFocused)

        fake.authorizationStatus = .authorized
        XCTAssertEqual(fake.isFocused, true)
        fake.focused = false
        // 許可の状態が変わった時も知らせる(denied でも isFocused は nil のまま)
        XCTAssertEqual(seen.map(\.authorization), [.notDetermined, .denied, .authorized, .authorized])
        XCTAssertEqual(seen.map(\.isFocused), [nil, nil, true, false])
        fake.focused = false
        XCTAssertEqual(seen.count, 4, "同じ値なら知らせない")
        observation.cancel()
        XCTAssertEqual(fake.observerCount, 0)
    }

    func testFakeFromEnvironmentAndFactory() {
        let fake = FakeFocusStatusSource(environment: ["METAMON_FOCUS_AUTH": "authorized", "METAMON_FOCUSED": "1"])
        XCTAssertEqual(fake.current, FocusStatus(authorization: .authorized, isFocused: true))
        let denied = FakeFocusStatusSource(environment: ["METAMON_FOCUS_AUTH": "denied"])
        XCTAssertEqual(denied.current, FocusStatus(authorization: .denied, isFocused: nil))

        XCTAssertTrue(FocusStatusSources.make(reviewBuild: false, environment: ["METAMON_MOCK": "1"]) is FakeFocusStatusSource)
        XCTAssertTrue(FocusStatusSources.make(reviewBuild: false, environment: [:]) is LiveFocusStatusSource)
    }

    /// 審査用の版は説明文が消えるので、OS の許可を求めず常に restricted / nil
    func testReviewBuildFactoryIsRestrictedAndNeverRequests() async {
        let source = FocusStatusSources.make(reviewBuild: true, environment: [:])
        XCTAssertFalse(source is LiveFocusStatusSource)
        XCTAssertEqual(source.current, .restricted)
        let result = await source.authorization(requesting: true)
        XCTAssertEqual(result, .restricted)
        XCTAssertEqual((source as? FakeFocusStatusSource)?.requestCount, 0)
        XCTAssertNil(source.isFocused)
    }

    #if !os(iOS)
    /// Mac は対象外: 常に restricted / nil
    func testMacSystemReadingsAreRestricted() async {
        let source = LiveFocusStatusSource()
        XCTAssertEqual(source.current, .restricted)
        let result = await source.authorization(requesting: true)
        XCTAssertEqual(result, .restricted)
    }
    #endif
}
