import XCTest
import MetamonServices
@testable import Metamon

/// #296: 設定の「クラッシュ報告を送る」の保存と、送り先(Crashlytics)への反映。
@MainActor
final class CrashReportingTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        suite = "crash296-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    func testDefaultIsOnAndAppliedAtLaunch() {
        let pref = CrashReportingPreference(defaults: defaults)
        XCTAssertTrue(pref.isEnabled, "ポリシー 1.8 のとおり既定はオン")
        let spy = SpyCrashReporter()
        pref.activate(reporter: spy)
        XCTAssertEqual(spy.enabled, [true])
    }

    func testTurningOffIsPersistedAndClearsUserID() {
        let pref = CrashReportingPreference(defaults: defaults)
        let spy = SpyCrashReporter()
        pref.activate(reporter: spy)
        pref.updateUser(uid: "uid-1")
        XCTAssertEqual(spy.userIDs.last, "uid-1")

        pref.setEnabled(false)
        XCTAssertEqual(spy.enabled.last, false)
        XCTAssertEqual(spy.userIDs.last, .some(nil), "オフにしたら uid も外す")
        XCTAssertFalse(CrashReportingPreference(defaults: defaults).isEnabled, "再起動しても残る")

        // オフの間にサインインし直しても uid は渡さない
        pref.updateUser(uid: "uid-2")
        XCTAssertEqual(spy.userIDs.last, .some(nil))

        // オンに戻すと、今の uid を付け直す
        pref.setEnabled(true)
        XCTAssertEqual(spy.enabled.last, true)
        XCTAssertEqual(spy.userIDs.last, "uid-2")
    }

    func testOffIsAppliedOnNextLaunch() {
        CrashReportingPreference(defaults: defaults).setEnabled(false)
        let spy = SpyCrashReporter()
        CrashReportingPreference(defaults: defaults).activate(reporter: spy)
        XCTAssertEqual(spy.enabled, [false])
        XCTAssertEqual(spy.userIDs, [nil])
    }

    func testPrivacyPolicyMentionsCrashReporting() throws {
        let doc = try XCTUnwrap(LegalDocument.privacyPolicy.bundledText)
        XCTAssertTrue(doc.contains("Firebase Crashlytics"))
        XCTAssertTrue(doc.contains("クラッシュ報告を送る"), "設定の項目名とポリシーの記述を揃える")
    }
}

private final class SpyCrashReporter: CrashReporter, @unchecked Sendable {
    var enabled: [Bool] = []
    var userIDs: [String?] = []
    func setCollectionEnabled(_ enabled: Bool) { self.enabled.append(enabled) }
    func setUserID(_ uid: String?) { userIDs.append(uid) }
}
