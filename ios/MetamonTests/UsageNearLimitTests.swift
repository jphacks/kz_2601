import XCTest
import MetamonServices
import MetamonRenderer
@testable import Metamon

/// 上限に近い時のお知らせ(#317): 使用量の読み取り・80% の境目・1 日 1 回・参加者には出さない・キャッシュ。
@MainActor
final class UsageNearLimitTests: XCTestCase {
    private var saved: AppLanguage = .system
    private var defaults: UserDefaults!
    private let suite = "UsageNearLimitTests"

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
        defaults = UserDefaults(suiteName: suite)
        defaults.removePersistentDomain(forName: suite)
    }

    override func tearDown() {
        LanguagePreference.shared.setting = saved
        defaults.removePersistentDomain(forName: suite)
    }

    // MARK: - Decoding

    func testDecodesMyUsageExample() throws {
        let json = """
        { "plan": "free", "tier": "signed_in", "resetAt": "2026-11-07T15:00:00Z",
          "ai": { "used": 12, "limit": 60, "remaining": 48, "weights": { "create": 3, "edit": 2, "chat": 1, "text": 1, "textTask": 2 } },
          "apps": { "used": 3, "limit": 100, "remaining": 97 },
          "perApp": { "recordsPerApp": 5000, "photosPerApp": 500 } }
        """
        let me = try MyUsage.decode(Data(json.utf8))
        XCTAssertEqual(me.plan, "free")
        XCTAssertFalse(me.isPro)
        XCTAssertEqual(me.apps, UsageMeter(used: 3, limit: 100, remaining: 97))
        XCTAssertEqual(me.perApp?.recordsPerApp, 5000)
        XCTAssertEqual(me.resetAt, ISO8601DateFormatter().date(from: "2026-11-07T15:00:00Z"))
        let today = try XCTUnwrap(me.remainingToday(observedAt: .now))
        XCTAssertEqual(today.remaining, 48, "AI は「今日の残り N」としてだけ使う(X3)")
        XCTAssertEqual(today.limit, 60)
    }

    func testDecodesNullsAsNotCounted() throws {
        let json = """
        { "plan": "pro", "tier": "pro", "resetAt": "2026-11-07T15:00:00Z", "ai": null,
          "apps": { "used": 4, "limit": null, "remaining": null }, "perApp": { "recordsPerApp": null, "photosPerApp": null } }
        """
        let me = try MyUsage.decode(Data(json.utf8))
        XCTAssertTrue(me.isPro)
        XCTAssertNil(me.ai)
        XCTAssertNil(me.remainingToday(observedAt: .now))
        XCTAssertNil(me.apps?.limit)
        XCTAssertNil(UsageNoticeRules.appsNotice(me), "数えない上限には出さない")
    }

    func testDecodesAppUsage() throws {
        let json = """
        { "appId": "kakeibo-1", "plan": "pro", "payer": "creator", "upgrade": false,
          "records": { "used": 120, "limit": 50000, "remaining": 49880 }, "photos": null }
        """
        let usage = try JSONDecoder().decode(AppUsage.self, from: Data(json.utf8))
        XCTAssertEqual(usage.appId, "kakeibo-1")
        XCTAssertFalse(usage.isMine)
        XCTAssertEqual(usage.records?.used, 120)
        XCTAssertNil(usage.photos)
    }

    // MARK: - Threshold

    func testThresholdIsEightyPercent() {
        XCTAssertFalse(UsageNoticeRules.isNear(UsageMeter(used: 79, limit: 100)))
        XCTAssertTrue(UsageNoticeRules.isNear(UsageMeter(used: 80, limit: 100)))
        XCTAssertTrue(UsageNoticeRules.isNear(UsageMeter(used: 120, limit: 100)))
        XCTAssertFalse(UsageNoticeRules.isNear(UsageMeter(used: 5, limit: nil)), "上限なし")
        XCTAssertFalse(UsageNoticeRules.isNear(UsageMeter(used: 0, limit: 0)))
        XCTAssertFalse(UsageNoticeRules.isNear(nil))
    }

    func testAppsNoticeUpgradeOnlyOnFree() throws {
        let free = MyUsage(plan: "free", apps: UsageMeter(used: 8, limit: 10))
        let notice = try XCTUnwrap(UsageNoticeRules.appsNotice(free))
        XCTAssertEqual(notice.resource, .apps)
        XCTAssertTrue(notice.upgrade)
        XCTAssertTrue(UsageCopy.showsProButton(notice, isPro: false))
        XCTAssertFalse(UsageCopy.showsProButton(notice, isPro: true), "Pro には出さない")

        let pro = MyUsage(plan: "pro", apps: UsageMeter(used: 90, limit: 100))
        XCTAssertEqual(UsageNoticeRules.appsNotice(pro)?.upgrade, false)
        XCTAssertNil(UsageNoticeRules.appsNotice(MyUsage(plan: "free", apps: UsageMeter(used: 7, limit: 10))))
    }

    func testAppNoticesForCreatorOnly() {
        let records = UsageMeter(used: 4500, limit: 5000), photos = UsageMeter(used: 450, limit: 500)
        let mine = AppUsage(appId: "a", plan: "free", payer: "self", upgrade: true, records: records, photos: photos)
        XCTAssertEqual(UsageNoticeRules.appNotices(mine).map(\.resource), [.recordsPerApp, .photosPerApp])
        XCTAssertEqual(UsageNoticeRules.appNotices(mine).first?.id, "recordsPerApp:a")

        let joined = AppUsage(appId: "a", plan: "free", payer: "creator", upgrade: true, records: records, photos: photos)
        XCTAssertTrue(UsageNoticeRules.appNotices(joined).isEmpty, "参加者には量も Pro も出さない")

        let low = AppUsage(appId: "a", plan: "free", payer: "self", upgrade: true,
                           records: UsageMeter(used: 10, limit: 5000), photos: nil)
        XCTAssertTrue(UsageNoticeRules.appNotices(low).isEmpty)
    }

    // MARK: - Once per day

    func testOncePerResourcePerDay() {
        let log = UsageNoticeLog(defaults: defaults)
        let now = ISO8601DateFormatter().date(from: "2026-11-08T03:00:00Z")! // 12:00 JST
        let apps = UsageNotice(resource: .apps, meter: UsageMeter(used: 9, limit: 10), upgrade: true)
        let records = UsageNotice(resource: .recordsPerApp, appId: "a", meter: UsageMeter(used: 9, limit: 10), upgrade: true)
        let otherApp = UsageNotice(resource: .recordsPerApp, appId: "b", meter: UsageMeter(used: 9, limit: 10), upgrade: true)

        XCTAssertTrue(log.shouldShow(apps, uid: "u1", now: now))
        log.markShown(apps, uid: "u1", now: now)
        XCTAssertFalse(log.shouldShow(apps, uid: "u1", now: now.addingTimeInterval(3600)), "同じ日はもう出さない")
        XCTAssertTrue(log.shouldShow(records, uid: "u1", now: now), "別のリソースは別に数える")
        log.markShown(records, uid: "u1", now: now)
        XCTAssertTrue(log.shouldShow(otherApp, uid: "u1", now: now), "別のアプリの記録も別")

        let nextDayJST = ISO8601DateFormatter().date(from: "2026-11-08T15:00:00Z")! // 0:00 JST
        XCTAssertTrue(log.shouldShow(apps, uid: "u1", now: nextDayJST), "JST で日付が変われば また出す")
        log.markShown(apps, uid: "u1", now: nextDayJST)
        XCTAssertTrue(log.shouldShow(records, uid: "u1", now: nextDayJST))
    }

    /// 1 日 1 回はアカウントごと: 同じ端末で別のアカウントに切り替えたら、そのアカウントにはまだ出す。
    func testOncePerDayIsPerAccount() {
        let log = UsageNoticeLog(defaults: defaults)
        let now = ISO8601DateFormatter().date(from: "2026-11-08T03:00:00Z")!
        let apps = UsageNotice(resource: .apps, meter: UsageMeter(used: 9, limit: 10), upgrade: true)
        log.markShown(apps, uid: "u1", now: now)
        XCTAssertFalse(log.shouldShow(apps, uid: "u1", now: now))
        XCTAssertTrue(log.shouldShow(apps, uid: "u2", now: now), "別のアカウントは別に数える")
        XCTAssertTrue(log.shouldShow(apps, uid: nil, now: now))
        log.markShown(apps, uid: "u2", now: now)
        XCTAssertFalse(log.shouldShow(apps, uid: "u2", now: now))
        XCTAssertFalse(log.shouldShow(apps, uid: "u1", now: now), "u2 の記録で u1 の分を消さない")
    }

    // MARK: - Copy

    func testNoticeCopyReusesLimitWording() {
        let near = UsageNotice(resource: .recordsPerApp, appId: "a", meter: UsageMeter(used: 4200, limit: 5000), upgrade: true)
        XCTAssertEqual(UsageCopy.notice(near), "このアプリに保存できる記録の数が上限に近づいています(4200 / 5000)")
        let full = UsageNotice(resource: .photosPerApp, appId: "a", meter: UsageMeter(used: 500, limit: 500), upgrade: true)
        XCTAssertEqual(UsageCopy.notice(full), "このアプリに保存できる写真の数が上限に達しています(500 / 500)")
        XCTAssertEqual(UsageCopy.amount(UsageMeter(used: 3, limit: nil)), "3(上限なし)")
        XCTAssertEqual(UsageCopy.accessibilityAmount(UsageMeter(used: 3, limit: 100)), "100 のうち 3")
    }

    // MARK: - Cache

    func testStoreCachesFiveMinutesAndRefreshesAfterInvalidate() async {
        let source = FakeSource()
        let store = PlanUsageStore()
        let t0 = Date(timeIntervalSince1970: 1_000_000)
        await store.refreshMe(source: source, now: t0)
        await store.refreshMe(source: source, now: t0.addingTimeInterval(60))
        XCTAssertEqual(source.meCalls.value, 1, "5 分以内は読まない")
        await store.refreshMe(source: source, now: t0.addingTimeInterval(PlanUsageStore.ttl + 1))
        XCTAssertEqual(source.meCalls.value, 2)
        let before = store.revision
        store.invalidateMe()
        XCTAssertGreaterThan(store.revision, before)
        await store.refreshMe(source: source, now: t0.addingTimeInterval(PlanUsageStore.ttl + 2))
        XCTAssertEqual(source.meCalls.value, 3, "作った・消した後は読み直す")

        await store.refreshApp(appId: "a", source: source, now: t0)
        await store.refreshApp(appId: "a", source: source, now: t0.addingTimeInterval(10))
        XCTAssertEqual(source.appCalls.value, 1)
        store.invalidateApp("a")
        await store.refreshApp(appId: "a", source: source, now: t0.addingTimeInterval(20))
        XCTAssertEqual(source.appCalls.value, 2, "記録を送った後は読み直す")
    }

    func testOlderServerHidesSection() async {
        let source = FakeSource(missing: true) // 404 → nil
        let store = PlanUsageStore()
        let me = await store.refreshMe(source: source)
        let app = await store.refreshApp(appId: "a", source: source)
        XCTAssertNil(me)
        XCTAssertNil(app)
        XCTAssertNil(UsageNoticeRules.appsNotice(store.me))
        XCTAssertTrue(UsageNoticeRules.appNotices(store.app("a")).isEmpty)
    }

    func testFailureKeepsLastValueAndRetries() async {
        let source = FakeSource()
        let store = PlanUsageStore()
        let t0 = Date(timeIntervalSince1970: 1_000_000)
        await store.refreshMe(source: source, now: t0)
        source.fail.value = true
        await store.refreshMe(source: source, force: true, now: t0.addingTimeInterval(1))
        XCTAssertEqual(store.me?.apps?.used, 9, "読めなかった時は前の値のまま")
        source.fail.value = false
        await store.refreshMe(source: source, now: t0.addingTimeInterval(2))
        XCTAssertEqual(source.meCalls.value, 3, "失敗の後は 5 分待たずに読み直す")
    }

    /// キャッシュの `me` を「今日の残り」に入れる時は読んだ時刻で記録する(後から HTTP の応答で入った新しい値を上書きしない)。
    func testCachedMeKeepsFetchTimeAsObservation() async throws {
        let source = FakeSource(ai: true)
        let store = PlanUsageStore()
        let t0 = Date(timeIntervalSince1970: 1_000_000)
        await store.refreshMe(source: source, now: t0)
        XCTAssertEqual(store.meObservedAt, t0)
        let cached = await store.refreshMe(source: source, now: t0.addingTimeInterval(120))
        XCTAssertEqual(source.meCalls.value, 1)
        XCTAssertEqual(store.meObservedAt, t0, "キャッシュを返しても読んだ時刻は変えない")

        let remaining = RemainingTodayStore(defaults: defaults)
        let header = RemainingToday(remaining: 40, limit: 60, observedAt: t0.addingTimeInterval(60)) // 後で使った分
        remaining.record(header)
        let fromCache = try XCTUnwrap(cached?.remainingToday(observedAt: try XCTUnwrap(store.meObservedAt)))
        remaining.record(fromCache)
        XCTAssertEqual(remaining.last?.remaining, 40, "古いキャッシュ(48)で新しい残り(40)を上書きしない")
    }

    /// 読む間にサインアウト(clear)したら、届いた前のアカウントの量を書き戻さない。
    func testClearDuringFetchDropsResult() async {
        let store = PlanUsageStore()
        let source = FakeSource(onFetch: { await MainActor.run { store.clear() } })
        let me = await store.refreshMe(source: source)
        XCTAssertNil(me)
        XCTAssertNil(store.me, "サインアウト後に前のアカウントの量を書き戻さない")
        XCTAssertNil(store.meObservedAt)
        let app = await store.refreshApp(appId: "a", source: source)
        XCTAssertNil(app)
        XCTAssertNil(store.app("a"))
        // サインアウトの後に始めた読み取りは入る
        let fresh = await store.refreshMe(source: FakeSource())
        XCTAssertNotNil(fresh)
        XCTAssertNotNil(store.me)
    }

    func testClearForgetsAccount() async {
        let store = PlanUsageStore()
        await store.refreshMe(source: FakeSource())
        await store.refreshApp(appId: "a", source: FakeSource())
        store.clear()
        XCTAssertNil(store.me)
        XCTAssertNil(store.app("a"))
    }
}

private final class Box<T>: @unchecked Sendable {
    var value: T
    init(_ value: T) { self.value = value }
}

private struct FakeSource: PlanUsageSource {
    var missing = false
    var ai = false
    /// 応答を返す直前に呼ぶ(読む間のサインアウト)
    var onFetch: (@Sendable () async -> Void)?
    let meCalls = Box(0)
    let appCalls = Box(0)
    let fail = Box(false)

    func myUsage() async throws -> MyUsage? {
        meCalls.value += 1
        await onFetch?()
        if fail.value { throw APIError.server(status: 500, code: "internal", message: "") }
        return missing ? nil : MyUsage(plan: "free", ai: ai ? MyUsage.AI(used: 12, limit: 60, remaining: 48) : nil,
                                       apps: UsageMeter(used: 9, limit: 10))
    }

    func appUsage(appId: String) async throws -> AppUsage? {
        appCalls.value += 1
        await onFetch?()
        return missing ? nil : AppUsage(appId: appId, plan: "free", payer: "self", upgrade: true,
                                        records: UsageMeter(used: 4000, limit: 5000), photos: nil)
    }
}
