import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 共有のリンクから取り込み・参加する(#1287・#1286): 保留・確認の出し分け・取り込んだアプリを開く
@MainActor
final class MacShareFlowTests: XCTestCase {
    private actor FakeSharing: SharingClient, MembershipClient {
        var preview: ShareLinkPreview?
        var imported: [(token: String, displayName: String?)] = []
        var failure: Error?

        init(preview: ShareLinkPreview?) { self.preview = preview }
        func setFailure(_ error: Error?) { failure = error }

        func previewShare(token: String) async throws -> ShareLinkPreview {
            guard let preview else { throw URLError(.badServerResponse) }
            return preview
        }
        func importShare(token: String) async throws -> InstalledApp {
            if let failure { throw failure }
            imported.append((token, nil))
            return InstalledApp(appId: "shared-app", name: "家計簿")
        }
        func importShare(token: String, displayName: String) async throws -> InstalledApp {
            if let failure { throw failure }
            imported.append((token, displayName))
            return InstalledApp(appId: "shared-app", name: "家計簿", joined: true, member: true)
        }
        func share(appId: String) async throws -> ShareInfo { throw URLError(.unsupportedURL) }
        func revokeShare(token: String) async throws {}
        func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing {
            throw URLError(.unsupportedURL)
        }
        func unpublish(appId: String) async throws {}
        func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] { [] }
        func install(publicAppId: String) async throws -> InstalledApp { throw URLError(.unsupportedURL) }
        func hasMembersOnlyData(appId: String) async throws -> Bool { false }
        func members(appId: String) async throws -> SharedMemberList { throw URLError(.unsupportedURL) }
        func removeMember(appId: String, uid: String) async throws {}
        func leave(appId: String) async throws {}
        func setAdmin(appId: String, uid: String, admin: Bool) async throws {}
    }

    /// 取り込んだアプリの DSL を返す appconfig
    private actor FakeAppConfig: AppConfigClient {
        let document: AppDocument
        init(document: AppDocument) { self.document = document }
        func fetchDocument(appId: String) async throws -> AppDocument { document }
        func listVersions(appId: String) async throws -> [VersionSummary] { [] }
        func restore(appId: String, toVersion version: Int) async throws -> AppDocument { document }
        func list(appId: String, entity: String) async throws -> [Record] { [] }
        func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { Record(id: "x", data: data) }
        func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { Record(id: id, data: data) }
        func delete(appId: String, entity: String, id: String) async throws {}
    }

    private let link = URL(string: "https://tailor-it.app/s/tok_1")!

    private func document() throws -> AppDocument {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return AppDocument(appId: "shared-app", version: 3, dsl: try AppDSL.load(contentsOf: url))
    }

    private func flow(_ sharing: FakeSharing?, signedIn: @escaping @MainActor () -> Bool = { true }) throws -> MacShareFlow {
        let flow = MacShareFlow(sharing: sharing, appConfig: sharing == nil ? nil : FakeAppConfig(document: try document()),
                                isSignedIn: signedIn, envelope: { _ in nil })
        flow.hold = LaunchConsentHold() // 起動時の順番待ち(#1885)はテストのホストのアプリと分ける
        return flow
    }

    // MARK: - 保留(ios/Shared の PendingShareLink)

    func testPendingLinkKeepsOnlyShareLinksUntilReady() {
        var pending = PendingShareLink()
        XCTAssertFalse(pending.receive(URL(string: "https://example.com/s/tok")!))
        XCTAssertTrue(pending.receive(URL(string: "tailorit://import/tok_1")!))
        XCTAssertNil(pending.take(isReady: false), "準備の前は渡さない")
        XCTAssertNotNil(pending.url, "保留は残す")
        XCTAssertEqual(pending.take(isReady: true)?.absoluteString, "tailorit://import/tok_1")
        XCTAssertNil(pending.take(isReady: true), "1 回だけ")
    }

    func testPendingLinkReadsUniversalLinkActivity() {
        var pending = PendingShareLink()
        let activity = NSUserActivity(activityType: NSUserActivityTypeBrowsingWeb)
        activity.webpageURL = link
        XCTAssertTrue(pending.receive(activity: activity))
        XCTAssertEqual(pending.url, link)
    }

    // MARK: - 流れ

    func testWithoutServerShowsUnavailable() throws {
        let flow = try flow(nil)
        flow.receive(link)
        XCTAssertEqual(flow.unavailable, link)
        XCTAssertNil(flow.link.url)
    }

    func testHoldsUntilSignedInThenOpensOnce() async throws {
        var signedIn = false
        let flow = try flow(FakeSharing(preview: ShareLinkPreview(token: "tok_1", name: "家計簿")), signedIn: { signedIn })
        flow.receive(link)
        await flow.openPendingIfReady()
        XCTAssertNil(flow.pending, "ログインの前は開かない")
        signedIn = true
        await flow.openPendingIfReady()
        XCTAssertEqual(flow.pending?.kind, .add)
        XCTAssertEqual(flow.pending?.token, "tok_1")
        flow.pending = nil
        await flow.openPendingIfReady()
        XCTAssertNil(flow.pending, "同じリンクは 2 回開かない")
    }

    func testSharedDataAsksToJoinAndAdoptsApp() async throws {
        let sharing = FakeSharing(preview: ShareLinkPreview(token: "tok_1", name: "家計簿", sharedData: true))
        let flow = try flow(sharing)
        await flow.open(token: "tok_1")
        let pending = try XCTUnwrap(flow.pending)
        XCTAssertEqual(pending.kind, .join)

        let message = await flow.confirm(pending, displayName: "たろう")
        XCTAssertNil(message)
        XCTAssertNil(flow.pending, "できたらシートを閉じる")
        XCTAssertEqual(flow.adopted?.document.appId, "shared-app")
        XCTAssertTrue(flow.adopted?.banner.contains("家計簿") == true)
        let imported = await sharing.imported
        XCTAssertEqual(imported.first?.displayName, "たろう")
    }

    func testMineOrPlainAppAsksToAdd() async throws {
        let flow = try flow(FakeSharing(preview: ShareLinkPreview(token: "tok_1", name: "家計簿", mine: true, sharedData: true)))
        await flow.open(token: "tok_1")
        XCTAssertEqual(flow.pending?.kind, .add, "自分のアプリは参加ではなく追加")
    }

    func testRevokedLinkKeepsSheetWithMessage() async throws {
        let sharing = FakeSharing(preview: nil)
        await sharing.setFailure(APIError.server(status: 404, code: "not_found", message: "gone"))
        let flow = try flow(sharing)
        await flow.open(token: "tok_1")
        let pending = try XCTUnwrap(flow.pending, "プレビューが取れなくても名前なしで聞く")
        let message = await flow.confirm(pending)
        XCTAssertNotNil(message)
        XCTAssertNotNil(flow.pending, "失敗したらシートは開いたまま")
        XCTAssertNil(flow.adopted)
    }

    /// 取り込んだアプリはサーバーにあるアプリとして一覧に足す(記録・版をサーバーにつなぐ)
    func testAdoptedAppIsOnServer() throws {
        let library = MacAppLibrary(store: nil)
        let document = try document()
        library.apply(document, created: true, onServer: true)
        XCTAssertTrue(library.isCreated("shared-app"))
        XCTAssertTrue(library.isOnServer("shared-app"))
    }
}
