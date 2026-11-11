import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

private let glanceRows: [String: [Record]] = ["expense": [
    Record(id: "a", data: ["amount": .number(38000), "memo": .string("家賃")], createdAt: "2026-11-01T00:00:00Z"),
    Record(id: "b", data: ["amount": .number(200), "memo": .string("コーヒー")], createdAt: "2026-11-02T00:00:00Z"),
]]

/// link-10: アプリの一覧のひと目の数字(既定の選び方・書式・端末の中だけで数える・消す)。
@MainActor
final class AppGlanceTests: XCTestCase {
    private func dsl() throws -> AppDSL {
        let json = """
        {"schemaVersion":1,"app":{"name":"家計簿","tabs":[{"screen":"home","title":"ホーム"},{"screen":"log","title":"記録"}]},
         "initialScreen":"home",
         "entities":[{"id":"expense","name":"支出","titleField":"memo","fields":[
           {"id":"amount","name":"金額","type":"currency","unit":"円"},
           {"id":"memo","name":"メモ","type":"string","encrypted":true}]}],
         "screens":[
           {"id":"log","title":"記録","components":[
             {"id":"memo_latest","type":"stat","label":"最後のメモ","entity":"expense","aggregate":"latest","field":"memo"}]},
           {"id":"home","title":"家計簿","components":[
             {"id":"total","type":"stat","label":"合計","entity":"expense","aggregate":"sum","field":"amount","format":"currency"},
             {"id":"times","type":"stat","label":"回数","entity":"expense","aggregate":"count"}]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private var records: [String: [Record]] { glanceRows }

    private func freshDefaults() -> UserDefaults {
        let name = "AppGlanceTests.\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        d.removePersistentDomain(forName: name)
        return d
    }

    // MARK: - 選び方

    func testDefaultIsFirstStatOfInitialScreen() throws {
        let d = try dsl()
        XCTAssertEqual(AppGlance.choices(in: d).map(\.id), ["total", "times", "memo_latest"], "初期画面 → タブ順")
        XCTAssertEqual(AppGlance.chosen(in: d, setting: nil)?.id, "total")
        XCTAssertEqual(AppGlance.chosen(in: d, setting: "")?.id, "total")
        XCTAssertEqual(AppGlance.chosen(in: d, setting: "gone")?.id, "total", "DSL に無い id は既定に戻す")
        XCTAssertEqual(AppGlance.chosen(in: d, setting: "times")?.id, "times")
        XCTAssertNil(AppGlance.chosen(in: d, setting: AppGlance.hidden))
    }

    func testNoStatShowsNothing() throws {
        let d = try JSONDecoder().decode(AppDSL.self, from: Data(#"""
        {"schemaVersion":1,"app":{"name":"メモ"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"メモ","components":[{"id":"t","type":"text","text":"こんにちは"}]}]}
        """#.utf8))
        XCTAssertNil(AppGlance.compute(dsl: d, records: [:], setting: nil))
    }

    // MARK: - 書式

    func testValueUsesWidgetFormatting() throws {
        let d = try dsl()
        let v = try XCTUnwrap(AppGlance.compute(dsl: d, records: records, setting: nil))
        XCTAssertEqual(v.statId, "total")
        XCTAssertEqual(v.label, "合計")
        let expr = ExprEvaluator(dsl: d, records: { glanceRows[$0] ?? [] })
        XCTAssertEqual(v.value, WidgetSnapshotWriter.statText(AppGlance.chosen(in: d, setting: nil)!, dsl: d, records: records, evaluator: expr))
        XCTAssertEqual(AppGlance.line(v), "合計 \(v.value)")
        XCTAssertEqual(AppGlance.compute(dsl: d, records: records, setting: "times")?.value, WidgetSnapshotWriter.formattedValue(
            AppGlance.chosen(in: d, setting: "times")!, entity: d.entity("expense"), value: 2))
        XCTAssertNil(AppGlance.compute(dsl: d, records: [:], setting: nil), "記録が未取得なら出さない")
    }

    func testAccessibilityLabelReadsNameLabelValue() {
        let v = AppGlance.Value(statId: "total", label: "今月", value: "¥38,200", at: .now)
        XCTAssertEqual(AppGlance.accessibilityLabel(appName: "家計簿", value: v), "家計簿, 今月, ¥38,200")
        XCTAssertEqual(AppGlance.accessibilityLabel(appName: "家計簿", value: nil), "家計簿")
    }

    func testEncryptedFieldIsLockedWithoutKey() throws {
        let d = try dsl()
        XCTAssertEqual(AppGlance.compute(dsl: d, records: records, setting: "memo_latest", canDecrypt: false)?.value, "🔒")
        XCTAssertEqual(AppGlance.compute(dsl: d, records: records, setting: "memo_latest", canDecrypt: true)?.value, "コーヒー")
    }

    // MARK: - 端末の中だけで数える

    func testLoadReadsOnlyLocalCacheAndOnlyNeededEntity() async throws {
        let d = try dsl()
        let store = AppGlanceStore(defaults: freshDefaults())
        let log = ReadLog()
        let rows = glanceRows
        let source = AppGlanceSource(document: { $0 == "kakeibo" ? d : nil },
                                     cached: { appId, entity in log.reads.append("\(appId)/\(entity)"); return rows[entity] ?? [] })
        await store.load(appId: "kakeibo", setting: nil, source: source)
        XCTAssertEqual(store.values["kakeibo"]?.statId, "total")
        XCTAssertEqual(log.reads, ["kakeibo/expense"])
        XCTAssertEqual(store.choices["kakeibo"]?.map(\.id), ["total", "times", "memo_latest"])

        // 少しの間は読み直さない(一覧の出し入れでローカル DB を読み続けない)
        await store.load(appId: "kakeibo", setting: nil, source: source)
        XCTAssertEqual(log.reads.count, 1)

        // 端末に DSL の無いアプリは何も読まない
        await store.load(appId: "other", setting: nil, source: source)
        XCTAssertNil(store.values["other"])
        XCTAssertEqual(log.reads.count, 1)
    }

    func testEmptyLocalRecordsKeepPreviousValue() async throws {
        let d = try dsl()
        let store = AppGlanceStore(defaults: freshDefaults())
        let full = AppGlanceSource(document: { _ in d }, cached: { _, e in glanceRows[e] ?? [] })
        let empty = AppGlanceSource(document: { _ in d }, cached: { _, _ in [] })
        await store.load(appId: "a", setting: nil, source: empty)
        XCTAssertNil(store.values["a"], "この端末で開いていないアプリを「0」と言い切らない")
        await store.load(appId: "b", setting: nil, source: full)
        let before = store.values["b"]
        store.invalidate("b")
        await store.load(appId: "b", setting: nil, source: empty)
        XCTAssertEqual(store.values["b"], before)
    }

    func testRuntimeLoadGlanceNeverCallsRecordsAPI() async throws {
        let client = CountingClient()
        var services = BuildConfig.make(environment: .mock)
        services.appConfig = client
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 3), services: services)
        let store = AppGlanceStore(defaults: freshDefaults())
        let before = await client.lists
        await rt.loadGlance(for: AppSummary(id: "not-cached-\(UUID().uuidString)", name: "x", currentVersion: 2), store: store)
        await rt.loadGlance(for: AppSummary(id: rt.document.appId, name: "家計簿", currentVersion: 3), store: store)
        let after = await client.lists
        XCTAssertEqual(after, before, "一覧の数字のために記録の API を呼ばない")
    }

    // MARK: - 保存・消す

    func testSetGlanceStatSavesToServerAndRollsBackOnFailure() async throws {
        var services = BuildConfig.make(environment: .mock)
        let mem = try XCTUnwrap(services.appConfig as? InMemoryAppConfigClient)
        let app = AppSummary(id: "a", name: "家計簿", currentVersion: 3)
        await mem.setApps([app])
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 3), services: services)
        rt.apps = [app]
        let store = AppGlanceStore(defaults: freshDefaults())
        await rt.setGlanceStat(app, to: "times", store: store)
        XCTAssertEqual(rt.apps[0].glanceStat, "times")
        let saved = try await mem.listApps()
        XCTAssertEqual(saved.first?.glanceStat, "times")

        services.appConfig = CountingClient()
        let failing = AppRuntime(document: try DSLExamples.document("kakeibo", version: 3), services: services)
        failing.apps = [app]
        await failing.setGlanceStat(app, to: AppGlance.hidden, store: store)
        XCTAssertNil(failing.apps[0].glanceStat, "保存に失敗したら戻す")
        XCTAssertNotNil(failing.toast)
    }

    func testClearWipesValuesAndSavedCopy() async throws {
        let defaults = freshDefaults()
        let d = try dsl()
        let store = AppGlanceStore(defaults: defaults)
        await store.load(appId: "a", setting: nil, source: AppGlanceSource(document: { _ in d }, cached: { _, e in glanceRows[e] ?? [] }))
        XCTAssertNotNil(store.values["a"])
        XCTAssertNotNil(AppGlanceStore(defaults: defaults).values["a"], "次に開いた時はすぐ出す")
        store.clear()
        XCTAssertTrue(store.values.isEmpty)
        XCTAssertTrue(store.choices.isEmpty)
        XCTAssertNil(defaults.data(forKey: AppGlanceStore.defaultsKey))
        XCTAssertTrue(AppGlanceStore(defaults: defaults).values.isEmpty, "サインアウト後に前の人の数字が残らない")
    }

    private final class ReadLog: @unchecked Sendable { var reads: [String] = [] }

    /// 記録の取得を数えるだけのクライアント(書き換えは失敗)。
    private actor CountingClient: AppConfigClient {
        private(set) var lists = 0
        struct Boom: Error {}
        func fetchDocument(appId: String) async throws -> AppDocument { try DSLExamples.document(DSLExamples.initial) }
        func listVersions(appId: String) async throws -> [VersionSummary] { [] }
        func restore(appId: String, toVersion version: Int) async throws -> AppDocument { throw Boom() }
        func list(appId: String, entity: String) async throws -> [Record] { lists += 1; return [] }
        func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { throw Boom() }
        func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { throw Boom() }
        func delete(appId: String, entity: String, id: String) async throws {}
        func setAppGlance(appId: String, glanceStat: String) async throws -> AppSummary { throw Boom() }
    }
}
