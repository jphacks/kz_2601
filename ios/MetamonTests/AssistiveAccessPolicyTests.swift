import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// アシスティブアクセス(#1997): 出す・出さないと最初の画面の決まり。
final class AssistiveAccessPolicyTests: XCTestCase {
    private let active = AssistiveAccessPolicy(isActive: true)

    // MARK: - 出す・出さない

    func testStandardShowsEverything() {
        for feature in AssistiveAccessPolicy.Feature.allCases {
            XCTAssertTrue(AssistiveAccessPolicy.standard.shows(feature), "\(feature)")
        }
    }

    func testAssistiveAccessHidesCreateAndPaidFeatures() {
        XCTAssertFalse(active.shows(.aiButton))
        XCTAssertFalse(active.shows(.voiceSheet))
        XCTAssertFalse(active.shows(.appSwitcher))
        XCTAssertFalse(active.shows(.paywall))
        XCTAssertFalse(active.shows(.launchPrompts))
        XCTAssertFalse(active.shows(.aiConsent))
        XCTAssertFalse(active.shows(.shareLinks))
        // 新しい Feature を足したら、隠すかどうかをここで決める
        XCTAssertEqual(AssistiveAccessPolicy.hiddenFeatures, Set(AssistiveAccessPolicy.Feature.allCases))
    }

    // MARK: - AI 送信の同意(同意の画面が無い)

    /// 未同意で AI 部品を押しても同意の画面は立てず、一言だけ出して送らない
    @MainActor
    func testRequireAIConsentShowsToastInsteadOfConsentSheet() throws {
        let rt = try consentRuntime()
        rt.assistiveAccess = active
        XCTAssertFalse(rt.requireAIConsent())
        XCTAssertFalse(rt.showAIConsent)
        XCTAssertEqual(rt.toast, AssistiveAccessPolicy.unavailableMessage)
    }

    @MainActor
    func testRequireAIConsentStillShowsConsentSheetOutsideAssistiveAccess() throws {
        let rt = try consentRuntime()
        XCTAssertFalse(rt.requireAIConsent())
        XCTAssertTrue(rt.showAIConsent)
    }

