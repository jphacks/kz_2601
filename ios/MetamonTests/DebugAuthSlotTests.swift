#if METAMON_STG
import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #2084: 開発用メニューのアカウントの枠(保存・Firebase のアプリの選び方・切り替えで端末のデータを消す)。
@MainActor
final class DebugAuthSlotTests: XCTestCase {
    private var suiteName = ""
    private var defaults: UserDefaults!

    override func setUp() {
        suiteName = "DebugAuthSlotTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suiteName)
        defaults = nil
    }

    func testDefaultAndUnknownValuesAreSlotZero() {
        XCTAssertEqual(DebugAuthSlot.current(in: defaults), 0)
        for raw in ["", "x", "-1", "4", "99", "1.5"] {
            XCTAssertEqual(DebugAuthSlot.slot(from: raw), 0, raw)
        }
        XCTAssertEqual(DebugAuthSlot.slot(from: nil), 0)
        XCTAssertEqual(DebugAuthSlot.slot(from: "3"), 3)
    }

    func testSaveRoundTripsAndSlotZeroRemovesKey() {
        DebugAuthSlot.save(2, in: defaults)
        XCTAssertEqual(DebugAuthSlot.current(in: defaults), 2)
        DebugAuthSlot.save(0, in: defaults)
        XCTAssertNil(defaults.object(forKey: DebugAuthSlot.key))
        XCTAssertEqual(DebugAuthSlot.current(in: defaults), 0)
    }

    /// 「全部元に戻す」で枠が消えない(データを消さずに別の uid に戻らないように)
    func testResetAllKeepsSlot() {
        DebugAuthSlot.save(3, in: defaults)
        DebugAuthSlot.markUsed(3, at: Date(timeIntervalSince1970: 100), in: defaults)
        DebugKnobStore(defaults: defaults, environment: { [:] }).resetAll()
        XCTAssertEqual(DebugAuthSlot.current(in: defaults), 3)
        XCTAssertNotNil(DebugAuthSlot.lastUsed(3, in: defaults))
        XCTAssertFalse(DebugAuthSlot.key.hasPrefix(DebugKnobs.keyPrefix))
    }

    func testAppNamePerSlot() {
        XCTAssertNil(DebugAuthSlot.appName(for: 0), "枠 0 は既定のアプリ")
        XCTAssertEqual(DebugAuthSlot.appName(for: 1), "metamon-slot-1")
        XCTAssertEqual(DebugAuthSlot.appName(for: 3), "metamon-slot-3")
        XCTAssertNil(DebugAuthSlot.appName(for: 4))
        XCTAssertNil(DebugAuthSlot.appName(for: 9))
        let names = DebugAuthSlot.slots.compactMap(DebugAuthSlot.appName(for:))
        XCTAssertEqual(Set(names).count, 3, "試験用の枠はそれぞれ別の名前")
    }

    /// BuildConfig の入口は保存した枠のアプリ名を使う
    func testBuildConfigUsesSavedSlot() {
        let saved = DebugKnobs.store.defaults.object(forKey: DebugAuthSlot.key)
        defer { DebugKnobs.store.defaults.set(saved, forKey: DebugAuthSlot.key) }
        DebugAuthSlot.save(0, in: DebugKnobs.store.defaults)
        XCTAssertNil(BuildConfig.authAppName)
        XCTAssertFalse(BuildConfig.isDebugTestAccount)
        DebugAuthSlot.save(3, in: DebugKnobs.store.defaults)
        XCTAssertEqual(BuildConfig.authAppName, "metamon-slot-3")
        XCTAssertTrue(BuildConfig.isDebugTestAccount, "試験用の枠は Apple への引き継ぎと削除を出さない")
    }

