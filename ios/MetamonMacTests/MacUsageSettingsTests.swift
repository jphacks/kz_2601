import XCTest
import MetamonServices
@testable import Metamon

/// Mac の設定の「今日の残り」「使っている量」(#1219)
@MainActor
final class MacUsageSettingsTests: XCTestCase {
    private struct Source: PlanUsageSource {
        var me: MyUsage?
        func myUsage() async throws -> MyUsage? { me }
        func appUsage(appId: String) async throws -> AppUsage? { nil }
    }

    override func tearDown() async throws {
        PlanUsageStore.shared.clear()
        RemainingTodayStore.shared.clear()
    }

    /// サーバーの使っている量を読み、AI の残りを「今日の残り」にも入れる(iOS の `refreshPlanUsage` と同じ)
    func testRefreshReadsUsageAndRecordsRemainingToday() async {
        let me = MyUsage(plan: "free", ai: .init(used: 6, limit: 30, remaining: 24), apps: UsageMeter(used: 3, limit: 5))
        await MacUsageSettingsSection.refresh(source: Source(me: me), force: true)
        XCTAssertEqual(PlanUsageStore.shared.me?.apps, UsageMeter(used: 3, limit: 5))
        XCTAssertEqual(RemainingTodayStore.shared.last?.remaining, 24)
        XCTAssertEqual(RemainingTodaySettingsSection.amount(RemainingTodayStore.shared.current()), "24 / 30")
    }

    /// サーバーにつながっていない(モック)時は何も読まない
    func testRefreshWithoutSourceKeepsNothing() async {
        await MacUsageSettingsSection.refresh(source: nil, force: true)
        XCTAssertNil(PlanUsageStore.shared.me)
        XCTAssertNil(RemainingTodayStore.shared.last)
    }

    func testUsageCopy() {
        XCTAssertEqual(UsageCopy.amount(UsageMeter(used: 3, limit: 5)), "3 / 5")
        XCTAssertTrue(UsageNoticeRules.isNear(UsageMeter(used: 4, limit: 5)))
    }
}