    @MainActor
    private func consentRuntime() throws -> AppRuntime {
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: BuildConfig.make(environment: .mock))
        rt.aiConsent = AIConsentStore(defaults: UserDefaults(suiteName: "AssistiveAccessPolicyTests.\(UUID().uuidString)")!)
        return rt
    }

    // MARK: - 音声シート・直してと頼む(話しかける画面が無い)

    @MainActor
    func testVoiceSheetCannotOpen() throws {
        let rt = try appRuntime()
        rt.showVoiceSheet = true
        rt.assistiveAccess = active
        XCTAssertFalse(rt.showVoiceSheet, "入った時に開いていたら閉じる")
        rt.showVoiceSheet = true
        XCTAssertFalse(rt.showVoiceSheet, "隠している間は立たない")
    }

    @MainActor
    func testIntentDoesNotCloseScreensAndShowsToast() throws {
        let rt = try appRuntime()
        rt.assistiveAccess = active
        rt.sheet = Route(screen: "detail", stack: "sheet")
        rt.openVoiceFromIntent()
        XCTAssertFalse(rt.showVoiceSheet)
        XCTAssertNotNil(rt.sheet, "開いている画面は閉じない")
        XCTAssertEqual(rt.toast, AssistiveAccessPolicy.unavailableMessage)
    }

    @MainActor
    func testFixRequestIsNotOffered() throws {
        let rt = try appRuntime()
        rt.assistiveAccess = active
        rt.reportFixable("動きませんでした", ctx: nil)
        XCTAssertNil(rt.fixOffer)
        rt.fixOffer = FixOffer(message: "動きませんでした", draft: "直して")
        rt.requestFix()
        XCTAssertFalse(rt.showVoiceSheet)
        XCTAssertEqual(rt.toast, AssistiveAccessPolicy.unavailableMessage)
    }

    #if !APP_STORE_REVIEW
    // MARK: - 課金・コイン

    @MainActor
    func testProRequestsBecomeToast() throws {
        let rt = try appRuntime()
        rt.assistiveAccess = active
        rt.requestProForAIText()
        XCTAssertEqual(rt.toast, AssistiveAccessPolicy.unavailableMessage)
        rt.toast = nil
        rt.presentProRequiredWrite(ProRequired(features: []))
        XCTAssertEqual(rt.toast, AssistiveAccessPolicy.unavailableMessage)
    }

    /// 記録の書き込みの 402(コイン): お店は出さず一言だけ
    @MainActor
    func testCoinDenialOnWriteDoesNotOpenShop() async throws {
        let gate = CoinGate(store: CoinStore(mode: .mock, service: MockCoinService()))
        await gate.store.refreshConfig()
        let presenter = UUID()
        gate.presenterAppeared(presenter)
        XCTAssertTrue(gate.handle(CoinDenial(kind: .coinsInsufficient)))
        XCTAssertNotNil(gate.request(for: presenter, assistiveAccess: .standard))
        XCTAssertNil(gate.request(for: presenter, assistiveAccess: active), "アシスティブアクセスではお店を出さない")

        let rt = try appRuntime()
        let error = APIError.server(status: 402, code: CoinDenial.Kind.coinsInsufficient.rawValue, message: "coins", details: [:])
        XCTAssertNotEqual(rt.message(for: error, entity: nil), AssistiveAccessPolicy.unavailableMessage)
        rt.assistiveAccess = active
        XCTAssertEqual(rt.message(for: error, entity: nil), AssistiveAccessPolicy.unavailableMessage)
    }
    #endif

    @MainActor
    private func appRuntime() throws -> AppRuntime {
        AppRuntime(document: try DSLExamples.document("kakeibo"), services: BuildConfig.make(environment: .mock))
    }

    // MARK: - 最初の画面

    func testStandardHomeIsUnchanged() {
        XCTAssertEqual(AssistiveAccessPolicy.standard.home(openedAppId: nil, currentAppId: "a"), .standard)
        XCTAssertEqual(AssistiveAccessPolicy.standard.home(openedAppId: "a", currentAppId: "a"), .standard)
    }

    func testAssistiveAccessStartsWithAppList() {
        XCTAssertEqual(active.home(openedAppId: nil, currentAppId: "a"), .appList)
    }

    func testOpenedAppShowsCurrentScreen() {
        XCTAssertEqual(active.home(openedAppId: "a", currentAppId: "a"), .openedApp)
    }

    /// 切り替えに失敗して今のアプリが別のまま → 一覧に留まる
    func testOpenedButNotSwitchedStaysOnList() {
        XCTAssertEqual(active.home(openedAppId: "b", currentAppId: "a"), .appList)
    }

    // MARK: - 一覧に出すアプリ

    func testListedAppsDropBlankAndArchivedKeepingOrder() {
        let apps = [
            AppSummary(id: "blank", name: "初期", currentVersion: 1),
            AppSummary(id: "b", name: "家計簿", currentVersion: 3),
            AppSummary(id: "old", name: "古い", currentVersion: 4, status: "archived"),
            AppSummary(id: "a", name: "日記", currentVersion: 2),
        ]
        XCTAssertEqual(AssistiveAccessPolicy.listedApps(apps).map(\.id), ["b", "a"])
    }

    // MARK: - 判定

    func testDetect() {
        XCTAssertFalse(AssistiveAccessPolicy.detect(systemEnabled: false, debugOverride: nil).isActive)
        XCTAssertTrue(AssistiveAccessPolicy.detect(systemEnabled: true, debugOverride: nil).isActive)
        XCTAssertTrue(AssistiveAccessPolicy.detect(systemEnabled: false, debugOverride: "1").isActive)
        XCTAssertFalse(AssistiveAccessPolicy.detect(systemEnabled: false, debugOverride: "0").isActive)
    }
}
