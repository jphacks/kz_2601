import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 「アプリを探す」(#2069): 検索の結果・空・エラーの出し分け、入れる流れ(上限・失敗)、通報・ブロック。
/// 問い合わせは偽のクライアント(通信しない)。
@MainActor
final class MacDiscoverTests: XCTestCase {
    private actor FakeSharing: SharingClient {
        var results: [PublicAppListing]
        var searchFailure: Error?
        var installFailures: [Error] = []
        var installed: InstalledApp
        private(set) var searches: [(query: String, sort: String)] = []
        private(set) var installs: [String] = []

        init(results: [PublicAppListing] = [], installed: InstalledApp = InstalledApp(appId: "installed-app", name: "家計簿")) {
            self.results = results
            self.installed = installed
        }

        func set(results: [PublicAppListing]) { self.results = results }
        func set(searchFailure: Error?) { self.searchFailure = searchFailure }
        func set(installFailures: [Error]) { self.installFailures = installFailures }

        func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] {
            searches.append((query, sort))
            if let searchFailure { throw searchFailure }
            return results
        }
        func install(publicAppId: String) async throws -> InstalledApp {
            installs.append(publicAppId)
            if !installFailures.isEmpty { throw installFailures.removeFirst() }
            return installed
        }
        func share(appId: String) async throws -> ShareInfo { throw URLError(.unsupportedURL) }
        func revokeShare(token: String) async throws {}
        func importShare(token: String) async throws -> InstalledApp { throw URLError(.unsupportedURL) }
        func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing {
            throw URLError(.unsupportedURL)
        }
        func unpublish(appId: String) async throws {}
    }

    private actor FakeAppConfig: AppConfigClient {
        let document: AppDocument
        private(set) var fetched: [String] = []
        init(document: AppDocument) { self.document = document }
        func fetchDocument(appId: String) async throws -> AppDocument {
            fetched.append(appId)
            return document
        }
        func listVersions(appId: String) async throws -> [VersionSummary] { [] }
        func restore(appId: String, toVersion version: Int) async throws -> AppDocument { document }
        func list(appId: String, entity: String) async throws -> [Record] { [] }
        func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { Record(id: "x", data: data) }
        func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { Record(id: id, data: data) }
        func delete(appId: String, entity: String, id: String) async throws {}
    }

    private actor FakeModeration: ModerationClient {
        var reports: [(ReportTarget, ReportReason)] = []
        var blockedListings: [String] = []
        var failure: Error?
        func set(failure: Error?) { self.failure = failure }

        func report(_ target: ReportTarget, reason: ReportReason) async throws {
            if let failure { throw failure }
            reports.append((target, reason))
        }
        func blockedUsers() async throws -> [BlockedUser] { [] }
        func block(uid: String, name: String?) async throws -> BlockedUser { BlockedUser(uid: uid, name: name ?? "") }
        func block(listingId: String) async throws -> BlockedUser {
            if let failure { throw failure }
            blockedListings.append(listingId)
            return BlockedUser(uid: "uid-publisher", name: "はなこ")
        }
        func unblock(uid: String) async throws {}
    }

    private let walk = PublicAppListing(id: "pub-walk", name: "散歩メモ", description: "歩いた道を記録", tags: ["散歩"],
                                        authorName: "はなこ", components: ["list", "map"], installs: 12)
    private let mine = PublicAppListing(id: "pub-mine", name: "自分の家計簿", mine: true)

    private func document() throws -> AppDocument {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return AppDocument(appId: "installed-app", version: 2, dsl: try AppDSL.load(contentsOf: url))
    }

    private func makeModel(_ sharing: FakeSharing?, moderation: FakeModeration? = nil, social: MacSocial? = nil) throws -> MacDiscoverModel {
        MacDiscoverModel(sharing: sharing, appConfig: sharing == nil ? nil : FakeAppConfig(document: try document()),
                         moderation: moderation, social: social, envelope: { _ in nil }, debounce: .zero)
    }

    private static let limitError = APIError.server(status: 402, code: "app_limit_reached", message: "上限",
                                                    details: ["held": .number(2), "limit": .number(2)])

    // MARK: - 検索の出し分け

    func testStartsLoadingThenShowsResults() async throws {
        let sharing = FakeSharing(results: [walk, mine])
        let model = try makeModel(sharing)
        XCTAssertEqual(model.phase, .loading, "最初の読み込みはスケルトン")
        await model.search()
        XCTAssertEqual(model.phase, .results)
        XCTAssertEqual(model.items.map(\.id), ["pub-walk", "pub-mine"])
        let searches = await sharing.searches
        XCTAssertEqual(searches.first?.sort, "popular", "既定は人気順")
    }

    func testSortAndQueryAreSentAsIs() async throws {
        let sharing = FakeSharing(results: [walk])
        let model = try makeModel(sharing)
        model.sort = .new
        model.query = "散歩"
        XCTAssertEqual(model.searchKey, "new|散歩|0")
        await model.search()
        let last = await sharing.searches.last
        XCTAssertEqual(last?.query, "散歩")
        XCTAssertEqual(last?.sort, "new")
    }

    func testEmptyShowsQueryInMessage() async throws {
        let model = try makeModel(FakeSharing())
        await model.search()
        XCTAssertEqual(model.phase, .empty(DiscoverCopy.emptyMessage(query: "")))
        model.query = "筋トレ"
        await model.search()
        guard case .empty(let message) = model.phase else { return XCTFail("0 件の表示") }
        XCTAssertTrue(message.contains("筋トレ"), "検索語を添える")
    }

    func testErrorShowsRetryAndRecovers() async throws {
        let sharing = FakeSharing(results: [walk])
        await sharing.set(searchFailure: URLError(.notConnectedToInternet))
        let model = try makeModel(sharing)
        await model.search()
        guard case .failed(let message) = model.phase else { return XCTFail("エラーの表示") }
        XCTAssertFalse(message.isEmpty)

        await sharing.set(searchFailure: nil)
        await model.search() // 「もう一度」
        XCTAssertEqual(model.phase, .results)
        XCTAssertNil(model.error)
    }

    func testErrorAfterResultsKeepsResults() async throws {
        let sharing = FakeSharing(results: [walk])
        let model = try makeModel(sharing)
        await model.search()
        await sharing.set(searchFailure: URLError(.timedOut))
        await model.search()
        XCTAssertEqual(model.phase, .results, "前の結果は残す(iOS と同じ)")
    }

    func testWithoutServerShowsUnavailable() async throws {
        let model = try makeModel(nil)
        await model.search()
        XCTAssertEqual(model.phase, .failed(DiscoverCopy.unavailable))
        await model.install(walk)
        XCTAssertEqual(model.installError, DiscoverCopy.unavailable)
    }

    // MARK: - 入れる

    func testInstallFetchesDocumentAndHandsItToShell() async throws {
        let sharing = FakeSharing(results: [walk])
        let model = try makeModel(sharing)
        await model.install(walk)
        let installs = await sharing.installs
        XCTAssertEqual(installs, ["pub-walk"])
        let installed = try XCTUnwrap(model.installed)
        XCTAssertEqual(installed.document.appId, "installed-app")
        XCTAssertTrue(installed.banner.contains("家計簿"))
        XCTAssertFalse(installed.shared, "自分のコピーは「共有されたアプリ」に入れない")
        XCTAssertNil(model.installing)
        XCTAssertNil(model.installError)
    }

    func testInstallJoiningSharedDataIsMarkedShared() async throws {
        let sharing = FakeSharing(installed: InstalledApp(appId: "installed-app", name: "町内会", joined: true))
        let model = try makeModel(sharing)
        await model.install(walk)
        XCTAssertEqual(model.installed?.shared, true)
        XCTAssertTrue(model.installed?.banner.hasSuffix(MacShareFlow.bannerExtra(InstalledApp(appId: "x", name: "", joined: true))) == true)
    }

    func testInstallAtLimitAsksToReplaceThenRetries() async throws {
        let sharing = FakeSharing()
        await sharing.set(installFailures: [Self.limitError])
        let model = try makeModel(sharing)
        await model.install(walk)
        let request = try XCTUnwrap(model.limitRequest, "上限なら置き換えのシート")
        XCTAssertEqual(request.limit.limit, 2)
        XCTAssertNil(model.installed)
        XCTAssertNil(model.installError, "上限は失敗の文言を出さない")

        // 1 つ空いたら入れ直す
        model.limitRequest = nil
        request.retry()
        for _ in 0..<50 where model.installed == nil { await Task.yield() }
        try await Task.sleep(for: .milliseconds(50))
        let installs = await sharing.installs
        XCTAssertEqual(installs, ["pub-walk", "pub-walk"])
        XCTAssertNotNil(model.installed)
    }

    func testInstallFailureKeepsSheetWithMessage() async throws {
        let sharing = FakeSharing()
        await sharing.set(installFailures: [URLError(.notConnectedToInternet)])
        let model = try makeModel(sharing)
        await model.install(walk)
        XCTAssertNil(model.installed)
        XCTAssertNotNil(model.installError)
        XCTAssertNil(model.installing, "失敗したらボタンを戻す")
    }

    // MARK: - 通報・ブロック

    func testModerationOnlyForOthersListings() throws {
        let withClient = try makeModel(FakeSharing(), moderation: FakeModeration())
        XCTAssertTrue(withClient.canModerate(walk))
        XCTAssertFalse(withClient.canModerate(mine), "自分の掲載には出さない")
        XCTAssertFalse(try makeModel(FakeSharing()).canModerate(walk), "通報の API が無ければ出さない")
    }

    func testReportSendsListingTarget() async throws {
        let moderation = FakeModeration()
        let model = try makeModel(FakeSharing(), moderation: moderation)
        model.reporting = walk
        await model.report(walk, reason: .spam)
        let reports = await moderation.reports
        XCTAssertEqual(reports.first?.0, .listing(id: "pub-walk"))
        XCTAssertNil(model.reporting)
        XCTAssertNotNil(model.notice)
    }

    func testBlockHidesPublisherAndReloads() async throws {
        let moderation = FakeModeration()
        let social = MacSocial()
        let sharing = FakeSharing(results: [walk])
        let model = try makeModel(sharing, moderation: moderation, social: social)
        await model.search()
        let before = model.searchKey
        model.blocking = walk
        await model.block(walk)
        let blocked = await moderation.blockedListings
        XCTAssertEqual(blocked, ["pub-walk"])
        XCTAssertEqual(social.blocked["uid-publisher"], "はなこ", "投稿も隠す")
        XCTAssertNotEqual(model.searchKey, before, "検索を読み直す")
        XCTAssertNil(model.blocking)
        XCTAssertEqual(model.notice?.title, L10n.tr("ブロックしました"))
    }

    func testBlockFailureKeepsKey() async throws {
        let moderation = FakeModeration()
        await moderation.set(failure: URLError(.timedOut))
        let model = try makeModel(FakeSharing(), moderation: moderation, social: MacSocial())
        let before = model.searchKey
        await model.block(walk)
        XCTAssertEqual(model.searchKey, before)
        XCTAssertEqual(model.notice?.title, L10n.tr("ブロックできませんでした"))
    }
}
