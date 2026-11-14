import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// サイドバーにサーバーの自分のアプリを混ぜる(#1057)
@MainActor
final class MacServerAppListTests: XCTestCase {
    /// appconfig の偽物(一覧・DSL・名前・並べ替え)
    private actor FakeAppConfig: AppConfigClient {
        var apps: [AppSummary] = []
        var docs: [String: AppDocument] = [:]
        var reachable = true
        var delay: Duration = .zero
        private(set) var listCalls = 0
        private(set) var renamed: [String: String] = [:]
        private(set) var orders: [[String]] = []

        func set(apps: [AppSummary]) { self.apps = apps }
        func set(doc: AppDocument) { docs[doc.appId] = doc }
        func setReachable(_ on: Bool) { reachable = on }
        func setDelay(_ d: Duration) { delay = d }

        func listApps() async throws -> [AppSummary] {
            listCalls += 1
            if delay != .zero { try? await Task.sleep(for: delay) }
            guard reachable else { throw URLError(.notConnectedToInternet) }
            return apps
        }

        func fetchDocument(appId: String) async throws -> AppDocument {
            guard reachable, let doc = docs[appId] else { throw URLError(.notConnectedToInternet) }
            return doc
        }

        func renameApp(appId: String, name: String) async throws -> AppSummary {
            renamed[appId] = name
            return AppSummary(id: appId, name: name, currentVersion: 3)
        }

        func reorderApps(ids: [String]) async throws { orders.append(ids) }

        func listVersions(appId: String) async throws -> [VersionSummary] { [] }
        func restore(appId: String, toVersion version: Int) async throws -> AppDocument { throw URLError(.badServerResponse) }
        func list(appId: String, entity: String) async throws -> [Record] { [] }
        func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { Record(id: "x", data: data) }
        func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { Record(id: id, data: data) }
        func delete(appId: String, entity: String, id: String) async throws {}
    }

    private var root: URL!

