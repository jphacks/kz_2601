import XCTest
import MetamonServices
@testable import Metamon

/// 中央の列の上のお知らせ(#1058): 新着・上限・サービスのお知らせの順番、閉じた記録、サインアウトで消えること
@MainActor
final class MacTopNoticesTests: XCTestCase {
    private var suiteName: String!
    private var defaults: UserDefaults!

    override func setUp() async throws {
        suiteName = "metamon.top-notices-test.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)
    }

    override func tearDown() async throws {
        defaults.removePersistentDomain(forName: suiteName)
    }

    private func appsNotice(used: Int = 9) -> UsageNotice {
        UsageNotice(resource: .apps, meter: UsageMeter(used: used, limit: 10), upgrade: true)
    }

    private struct StampSource: SharedRecordStampSource {
        var stamps: [RecordStamp]
        func sharedStamps(appId: String, entity: String, notBy uid: String) async throws -> [RecordStamp] { stamps }
    }

    private struct ConfigClient: ClientConfigClient {
        var config: ClientConfig
        func fetch() async throws -> ClientConfig { config }
    }

    // MARK: - 順番

    /// iOS と同じ: 完成の後のお知らせ → 新着 → 上限
    func testOrderPostBuildThenNewItemsThenUsage() {
        let notices = MacPostBuildNotices(defaults: defaults)
        notices.summary = .init(appId: "a", text: "できました", action: nil)
        notices.showsEditHint = true
        notices.newItems = .init(appId: "a", count: 2)
        notices.usage = appsNotice()

        XCTAssertEqual(notices.top, .postBuild(.summary))
        notices.dismissActive()
        XCTAssertEqual(notices.top, .postBuild(.editHint))
        notices.dismissActive()
        XCTAssertEqual(notices.top, .newItems)
        XCTAssertTrue(notices.isShowing)
        notices.dismissActive()
        XCTAssertNil(notices.newItems)
        XCTAssertEqual(notices.top, .usage)
        notices.dismissActive()
        XCTAssertNil(notices.usage)
        XCTAssertNil(notices.top)
        XCTAssertFalse(notices.isShowing)
    }

    /// サービスのお知らせは別の段(列を押し下げる)で、写しを止めない
    func testServiceNoticeDoesNotBlockThumbnail() {
        let notices = MacPostBuildNotices(defaults: defaults)
        notices.updateService(ServiceNotice(id: "m1", message: .init(ja: "メンテナンス")))
        XCTAssertEqual(notices.visibleService?.id, "m1")
        XCTAssertFalse(notices.isShowing)
        XCTAssertNil(notices.top)
    }

    func testUsageCandidatesPreferAppThenApps() {
        let app = AppUsage(appId: "a", plan: "free", payer: "self", upgrade: true,
                           records: UsageMeter(used: 95, limit: 100), photos: nil)
        let me = MyUsage(plan: "free", apps: UsageMeter(used: 9, limit: 10))
        let list = MacPostBuildNotices.usageCandidates(app: app, me: me)
        XCTAssertEqual(list.map(\.resource), [.recordsPerApp, .apps])
        XCTAssertTrue(MacPostBuildNotices.usageCandidates(app: nil, me: MyUsage(plan: "free", apps: UsageMeter(used: 1, limit: 10))).isEmpty)
    }

    // MARK: - 閉じた・見た記録

    /// 閉じたサービスのお知らせは、作り直しても(次の起動)出さない。新しい id は出す
    func testServiceDismissalPersists() {
        let notices = MacPostBuildNotices(defaults: defaults)
        notices.updateService(ServiceNotice(id: "m1", message: .init(ja: "メンテナンス")))
        notices.dismissService()
        XCTAssertNil(notices.visibleService)

        let relaunched = MacPostBuildNotices(defaults: defaults)
        relaunched.updateService(ServiceNotice(id: "m1", message: .init(ja: "メンテナンス")))
        XCTAssertNil(relaunched.visibleService)
        relaunched.updateService(ServiceNotice(id: "m2", severity: .warning, message: .init(ja: "AI を止めています")))
        XCTAssertEqual(relaunched.visibleService?.id, "m2")
    }

