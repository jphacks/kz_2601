import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// コントロール(iOS 18・#1993)を押した時の本体の動き・文言。
@MainActor
final class ControlIntentsTests: XCTestCase {
    private let json = """
    {"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home",
     "entities":[{"id":"expense","name":"支出","fields":[{"id":"amount","name":"金額","type":"number"}]},
                 {"id":"memo","name":"メモ","fields":[{"id":"text","name":"内容","type":"string"}]}],
     "screens":[{"id":"home","title":"家計簿","components":[
                  {"type":"list","entity":"expense","item":{"title":"amount"}},
                  {"type":"button","label":"追加","action":{"type":"navigate","screen":"add","presentation":"sheet"}}]},
                {"id":"add","title":"支出を追加","entity":"expense","components":[{"type":"numberField","field":"amount"}]}]}
    """

    private func runtime() throws -> AppRuntime {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let services = BuildConfig.make(environment: .mock)
        return AppRuntime(document: AppDocument(appId: "kakeibo-\(UUID().uuidString.prefix(6))", version: 1, dsl: dsl), services: services)
    }

    func testFormScreenIsTheEntitysForm() throws {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        XCTAssertEqual(AppRuntime.formScreen(for: "expense", in: dsl), "add")
        XCTAssertNil(AppRuntime.formScreen(for: "memo", in: dsl)) // フォーム画面の無い entity はアプリを開くだけ
    }

    func testRecordControlOpensTheForm() async throws {
        let rt = try runtime()
        await rt.openFromControl(appId: rt.document.appId, entityId: "expense")
        XCTAssertEqual(NavigationResolver.top(rt.navigationState, dsl: rt.dsl).screen, "add")
        XCTAssertFalse(rt.showAppSwitcher)
    }

    func testOpenAppControlOnCurrentAppStaysOnRoot() async throws {
        let rt = try runtime()
        await rt.openFromControl(appId: rt.document.appId, entityId: nil)
        XCTAssertNil(rt.sheet)
        XCTAssertEqual(NavigationResolver.top(rt.navigationState, dsl: rt.dsl).screen, "home")
    }

    /// 未選択・消したアプリは一覧を開く(落ちない)
    func testMissingAppOpensSwitcher() async throws {
        let rt = try runtime()
        await rt.openFromControl(appId: nil, entityId: "expense")
        let opened = expectation(description: "switcher")
        Task {
            for _ in 0..<50 where !rt.showAppSwitcher { try? await Task.sleep(for: .milliseconds(20)) }
            opened.fulfill()
        }
        await fulfillment(of: [opened], timeout: 3)
        XCTAssertTrue(rt.showAppSwitcher)
        XCTAssertNil(rt.sheet)
    }

    func testIntentsCarryIds() {
        XCTAssertEqual(OpenAppFromControlIntent(appId: "a1").appId, "a1")
        let record = OpenRecordFromControlIntent(appId: "a1", entityId: "expense")
        XCTAssertEqual(record.appId, "a1")
        XCTAssertEqual(record.entityId, "expense")
        XCTAssertTrue(OpenAppFromControlIntent.openAppWhenRun)
        XCTAssertTrue(OpenRecordFromControlIntent.openAppWhenRun)
    }

    func testCopyWording() {
        // 端末の言語に左右されないよう、訳す前のキー(ソースの日本語)で見る
        #if APP_STORE_REVIEW
        XCTAssertEqual(OpenAppFromControlIntent.title.key, "記録帳を開く")
        XCTAssertFalse(ActionButtonHintSection.hint.contains("作った"))
        #else
        XCTAssertEqual(OpenAppFromControlIntent.title.key, "アプリを開く")
        #endif
    }

    func testCopyHasEnglish() throws {
        let path = try XCTUnwrap(Bundle(for: AppRuntime.self).path(forResource: "en", ofType: "lproj"))
        let en = try XCTUnwrap(Bundle(path: path))
        let keys = [
            OpenAppFromControlIntent.title.key, OpenRecordFromControlIntent.title.key, "アクションボタン",
            "アクションボタンのある iPhone では、設定アプリの「アクションボタン」で「ショートカット」を選び、Tailor It の「アプリを開く」を割り当てると、押すだけで作ったアプリを開けます。",
            "アクションボタンのある iPhone では、設定アプリの「アクションボタン」で「ショートカット」を選び、Tailor It の「記録帳を開く」を割り当てると、押すだけで記録帳を開けます。",
        ]
        for key in keys {
            let value = en.localizedString(forKey: key, value: "__missing__", table: nil)
            XCTAssertNotEqual(value, "__missing__", "英語が無い: \(key)")
            XCTAssertNotEqual(value, key, "英語が日本語のまま: \(key)")
        }
    }

