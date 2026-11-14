import XCTest
import MetamonDSL
import MetamonServices
@testable import MetamonRenderer
@testable import Metamon

/// iOS の品質の見直し 3 回目: アカウント削除の後に古い状態が残らない・端末の設定は残る。
@MainActor
final class QualitySweep3Tests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        suite = "sweep3-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    // MARK: - 端末のデータ削除(UserDefaults)

    func testWipeResetsConsentInMemorySoConsentScreenComesBack() {
        let consent = AIConsentStore(defaults: defaults)
        let tutorial = FirstRunTutorialStore(defaults: defaults)
        consent.grant()
        consent.markPrompted()
        tutorial.markTutorialSeen()

        LocalWipe.clearDefaults(defaults, domain: suite)
        XCTAssertFalse(AIConsentStore.isGranted(in: defaults), "送信の砦は同意なしと見る")
        consent.reload()
        tutorial.reload()

        XCTAssertFalse(consent.isGranted, "画面も同意なしに合わせる(合わないと同意の画面が出ずに送れないまま)")
        XCTAssertNil(consent.grantedAt)
        XCTAssertFalse(consent.hasBeenPrompted)
        XCTAssertFalse(tutorial.hasSeenTutorial)

        let rt = AppRuntime(document: AppDocument(appId: "sweep3", version: 0, dsl: AppDSL.fallback),
                            services: BuildConfig.make(environment: .mock))
        rt.aiConsent = consent
        XCTAssertFalse(rt.requireAIConsent())
        XCTAssertTrue(rt.showAIConsent, "生成しようとすると同意の画面が出る")
    }

    func testWipeKeepsLanguageAndOptOuts() {
        defaults.set(AppLanguage.en.rawValue, forKey: "metamon.appLanguage")
        CrashReportingPreference(defaults: defaults).setEnabled(false)
        defaults.set(false, forKey: "metamon.aiButtonMetrics.enabled")
        defaults.set("x", forKey: "metamon.someUserState")

        LocalWipe.clearDefaults(defaults, domain: suite)

        XCTAssertEqual(LanguagePreference(defaults: defaults).setting, .en)
        XCTAssertFalse(CrashReportingPreference(defaults: defaults).isEnabled, "オプトアウトが黙ってオンに戻らない")
        XCTAssertEqual(defaults.object(forKey: "metamon.aiButtonMetrics.enabled") as? Bool, false)
        XCTAssertNil(defaults.object(forKey: "metamon.someUserState"))
    }
}
