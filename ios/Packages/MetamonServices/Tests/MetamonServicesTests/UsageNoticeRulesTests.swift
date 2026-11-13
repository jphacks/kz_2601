import XCTest
@testable import MetamonServices

/// 上限に近い時のお知らせ(`UsageNoticeRules`)と 1 日 1 回の記録(`UsageNoticeLog`)。
/// 日付は JST で数えるので、端末の TZ に依らず通る(TZ=America/Los_Angeles でも回す)。
final class UsageNoticeRulesTests: XCTestCase {
    private let suite = "UsageNoticeRulesTests"
    private var defaults: UserDefaults!

    override func setUp() {
        defaults = UserDefaults(suiteName: suite)
        defaults.removePersistentDomain(forName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    private func me(_ plan: String, apps: UsageMeter?) -> MyUsage { MyUsage(plan: plan, apps: apps) }

    private func app(payer: String = "self", upgrade: Bool = true, records: UsageMeter?, photos: UsageMeter?) -> AppUsage {
        AppUsage(appId: "app-1", plan: "free", payer: payer, upgrade: upgrade, records: records, photos: photos)
    }

    private func utc(_ s: String) -> Date { ISO8601DateFormatter().date(from: s)! }

    // MARK: - isNear

    func testIsNearBoundaryTable() {
        let cases: [(name: String, meter: UsageMeter?, want: Bool)] = [
            ("79%", UsageMeter(used: 79, limit: 100), false),
            ("80% ちょうど", UsageMeter(used: 80, limit: 100), true),
            ("100%", UsageMeter(used: 100, limit: 100), true),
            ("超えた", UsageMeter(used: 130, limit: 100), true),
            ("小さい上限 3/4 = 75%", UsageMeter(used: 3, limit: 4), false),
            ("小さい上限 4/5 = 80%", UsageMeter(used: 4, limit: 5), true),
            ("上限なし", UsageMeter(used: 999, limit: nil), false),
            ("上限 0", UsageMeter(used: 0, limit: 0), false),
            ("上限 0 で使った", UsageMeter(used: 5, limit: 0), false),
            ("値なし", nil, false),
        ]
        for c in cases { XCTAssertEqual(UsageNoticeRules.isNear(c.meter), c.want, c.name) }
        XCTAssertEqual(UsageNoticeRules.threshold, 0.8)
    }

    // MARK: - appsNotice

    func testAppsNotice() {
        let near = UsageMeter(used: 8, limit: 10)
        XCTAssertEqual(UsageNoticeRules.appsNotice(me("free", apps: near)),
                       UsageNotice(resource: .apps, meter: near, upgrade: true), "無料は Pro で上がる")
        XCTAssertEqual(UsageNoticeRules.appsNotice(me("pro", apps: near))?.upgrade, false, "Pro は Pro を勧めない")
        XCTAssertNil(UsageNoticeRules.appsNotice(me("free", apps: UsageMeter(used: 7, limit: 10))))
        XCTAssertNil(UsageNoticeRules.appsNotice(me("pro", apps: UsageMeter(used: 50, limit: nil))), "上限なし")
        XCTAssertNil(UsageNoticeRules.appsNotice(me("free", apps: nil)))
        XCTAssertNil(UsageNoticeRules.appsNotice(nil))
        XCTAssertEqual(UsageNoticeRules.appsNotice(me("free", apps: near))?.id, "apps")
    }

    // MARK: - appNotices

    func testAppNoticesForTheCreator() {
        let r = UsageMeter(used: 90, limit: 100), p = UsageMeter(used: 8, limit: 10)
        let both = UsageNoticeRules.appNotices(app(records: r, photos: p))
        XCTAssertEqual(both.map(\.resource), [.recordsPerApp, .photosPerApp], "記録 → 写真の順")
        XCTAssertEqual(both.map(\.id), ["recordsPerApp:app-1", "photosPerApp:app-1"])
        XCTAssertTrue(both.allSatisfy { $0.upgrade && $0.appId == "app-1" })

        XCTAssertEqual(UsageNoticeRules.appNotices(app(records: UsageMeter(used: 10, limit: 100), photos: p)).map(\.resource), [.photosPerApp])
        XCTAssertEqual(UsageNoticeRules.appNotices(app(upgrade: false, records: r, photos: nil)).map(\.upgrade), [false],
                       "Pro の作成者には Pro を勧めない")
        XCTAssertEqual(UsageNoticeRules.appNotices(app(records: UsageMeter(used: 9_000, limit: nil), photos: nil)), [], "上限なし")
    }

    func testAppNoticesNotForParticipants() {
        let full = UsageMeter(used: 100, limit: 100)
        XCTAssertEqual(UsageNoticeRules.appNotices(app(payer: "creator", records: full, photos: full)), [])
        XCTAssertEqual(UsageNoticeRules.appNotices(nil), [])
    }

    // MARK: - UsageNoticeLog

    func testOncePerDayPerAccount() {
        let log = UsageNoticeLog(defaults: defaults)
        let n = UsageNotice(resource: .apps, meter: UsageMeter(used: 9, limit: 10), upgrade: true)
        let morning = utc("2026-11-12T00:00:00Z") // JST 9:00
        XCTAssertTrue(log.shouldShow(n, uid: "u1", now: morning))
        log.markShown(n, uid: "u1", now: morning)
        XCTAssertFalse(log.shouldShow(n, uid: "u1", now: morning), "同じ日に 2 回出さない")
        XCTAssertFalse(log.shouldShow(n, uid: "u1", now: utc("2026-11-12T14:59:59Z")), "JST 23:59:59 はまだ同じ日")
        XCTAssertTrue(log.shouldShow(n, uid: "u1", now: utc("2026-11-12T15:00:00Z")), "JST の 0:00 で次の日")
        XCTAssertTrue(log.shouldShow(n, uid: "u2", now: morning), "利用者が変われば出す")
        XCTAssertTrue(log.shouldShow(n, uid: nil, now: morning), "サインアウトも別")
        let photos = UsageNotice(resource: .photosPerApp, appId: "app-1", meter: UsageMeter(used: 9, limit: 10), upgrade: true)
        XCTAssertTrue(log.shouldShow(photos, uid: "u1", now: morning), "リソースが違えば別")
    }

    func testMarkShownDropsOlderDaysAndClear() {
        let log = UsageNoticeLog(defaults: defaults)
        let a = UsageNotice(resource: .apps, meter: UsageMeter(used: 9, limit: 10), upgrade: true)
        let r = UsageNotice(resource: .recordsPerApp, appId: "x", meter: UsageMeter(used: 9, limit: 10), upgrade: true)
        let day1 = utc("2026-11-12T03:00:00Z"), day2 = utc("2026-11-13T03:00:00Z")
        log.markShown(a, uid: "u1", now: day1)
        log.markShown(r, uid: "u1", now: day2)
        let stored = defaults.dictionary(forKey: UsageNoticeLog.key) as? [String: String]
        XCTAssertEqual(stored, ["u1|recordsPerApp:x": "2026-11-13"], "前の日の分は捨てる")
        XCTAssertFalse(log.shouldShow(r, uid: "u1", now: day2))

        log.clear()
        XCTAssertNil(defaults.dictionary(forKey: UsageNoticeLog.key))
        XCTAssertTrue(log.shouldShow(r, uid: "u1", now: day2))
    }

    func testDayIsCountedInJST() {
        XCTAssertEqual(UsageNoticeLog.day(utc("2026-12-31T14:59:59Z")), "2026-12-31")
        XCTAssertEqual(UsageNoticeLog.day(utc("2026-12-31T15:00:00Z")), "2027-1-1")
        XCTAssertEqual(UsageNoticeLog.entry(UsageNotice(resource: .apps, meter: UsageMeter(used: 1, limit: 1), upgrade: false), uid: nil), "-|apps")
    }
}
