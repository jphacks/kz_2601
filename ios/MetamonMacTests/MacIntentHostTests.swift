import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// ショートカット・メニューバーの記録先に出すアプリ(`IntentHost.recordableApps`・#1034)。
/// 端末に残る「作ったアプリ」だけを返し、同梱の例(記録がメモリ上で消える)は外す。作り物の一覧で確かめる
@MainActor
final class MacIntentHostTests: XCTestCase {
    /// 一覧だけを返す appconfig(中身は取らない = サーバーにあって未取得のアプリになる)
    private actor ListOnly: AppConfigClient {
        let apps: [AppSummary]
        init(apps: [AppSummary]) { self.apps = apps }

        func listApps() async throws -> [AppSummary] { apps }
        func fetchDocument(appId: String) async throws -> AppDocument { throw URLError(.notConnectedToInternet) }
        func listVersions(appId: String) async throws -> [VersionSummary] { [] }
        func restore(appId: String, toVersion version: Int) async throws -> AppDocument { throw URLError(.badServerResponse) }
        func list(appId: String, entity: String) async throws -> [Record] { [] }
        func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { Record(id: "x", data: data) }
        func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { Record(id: id, data: data) }
        func delete(appId: String, entity: String, id: String) async throws {}
    }

    private let example = SampleApp(id: "example", name: "読書メモ", description: nil, url: nil)

    /// `fields` が空の entity は記録先にならない(フォームの無いアプリ)
    private func dsl(name: String, fields: Bool = true) throws -> AppDSL {
        let field = fields ? #"{"id":"amount","name":"金額","type":"number"}"# : ""
        let json = """
        {"schemaVersion":1,"app":{"name":"\(name)"},"initialScreen":"home",
         "entities":[{"id":"memo","name":"メモ","fields":[\(field)]}],
         "screens":[{"id":"home","title":"ホーム","entity":"memo","components":[]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private func host(library: MacAppLibrary?) -> IntentHost {
        let host = IntentHost(server: { nil })
        if let library { host.register(library: library, bundled: [example]) }
        return host
    }

    func testEmptyBeforeTheLibraryIsRegistered() async {
        let host = host(library: nil)
        let apps = await host.apps()
        let recordable = await host.recordableApps()
        XCTAssertTrue(apps.isEmpty)
        XCTAssertTrue(recordable.isEmpty, "一覧を渡す前は記録先も無い")
        XCTAssertFalse(host.canWriteRecords)
    }

    func testBundledExamplesAreListedButNotRecordable() async throws {
        let library = MacAppLibrary(store: nil)
        library.apply(AppDocument(appId: "mine", version: 3, dsl: try dsl(name: "家計簿")), created: true)
        let host = host(library: library)

        let apps = await host.apps()
        XCTAssertEqual(apps.map(\.id), ["mine", "example"], "一覧はサイドバーと同じ並び(作ったアプリ → 同梱の例)")
        let recordable = await host.recordableApps()
        XCTAssertEqual(recordable.map(\.id), ["mine"], "同梱の例は記録先にしない(記録がメモリ上で消える)")
        XCTAssertEqual(recordable.first?.name, "家計簿", "名前は手元の DSL から")
        XCTAssertEqual(recordable.first?.currentVersion, 3)
    }

    func testEditedExampleIsStillNotRecordable() async throws {
        let library = MacAppLibrary(store: nil)
        library.apply(AppDocument(appId: example.id, version: 2, dsl: try dsl(name: "読書メモ(直した)")), created: false)
        let host = host(library: library)

        let apps = await host.apps()
        XCTAssertEqual(apps.map(\.name), ["読書メモ(直した)"], "直した例は直した名前で一覧に出す")
        let recordable = await host.recordableApps()
        XCTAssertTrue(recordable.isEmpty, "直しても同梱の例は作ったアプリにならない")
    }

    func testServerAppWithoutItsDSLUsesTheServerSummary() async throws {
        let library = MacAppLibrary(store: nil, server: ListOnly(apps: [AppSummary(id: "srv", name: "買い物メモ", currentVersion: 5)]))
        library.apply(AppDocument(appId: "mine", version: 1, dsl: try dsl(name: "家計簿")), created: true)
        await library.refreshServerApps()
        let host = host(library: library)

        let recordable = await host.recordableApps()
        XCTAssertEqual(recordable.map(\.id), ["mine", "srv"], "この Mac だけのアプリはサーバーの一覧の上に残る")
        let server = try XCTUnwrap(recordable.last)
        XCTAssertEqual(server.name, "買い物メモ", "中身を取る前はサーバーの一覧の名前と版")
        XCTAssertEqual(server.currentVersion, 5)
        XCTAssertTrue(library.isServerOnly("srv"))
    }

    func testDeletedAppLeavesTheRecordTargets() async throws {
        let library = MacAppLibrary(store: nil)
        library.apply(AppDocument(appId: "mine", version: 1, dsl: try dsl(name: "家計簿")), created: true)
        library.apply(AppDocument(appId: "other", version: 1, dsl: try dsl(name: "日記")), created: true)
        let host = host(library: library)

        library.delete("mine")
        let recordable = await host.recordableApps()
        XCTAssertEqual(recordable.map(\.id), ["other"], "消したアプリは記録先から消える")
    }

    /// 記録先の候補は `recordableApps` を通る。記録できるフォーム(項目のある entity)の無いアプリは候補にならない
    func testRecordTargetsSkipAppsWithoutAForm() async throws {
        let library = MacAppLibrary(store: nil)
        library.apply(AppDocument(appId: "empty", version: 1, dsl: try dsl(name: "からっぽ", fields: false)), created: true)
        library.apply(AppDocument(appId: "mine", version: 1, dsl: try dsl(name: "家計簿")), created: true)
        let host = host(library: library)

        let recordable = await host.recordableApps()
        XCTAssertEqual(Set(recordable.map(\.id)), ["empty", "mine"])
        let targets = await RecordTargetEntity.all(host: host, cachedOnly: true)
        XCTAssertEqual(targets.map(\.appId), ["mine"], "項目のある entity が無いアプリは記録先に出さない")
        XCTAssertEqual(targets.map(\.entityId), ["memo"])
    }
}
