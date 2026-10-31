import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// シェアと通報・ブロック(#321)。成否を切り替えられる。
private final class FakeModeration: SharingClient, ModerationClient, @unchecked Sendable {
    private let lock = NSLock()
    var error: Error?
    private(set) var reports: [(ReportTarget, ReportReason)] = []
    private(set) var blockedUIDs: [String] = []

    func share(appId: String) async throws -> ShareInfo { throw URLError(.badURL) }
    func revokeShare(token: String) async throws {}
    func importShare(token: String) async throws -> InstalledApp { InstalledApp(appId: "a", name: "n") }
    func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing { PublicAppListing(id: appId, name: "n") }
    func unpublish(appId: String) async throws {}
    func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] { [] }
    func install(publicAppId: String) async throws -> InstalledApp { InstalledApp(appId: "a", name: "n") }

    func report(_ target: ReportTarget, reason: ReportReason) async throws {
        if let error { throw error }
        lock.withLock { reports.append((target, reason)) }
    }
    func blockedUsers() async throws -> [BlockedUser] { [BlockedUser(uid: "u-old", name: "前の人")] }
    func block(uid: String, name: String?) async throws -> BlockedUser {
        if let error { throw error }
        lock.withLock { blockedUIDs.append(uid) }
        return BlockedUser(uid: uid, name: name ?? "匿名")
    }
    func block(listingId: String) async throws -> BlockedUser {
        if let error { throw error }
        return BlockedUser(uid: "publisher", name: "こた")
    }
    func unblock(uid: String) async throws { if let error { throw error } }
}

@MainActor
final class ModerationTests: XCTestCase {
    private let me = MockAuthService.fixedUID
    private let other = Record(id: "r1", data: [:], author: "u-hanako", authorName: "はなこ")
    private lazy var mine = Record(id: "r2", data: [:], author: me, authorName: "わたし")

    override func setUp() {
        super.setUp()
        let ui = ModerationUI.shared
        ui.blocked = [:]
        ui.reporting = nil
        ui.blocking = nil
        ui.notice = nil
        ui.loadedFor = nil
    }

    private func runtime(_ fake: FakeModeration?, scope: String = "public") throws -> AppRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"掲示板","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[{"id":"post","name":"投稿","scope":"\(scope)","fields":[{"id":"body","name":"本文","type":"text"}]}],
         "screens":[{"id":"home","title":"掲示板","components":[{"type":"list","entity":"post"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        var services = BuildConfig.make(environment: .mock)
        services.sharing = fake
        return AppRuntime(document: AppDocument(appId: "board", version: 1, dsl: dsl), services: services)
    }

    func testCanReportOnlyOthersSharedPosts() throws {
        let rt = try runtime(FakeModeration())
        XCTAssertTrue(rt.canReport(entity: "post", record: other))
        XCTAssertFalse(rt.canReport(entity: "post", record: mine), "自分の投稿は通報しない")
        XCTAssertFalse(rt.canReport(entity: "post", record: Record(id: "x", data: [:])), "作者の分からない行")
        XCTAssertFalse(try runtime(nil).canReport(entity: "post", record: other), "シェアが無ければ出さない")
        XCTAssertFalse(try runtime(FakeModeration(), scope: "private").canReport(entity: "post", record: other), "private の entity")
    }

    func testReportSendsTargetAndShowsNotice() async throws {
        let fake = FakeModeration()
        let rt = try runtime(fake)
        rt.reportRecord(entity: "post", record: other)
        let req = try XCTUnwrap(ModerationUI.shared.reporting)
        await rt.submitReport(req, reason: .harassment)
        XCTAssertEqual(fake.reports.first?.0, .record(appId: "board", entityId: "post", recordId: "r1"))
        XCTAssertEqual(fake.reports.first?.1, .harassment)
        XCTAssertEqual(ModerationUI.shared.notice?.title, L10n.tr("通報しました"))
    }

    func testReportQuotaMessage() async throws {
        let fake = FakeModeration()
        fake.error = APIError.server(status: 429, code: "rate_limited", message: "too many reports today")
        let rt = try runtime(fake)
        await rt.submitReport(ModerationRequest(target: .listing(id: "L1"), authorName: "x"), reason: .spam)
        XCTAssertEqual(ModerationUI.shared.notice?.message, L10n.tr("今日はこれ以上通報できません。明日またお試しください"))
    }

    func testBlockHidesPostsImmediately() async throws {
        let fake = FakeModeration()
        let rt = try runtime(fake)
        XCTAssertEqual(rt.hidingBlocked([other, mine], entity: "post").map(\.id), ["r1", "r2"])
        rt.blockAuthor(entity: "post", record: other)
        await rt.confirmBlock(try XCTUnwrap(ModerationUI.shared.blocking))
        XCTAssertEqual(fake.blockedUIDs, ["u-hanako"])
        XCTAssertEqual(rt.hidingBlocked([other, mine], entity: "post").map(\.id), ["r2"])
        XCTAssertEqual(ModerationUI.shared.notice?.title, L10n.tr("ブロックしました"))
    }

    func testBlockFailureRestores() async throws {
        let fake = FakeModeration()
        fake.error = URLError(.notConnectedToInternet)
        let rt = try runtime(fake)
        rt.blockAuthor(entity: "post", record: other)
        await rt.confirmBlock(try XCTUnwrap(ModerationUI.shared.blocking))
        XCTAssertTrue(ModerationUI.shared.blocked.isEmpty)
        XCTAssertEqual(rt.hidingBlocked([other], entity: "post").count, 1)
    }

    func testBlockPublisherFromDiscoverAndLoadList() async throws {
        let fake = FakeModeration()
        let rt = try runtime(fake)
        await rt.loadBlockedUsers()
        XCTAssertEqual(ModerationUI.shared.blocked, ["u-old": "前の人"])
        let before = ModerationUI.shared.blockCount
        rt.blockPublisher(PublicAppListing(id: "L1", name: "家計簿", authorName: "こた"))
        let req = try XCTUnwrap(ModerationUI.shared.blocking)
        XCTAssertTrue(req.fromDiscover)
        await rt.confirmBlock(req)
        XCTAssertEqual(ModerationUI.shared.blocked["publisher"], "こた")
        XCTAssertEqual(ModerationUI.shared.blockCount, before + 1, "「探す」を読み直す")
    }
}
