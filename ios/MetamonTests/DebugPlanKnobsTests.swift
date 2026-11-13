#if METAMON_STG
import XCTest
import MetamonServices
@testable import Metamon

/// #2080: 開発用メニューの「課金と量」のつまみが、ProGate・CoinGate・使っている量の判定に効く。
/// つまみが無い時(Release と同じ条件)は今までどおり。
@MainActor
final class DebugPlanKnobsTests: XCTestCase {
    private var suiteName = ""
    private var defaults: UserDefaults!
    private final class EnvBox: @unchecked Sendable { var values: [String: String] = [:] }
    private var envBox = EnvBox()
    private var env: [String: String] {
        get { envBox.values }
        set { envBox.values = newValue }
    }
    private var saved: DebugKnobStore!
    private var usageDefaults: UserDefaults!
    private var usageSuite = ""

    override func setUp() async throws {
        suiteName = "DebugPlanKnobsTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)
        usageSuite = suiteName + ".usage"
        usageDefaults = UserDefaults(suiteName: usageSuite)
        envBox = EnvBox()
        saved = DebugKnobs.store
        let box = envBox
        DebugKnobs.store = DebugKnobStore(defaults: defaults, environment: { box.values })
    }

    override func tearDown() async throws {
        DebugKnobs.store = saved
        defaults.removePersistentDomain(forName: suiteName)
        usageDefaults.removePersistentDomain(forName: usageSuite)
    }

    private func gate(limit: Int = 3, limitDisabled: Bool = false) -> ProGate {
        ProGate(store: ProEntitlementStore(mode: .mock), usage: CreationUsageStore(defaults: usageDefaults),
                quota: CreationQuota(dailyFreeLimit: limit), limitDisabled: limitDisabled)
    }

    // MARK: - つまみ無し(Release と同じ条件)

    func testWithoutKnobsGateIsUnchanged() {
        let g = gate(limit: 3)
        XCTAssertFalse(g.isPro)
        XCTAssertEqual(g.isPro, g.store.isPro)
        XCTAssertEqual(g.quota.dailyFreeLimit, 3)
        XCTAssertTrue(g.isEnforced)
        XCTAssertTrue(g.needsPaywall)
        XCTAssertEqual(g.decision, .allowed(remaining: 3))
        XCTAssertFalse(gate(limitDisabled: true).isEnforced)
        XCTAssertNil(CoinGate.debugService)
        XCTAssertNil(DebugMockUsage.source)
    }

    // MARK: - プラン

    func testPlanOverrideMakesGateProWithoutTouchingStore() {
        let g = gate(limit: 0)
        XCTAssertEqual(g.decision, .blocked)
        DebugPlanState.shared.set(DebugPlanKnobs.plan, "pro")
        XCTAssertTrue(g.isPro)
        XCTAssertFalse(g.store.isPro)
        XCTAssertFalse(g.needsPaywall)
        XCTAssertEqual(g.decision, .unlimited)
    }

    func testPlanOverrideFreeAndBackToActual() {
        let g = gate(limit: 0)
        DebugPlanState.shared.set(DebugPlanKnobs.plan, "free")
        XCTAssertFalse(g.isPro)
        XCTAssertTrue(g.needsPaywall)
        DebugPlanState.shared.set(DebugPlanKnobs.plan, nil)
        XCTAssertEqual(g.isPro, g.store.isPro)
    }

    func testPlanFromEnvironmentWinsOverMenu() {
        DebugPlanState.shared.set(DebugPlanKnobs.plan, "free")
        env[DebugPlanKnobs.plan] = "pro"
        XCTAssertTrue(gate().isPro)
    }

    // MARK: - 1 日の無料回数・制限の有効