    override func setUp() async throws {
        root = FileManager.default.temporaryDirectory.appendingPathComponent("mac1057-\(UUID().uuidString)", isDirectory: true)
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: root)
    }

    private func cache(_ name: String = "server") -> AppListCache {
        AppListCache(directory: root.appendingPathComponent(name, isDirectory: true))
    }

    private func document(_ appId: String, _ name: String, version: Int = 2) throws -> AppDocument {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        var dsl = try AppDSL.load(contentsOf: url)
        dsl.app.name = name
        return AppDocument(appId: appId, version: version, dsl: dsl)
    }

    private func summary(_ id: String, _ name: String, version: Int = 2, updated: String? = nil, sort: Int? = nil,
                         status: String = "active") -> AppSummary {
        AppSummary(id: id, name: name, currentVersion: version, status: status, updatedAt: updated, sortIndex: sort)
    }

    // MARK: - 並び

    func testMergedPutsDeviceOnlyAppsFirstThenServerOrder() {
        XCTAssertEqual(MacServerAppList.merged(local: ["local", "b", "dup"], server: ["a", "b"]), ["local", "dup", "a", "b"])
        XCTAssertEqual(MacServerAppList.merged(local: ["x"], server: []), ["x"], "サーバーが空なら端末のまま")
        XCTAssertEqual(MacServerAppList.merged(local: [], server: ["a", "a"]), ["a"], "重ねない")
    }

    func testRefreshUsesIOSOrderingAndSkipsBlankAndArchived() async throws {
        let server = FakeAppConfig()
        await server.set(apps: [
            summary("old", "古い", updated: "2026-01-01T00:00:00Z"),
            summary("new", "新しい", updated: "2026-02-01T00:00:00Z"),
            summary("blank", "空", version: 1),
            summary("gone", "しまった", status: "archived"),
        ])
        let library = MacAppLibrary(store: nil, server: server, serverEnvelope: { _ in nil })
        library.apply(try document("mac-local", "端末だけ"), created: true)

        await library.refreshServerApps()

        XCTAssertEqual(library.createdIds, ["mac-local", "new", "old"], "端末だけのアプリを上に、続けて iOS と同じ新しい順")
        XCTAssertEqual(library.serverListState, .loaded)
        XCTAssertTrue(library.isServerOnly("new"))
        XCTAssertTrue(library.isOnServer("new"), "サーバーのアプリは記録もサーバーにつなぐ")
        XCTAssertFalse(library.isOnServer("mac-local"))
    }

    func testRefreshFollowsUserSortIndex() async throws {
        let server = FakeAppConfig()
        await server.set(apps: [summary("a", "A", sort: 2), summary("b", "B", sort: 1), summary("c", "C", updated: "2026-03-01T00:00:00Z")])
        let library = MacAppLibrary(store: nil, server: server)
        await library.refreshServerApps()
        XCTAssertEqual(library.createdIds, ["c", "b", "a"], "並べた後に増えたアプリが先頭・続けて並べた順(iOS と同じ)")
    }

    // MARK: - キャッシュ・オフライン・モック

    func testCachedListShowsImmediatelyThenRefreshReplaces() async throws {
        let saved = cache()
        saved.store([summary("cached", "前回")])
        let server = FakeAppConfig()
        await server.set(apps: [summary("cached", "前回の名前を変えた"), summary("fresh", "新しい")])
        let library = MacAppLibrary(store: nil, server: server, serverCache: saved)

        XCTAssertEqual(library.createdIds, ["cached"], "取り直す前から前回の一覧を出す")
        XCTAssertEqual(library.serverListState, .idle, "前回の一覧があればスケルトンを出さない")

        await library.refreshServerApps()
        XCTAssertEqual(Set(library.createdIds), ["cached", "fresh"])
        XCTAssertEqual(library.serverSummary("cached")?.name, "前回の名前を変えた")
    }

    func testOfflineKeepsCachedList() async throws {
        let saved = cache()
        saved.store([summary("cached", "前回")])
        let server = FakeAppConfig()
        await server.setReachable(false)
        let library = MacAppLibrary(store: nil, server: server, serverCache: saved)

        await library.refreshServerApps()
        XCTAssertEqual(library.createdIds, ["cached"])
        XCTAssertEqual(library.serverListState, .offline)
    }

    func testMockConfigIgnoresServerList() async throws {
        let saved = cache()
        saved.store([summary("cached", "前回")])
        let library = MacAppLibrary(store: nil, server: nil, serverCache: saved)
        await library.refreshServerApps()
        XCTAssertTrue(library.createdIds.isEmpty, "モックは今まで通り端末のアプリだけ")
        XCTAssertEqual(library.serverListState, .idle)
    }

    func testConcurrentRefreshesShareOneRequest() async throws {
        let server = FakeAppConfig()
        await server.setDelay(.milliseconds(100))
        await server.set(apps: [summary("a", "A")])
        let library = MacAppLibrary(store: nil, server: server)
        async let first: Void = library.refreshServerApps()
        async let second: Void = library.refreshServerApps()
        _ = await (first, second)
        let calls = await server.listCalls
        XCTAssertEqual(calls, 1, "前面に戻る・ウインドウごとに頼まれても 1 回にまとめる")
    }

    // MARK: - 開く・消す・名前・並べ替え

    func testDownloadOpensServerApp() async throws {
        let server = FakeAppConfig()
        await server.set(apps: [summary("a", "A")])
        await server.set(doc: try document("a", "サーバーのアプリ"))
        let library = MacAppLibrary(store: nil, server: server, serverEnvelope: { _ in nil })
        await library.refreshServerApps()

        await library.download("a")
        XCTAssertFalse(library.isServerOnly("a"))
        XCTAssertEqual(library.generated["a"]?.dsl.app.name, "サーバーのアプリ")
        XCTAssertTrue(library.isCreated("a"))
    }

    func testDownloadFailureCanRetry() async throws {
        let server = FakeAppConfig()
        await server.set(apps: [summary("a", "A")])
        let library = MacAppLibrary(store: nil, server: server)
        await library.refreshServerApps()

        await library.download("a")
        XCTAssertTrue(library.downloadFailed.contains("a"))
        await server.set(doc: try document("a", "A"))
        await library.download("a")
        XCTAssertFalse(library.downloadFailed.contains("a"))
        XCTAssertNotNil(library.generated["a"])
    }

    func testDeleteRemovesServerRowUntilNextRefresh() async throws {
        let server = FakeAppConfig()
        await server.set(apps: [summary("a", "A"), summary("b", "B")])
        let library = MacAppLibrary(store: nil, server: server)
        await library.refreshServerApps()

        library.delete("a")
        XCTAssertFalse(library.createdIds.contains("a"))
        XCTAssertNil(library.serverSummary("a"), "取り直すまでに戻さない")
    }

    func testRenameServerOnlyAppIsOptimisticAndSentToServer() async throws {
        let server = FakeAppConfig()
        await server.set(apps: [summary("a", "A")])
        let library = MacAppLibrary(store: nil, server: server)
        await library.refreshServerApps()

        XCTAssertTrue(library.rename("a", to: "新しい名前"))
        XCTAssertEqual(library.serverSummary("a")?.name, "新しい名前", "先に一覧を変える")
        var renamed: String?
        for _ in 0..<50 {
            renamed = await server.renamed["a"]
            if renamed != nil { break }
            try await Task.sleep(for: .milliseconds(10))
        }
        XCTAssertEqual(renamed, "新しい名前")
    }

    func testMoveSendsServerOrder() async throws {
        let server = FakeAppConfig()
        await server.set(apps: [summary("a", "A", sort: 1), summary("b", "B", sort: 2)])
        let library = MacAppLibrary(store: nil, server: server)
        await library.refreshServerApps()

        library.move("b", to: 0)
        XCTAssertEqual(library.createdIds, ["b", "a"])
        XCTAssertEqual(library.serverApps.map(\.id), ["b", "a"], "取り直しで戻らないようサーバーの一覧も並べ替える")
        var orders: [[String]] = []
        for _ in 0..<50 {
            orders = await server.orders
            if !orders.isEmpty { break }
            try await Task.sleep(for: .milliseconds(10))
        }
        XCTAssertEqual(orders.last, ["b", "a"])
    }

    // MARK: - サインアウト・アカウントの切り替え

    func testWipeClearsServerListAndDropsInFlightRefresh() async throws {
        let server = FakeAppConfig()
        await server.set(apps: [summary("a", "A")])
        await server.setDelay(.milliseconds(150))
        let library = MacAppLibrary(store: nil, server: server)

        let refresh = Task { await library.refreshServerApps() }
        try await Task.sleep(for: .milliseconds(20))
        library.resetAfterWipe()
        await refresh.value

        XCTAssertTrue(library.serverApps.isEmpty, "前のアカウントの一覧を戻さない")
        XCTAssertTrue(library.createdIds.isEmpty)
        XCTAssertEqual(library.serverListState, .idle)
    }

    // MARK: - 端末の一覧の置き場(サーバーの一覧のキャッシュと分ける)

    func testLocalListMigratesFromLegacyOnce() throws {
        let legacy = cache("legacy")
        let local = cache("local")
        let documents = DocumentCache(directory: root.appendingPathComponent("dsl", isDirectory: true))
        let suite = "mac1057-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        let envelope = try XCTUnwrap(DocumentEnvelope.make(appId: "mac-1", version: 2, dsl: try Data(contentsOf: url)))
        documents.store(appId: "mac-1", envelope: envelope)
        legacy.store([summary("mac-1", "端末のアプリ")])

        let store = LocalAppLibrary(list: local, documents: documents, defaults: defaults, legacyList: legacy)
        XCTAssertEqual(store.load().map(\.summary.id), ["mac-1"], "前の版の一覧を移す")
        XCTAssertTrue(local.exists)

        local.store([])
        legacy.store([summary("mac-1", "サーバーの一覧")]) // 以後はサーバーの一覧のキャッシュ
        XCTAssertTrue(store.load().isEmpty, "移した後はサーバーの一覧を端末の一覧と取り違えない")
    }
}