    // MARK: - コントロールの一覧(#1993 レビュー)

    /// ログアウト・アカウント削除で、待っている引き直しを止める(前の人の一覧を書き戻さない)
    func testWipeCancelsPendingCatalogRefresh() async throws {
        let rt = try runtime()
        let pending = IntentHost.shared.appsMayHaveChanged()
        XCTAssertTrue(IntentHost.shared.hasPendingRefresh)
        await rt.wipeLocalData()
        XCTAssertTrue(pending.isCancelled)
        IntentHost.shared.cancelPendingRefresh()
    }

    /// 集めている間に取り消されたら書かない
    func testRefreshCatalogSkipsWriteWhenCancelledWhileCollecting() async throws {
        let suite = "control-catalog-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let apps = [AppSummary(id: "a1", name: "家計簿", currentVersion: 3)]

        let cancelled = Task { @MainActor in
            await ControlIntentBridge.refreshCatalog(
                apps: { apps },
                targets: { withUnsafeCurrentTask { $0?.cancel() }; return [] },
                defaults: defaults)
        }
        let wroteWhenCancelled = await cancelled.value
        XCTAssertFalse(wroteWhenCancelled)
        XCTAssertEqual(ControlCatalog.load(from: defaults), ControlCatalog())

        let wrote = await ControlIntentBridge.refreshCatalog(apps: { apps }, targets: { [] }, defaults: defaults)
        XCTAssertTrue(wrote)
        XCTAssertEqual(ControlCatalog.load(from: defaults).app(id: "a1")?.name, "家計簿")
        let again = await ControlIntentBridge.refreshCatalog(apps: { apps }, targets: { [] }, defaults: defaults)
        XCTAssertFalse(again, "変わっていなければ書かない(コントロールを読み直さない)")
    }

    /// コントロールの一覧はキャッシュの DSL だけで作る(`GET /dsl` を呼ばない)
    func testCatalogTargetsDoNotFetchDocuments() async throws {
        let spy = CountingAppConfig(apps: [
            AppSummary(id: "ctl-\(UUID().uuidString)", name: "家計簿", currentVersion: 3),
            AppSummary(id: "ctl-\(UUID().uuidString)", name: "日記", currentVersion: 3),
        ])
        let doc = try runtime().document
        var services = BuildConfig.make(environment: .mock)
        services.appConfig = spy
        let rt = AppRuntime(document: doc, services: services)
        let host = IntentHost()
        host.register(rt)

        _ = await RecordTargetEntity.all(host: host, cachedOnly: true)
        XCTAssertEqual(spy.fetches, 0)
        _ = await RecordTargetEntity.all(host: host) // Siri の候補は従来どおり取りに行く
        XCTAssertEqual(spy.fetches, 2)
    }
}

/// `GET /dsl` の回数を数える appconfig
private final class CountingAppConfig: AppConfigClient, @unchecked Sendable {
    private let lock = NSLock()
    private var count = 0
    private let apps: [AppSummary]
    init(apps: [AppSummary]) { self.apps = apps }
    var fetches: Int { lock.withLock { count } }

    func listApps() async throws -> [AppSummary] { apps }
    func fetchDocument(appId: String) async throws -> AppDocument {
        lock.withLock { count += 1 }
        throw APIError.server(status: 404, code: "not_found", message: "spy")
    }
    func listVersions(appId: String) async throws -> [VersionSummary] { [] }
    func restore(appId: String, toVersion version: Int) async throws -> AppDocument {
        throw APIError.server(status: 404, code: "not_found", message: "spy")
    }
    func list(appId: String, entity: String) async throws -> [Record] { [] }
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        throw APIError.server(status: 501, code: "unsupported", message: "spy")
    }
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        throw APIError.server(status: 501, code: "unsupported", message: "spy")
    }
    func delete(appId: String, entity: String, id: String) async throws {}
}