    /// 上限のお知らせは 1 日 1 回(見えた時点で数える)。別のアカウントは別に数える
    func testUsageShownOncePerDay() {
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        let notices = MacPostBuildNotices(defaults: defaults)
        notices.offerUsage([appsNotice()], uid: "u1", now: now)
        XCTAssertNotNil(notices.usage)
        notices.markUsageShown(uid: "u1", now: now)
        notices.dismissActive()

        let relaunched = MacPostBuildNotices(defaults: defaults)
        relaunched.offerUsage([appsNotice()], uid: "u1", now: now.addingTimeInterval(60))
        XCTAssertNil(relaunched.usage, "同じ日はもう出さない")
        relaunched.offerUsage([appsNotice()], uid: "u2", now: now.addingTimeInterval(60))
        XCTAssertNotNil(relaunched.usage, "別のアカウントは別に数える")
        relaunched.dismissActive()
        relaunched.offerUsage([appsNotice()], uid: "u1", now: now.addingTimeInterval(2 * 86_400))
        XCTAssertNotNil(relaunched.usage, "次の日はまた出す")
    }

    /// 新着: 初めて開いた時は基準だけ作る → 次に開いた時に他の人の分を数える
    func testNewItemsCountsSinceLastOpened() async {
        let notices = MacPostBuildNotices(defaults: defaults)
        let t0 = Date(timeIntervalSince1970: 1_800_000_000)
        let source = StampSource(stamps: [
            RecordStamp(id: "r1", author: "other", createdAt: "2027-01-15T08:10:00Z"),
            RecordStamp(id: "r2", author: "me", createdAt: "2027-01-15T08:10:00Z"),
        ])
        await notices.checkNewItems(appId: "a", entities: ["memo"], uid: "me", source: source, now: t0)
        XCTAssertNil(notices.newItems, "初めて開いた時は出さない")

        await notices.checkNewItems(appId: "a", entities: ["memo"], uid: "me", source: source, now: t0.addingTimeInterval(86_400 * 30))
        XCTAssertEqual(notices.newItems?.count, 1)
        XCTAssertEqual(notices.top, .newItems)

        await notices.checkNewItems(appId: "a", entities: ["memo"], uid: nil, source: source)
        XCTAssertNil(notices.newItems, "サインインしていなければ出さない")
    }

    func testServiceNoticeFromClientConfig() async {
        let notices = MacPostBuildNotices(defaults: defaults)
        var config = ClientConfig()
        config.notice = ServiceNotice(id: "m1", message: .init(ja: "メンテナンス"))
        await notices.checkService(checker: UpdateChecker(client: ConfigClient(config: config), build: "100"))
        XCTAssertEqual(notices.visibleService?.id, "m1")
        await notices.checkService(checker: UpdateChecker(client: nil, build: "100"))
        XCTAssertNil(notices.visibleService, "読めない時は出さない")
    }

    // MARK: - サインアウト(#972)

    /// `MacLocalWipe` で閉じた・見た記録が消え、`resetAfterWipe` で出ているものも下がる
    func testWipeClearsSeenStateAndVisibleNotices() async {
        let now = Date()
        let notices = MacPostBuildNotices(defaults: defaults)
        notices.updateService(ServiceNotice(id: "m1", message: .init(ja: "メンテナンス")))
        notices.dismissService()
        notices.offerUsage([appsNotice()], uid: "u1", now: now)
        notices.markUsageShown(uid: "u1", now: now)
        notices.lastOpened.mark(appId: "a", at: now)
        notices.newItems = .init(appId: "a", count: 3)
        notices.summary = .init(appId: "a", text: "できました", action: nil)
        defaults.synchronize()

        let root = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-top-notices-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        var wipe = MacLocalWipe()
        wipe.records = MacRecordEngines(root: root.appendingPathComponent("records", isDirectory: true))
        wipe.thumbnails = MacAppThumbnailStore(directory: root.appendingPathComponent("thumbs", isDirectory: true))
        wipe.documents = DocumentCache(directory: root.appendingPathComponent("dsl", isDirectory: true))
        wipe.appList = AppListCache(directory: root.appendingPathComponent("list", isDirectory: true))
        wipe.localList = AppListCache(directory: root.appendingPathComponent("local-list", isDirectory: true))
        #if !APP_STORE_REVIEW
        wipe.feeds = FeedCache()
        #endif
        wipe.defaults = defaults
        wipe.domain = suiteName
        wipe.resetSharedState = {}
        wipe.removeNotifications = {}
        wipe.clearWidget = {}
        await wipe.run()
        notices.resetAfterWipe()

        XCTAssertNil(notices.top)
        XCTAssertNil(notices.lastOpened.lastOpened(appId: "a"))
        XCTAssertEqual(notices.visibleService?.id, "m1", "閉じた記録が消えたのでまた出す")
        notices.offerUsage([appsNotice()], uid: "u1", now: now)
        XCTAssertNotNil(notices.usage, "見た記録が消えたのでまた出す")
    }
}