    /// Firebase がある版では、名前付きのアプリの Auth を使う(ネットワークには出ない)
    func testFirebaseAuthServiceUsesNamedApp() throws {
        try XCTSkipUnless(FirebaseBootstrap.isConfigured, "GoogleService-Info.plist が無い版")
        XCTAssertEqual(FirebaseAuthService()?.firebaseAppName, "__FIRAPP_DEFAULT")
        XCTAssertEqual(FirebaseAuthService(appName: nil)?.firebaseAppName, "__FIRAPP_DEFAULT")
        XCTAssertEqual(FirebaseAuthService(appName: "metamon-slot-2")?.firebaseAppName, "metamon-slot-2")
        XCTAssertEqual(FirebaseAuthService(appName: "metamon-slot-2")?.firebaseAppName, "metamon-slot-2", "2 回目は作ったものを使う")
    }

    func testLastUsedPerSlot() {
        XCTAssertNil(DebugAuthSlot.lastUsed(1, in: defaults))
        DebugAuthSlot.markUsed(1, at: Date(timeIntervalSince1970: 1_000), in: defaults)
        XCTAssertEqual(DebugAuthSlot.lastUsed(1, in: defaults), Date(timeIntervalSince1970: 1_000))
        XCTAssertNil(DebugAuthSlot.lastUsed(2, in: defaults), "枠ごとに別")
    }

    /// 試験用の枠では引き継ぎの案内を予約しない
    func testTestSlotDoesNotOfferAccountLink() throws {
        let saved = DebugKnobs.store.defaults.object(forKey: DebugAuthSlot.key)
        defer { DebugKnobs.store.defaults.set(saved, forKey: DebugAuthSlot.key) }
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: BuildConfig.make(environment: .mock))
        rt.isAnonymousUser = { true }
        DebugAuthSlot.save(2, in: DebugKnobs.store.defaults)
        rt.offerAccountLinkIfNeeded(defaults: defaults, launch: "a")
        XCTAssertNil(defaults.string(forKey: AccountLinkPrompt.pendingKey))
        DebugAuthSlot.save(0, in: DebugKnobs.store.defaults)
        rt.offerAccountLinkIfNeeded(defaults: defaults, launch: "a")
        XCTAssertEqual(defaults.string(forKey: AccountLinkPrompt.pendingKey), "a", "枠 0 は今までどおり")
    }

    func testSwitchWipesThenSavesThenRelaunches() async {
        var steps: [String] = []
        await DebugAuthSlot.switchTo(2, defaults: defaults,
                                     wipe: { steps.append("wipe:\(DebugAuthSlot.current(in: self.defaults))") },
                                     relaunch: { steps.append("relaunch:\(DebugAuthSlot.current(in: self.defaults))") })
        XCTAssertEqual(steps, ["wipe:0", "relaunch:2"])
    }

    /// 切り替えはログアウトと同じ後片付けで端末のデータを消し、サインインは残す
    func testSwitchRunsTheSignOutWipeAndKeepsSignIn() async throws {
        let d = UserDefaults.standard
        let saved = LocalWipe.preservedKeys.map { ($0, d.object(forKey: $0)) }
        defer { for (key, value) in saved { d.set(value, forKey: key) } }
        let services = BuildConfig.make(environment: .mock)
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: services)
        rt.apps = [AppSummary(id: "prev-app", name: "前の枠のアプリ", currentVersion: 1)]
        let oldAppId = rt.document.appId
        let session = AuthSession(service: services.auth)
        XCTAssertNotNil(session.user)
        var relaunched = false

        await DebugAuthSlot.switchTo(1, defaults: defaults, wipe: { await rt.wipeBeforeAccountChange() }, relaunch: { relaunched = true })

        XCTAssertTrue(rt.apps.isEmpty)
        XCTAssertNotEqual(rt.document.appId, oldAppId)
        XCTAssertTrue(rt.dsl.isVoiceOnly)
        XCTAssertNotNil(services.auth.currentUser, "今の枠のサインインは残す")
        XCTAssertEqual(DebugAuthSlot.current(in: defaults), 1)
        XCTAssertTrue(relaunched)
    }
}
#endif