    func testFreeLimitOverrideChangesDecision() {
        let g = gate(limit: 100)
        DebugPlanState.shared.set(DebugPlanKnobs.iapFreeLimit, "0")
        XCTAssertEqual(g.quota.dailyFreeLimit, 0)
        XCTAssertEqual(g.decision, .blocked)
        DebugPlanState.shared.set(DebugPlanKnobs.iapFreeLimit, "1")
        XCTAssertEqual(g.decision, .allowed(remaining: 1))
        g.recordNewApp()
        XCTAssertEqual(g.decision, .blocked)
        g.debugResetUsageToday()
        XCTAssertEqual(g.usedToday, 0)
        XCTAssertEqual(g.decision, .allowed(remaining: 1))
        DebugPlanState.shared.set(DebugPlanKnobs.iapFreeLimit, nil)
        XCTAssertEqual(g.quota.dailyFreeLimit, 100)
    }

    func testEnforceKnobOverridesLimitDisabled() {
        let g = gate(limit: 0, limitDisabled: true)
        XCTAssertFalse(g.isEnforced)
        XCTAssertEqual(g.decision, .unlimited)
        DebugPlanState.shared.set(DebugPlanKnobs.iapEnforce, "1")
        XCTAssertTrue(g.isEnforced)
        XCTAssertEqual(g.decision, .blocked)
        XCTAssertTrue(g.needsPaywall)
    }

    // MARK: - コイン

    func testCoinMockUsesBalanceAndDisabledKnobs() async throws {
        XCTAssertNil(CoinGate.debugService)
        DebugKnobs.set(DebugPlanKnobs.coins, "1")
        DebugKnobs.set(DebugPlanKnobs.coinsBalance, "0")
        let mock = try XCTUnwrap(CoinGate.debugService as? MockCoinService)
        let wallet = try await mock.wallet()
        XCTAssertEqual(wallet.balance.available, 0)
        let enabled = try await mock.planConfig()
        XCTAssertTrue(enabled.isEnabled)

        DebugKnobs.set(DebugPlanKnobs.coinsDisabled, "1")
        let disabled = try await XCTUnwrap(CoinGate.debugService).planConfig()
        XCTAssertFalse(disabled.isEnabled)
    }

    func testCoinStoreWithZeroBalanceMockIsEnabledAndDisabledHidesCoins() async {
        DebugKnobs.set(DebugPlanKnobs.coins, "1")
        DebugKnobs.set(DebugPlanKnobs.coinsBalance, "0")
        let store = CoinStore(mode: .mock, service: DebugPlanKnobs.coinService())
        await store.refreshConfig()
        await store.refreshWallet()
        XCTAssertTrue(store.isEnabled)
        XCTAssertEqual(store.wallet?.balance.available, 0)

        DebugKnobs.set(DebugPlanKnobs.coinsDisabled, "1")
        store.service = DebugPlanKnobs.coinService()
        await store.refreshConfig()
        XCTAssertFalse(store.isEnabled)
    }

    // MARK: - 使っている量

    func testMockUsageFromMenuPreset() async throws {
        let nearLimit = try XCTUnwrap(DebugPlanChoices.usages.first { $0.0 == "上限" }?.1)
        DebugKnobs.set(DebugPlanKnobs.mockUsage, nearLimit)
        let source = try XCTUnwrap(DebugMockUsage.source)
        let me = try await source.myUsage()
        XCTAssertEqual(me?.apps, UsageMeter(used: 10, limit: 10))

        let participant = try XCTUnwrap(DebugPlanChoices.usages.first { $0.0 == "参加者として見る" }?.1)
        DebugKnobs.set(DebugPlanKnobs.mockUsage, participant)
        let app = try await XCTUnwrap(DebugMockUsage.source).appUsage(appId: "a")
        XCTAssertEqual(app?.payer, "creator")
    }

    // MARK: - アプリの数の上限

    func testAppLimitSamples() {
        XCTAssertTrue(DebugPlanChoices.appLimit(canBuySlot: true).canBuySlot)
        XCTAssertNotNil(DebugPlanChoices.appLimit(canBuySlot: true).slotCoins)
        XCTAssertFalse(DebugPlanChoices.appLimit(canBuySlot: false).canBuySlot)
        XCTAssertGreaterThanOrEqual(DebugPlanChoices.appLimit(canBuySlot: false).held, DebugPlanChoices.appLimit(canBuySlot: false).limit)
    }

    func testSectionIsFound() {
        XCTAssertTrue(DebugMenuRegistry.discover().contains { $0 == DebugPlanSection.self })
    }
}
#endif
