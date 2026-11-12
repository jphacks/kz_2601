import XCTest
import ServiceManagement
import MetamonDSL
import MetamonServices
@testable import Metamon

/// メニューバーの常駐(#1848): 設定の既定・選んだアプリの保存・stat の集計。
@MainActor
final class MacMenuBarTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite: String!

    override func setUp() async throws {
        suite = "MacMenuBarTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() async throws {
        defaults.removePersistentDomain(forName: suite)
    }

    private func kakeibo() throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return try AppDSL.load(contentsOf: url)
    }

    private func expense(_ id: String, amount: Double, date: Date = .now) -> Record {
        let day = FieldCoding.dateFormatter.string(from: date)
        return Record(id: id, data: ["amount": .number(amount), "date": .string(day)])
    }

    // MARK: - 設定

    func testSettingsDefaultOff() {
        let settings = MacMenuBarSettings(defaults: defaults)
        XCTAssertFalse(settings.isEnabled, "メニューバーは既定で出さない")
        XCTAssertNil(settings.selectedAppId)
        settings.isEnabled = true
        XCTAssertTrue(defaults.bool(forKey: MacMenuBarSettings.enabledKey))
    }

    func testLoginItemDefaultOffAndToggles() {
        let service = FakeLoginItemService()
        let item = MacLoginItem(service: service)
        XCTAssertFalse(item.isEnabled, "ログイン時に開くは既定で OFF")
        XCTAssertEqual(service.registered, 0, "自分からは登録しない")
        item.setEnabled(true)
        XCTAssertTrue(item.isEnabled)
        XCTAssertEqual(service.registered, 1)
        item.setEnabled(false)
        XCTAssertFalse(item.isEnabled)
        XCTAssertEqual(service.unregistered, 1)
    }

    func testLoginItemFailureAndApproval() {
        let service = FakeLoginItemService()
        service.fails = true
        let item = MacLoginItem(service: service)
        item.setEnabled(true)
        XCTAssertTrue(item.failed)
        XCTAssertFalse(item.isEnabled)
        service.fails = false
        service.status = .requiresApproval
        item.refresh()
        XCTAssertTrue(item.needsApproval)
    }

    // MARK: - 選んだアプリ

    func testSelectionPersistsAcrossModels() async throws {
        let source = FakeMenuBarSource(dsl: try kakeibo())
        let settings = MacMenuBarSettings(defaults: defaults)
        let first = MacMenuBarModel(source: source, settings: settings)
        await first.reload()
        XCTAssertEqual(first.selectedId, "a", "未選択なら先頭")
        await first.select("b")
        XCTAssertEqual(settings.selectedAppId, "b")

        let second = MacMenuBarModel(source: source, settings: settings)
        await second.reload()
        XCTAssertEqual(second.selectedId, "b", "次に開いた時も同じアプリ")
    }

    func testSelectionFallsBackWhenAppIsGone() async throws {
        let source = FakeMenuBarSource(dsl: try kakeibo())
        let settings = MacMenuBarSettings(defaults: defaults)
        settings.selectedAppId = "deleted"
        let model = MacMenuBarModel(source: source, settings: settings)
        await model.reload()
        XCTAssertEqual(model.selectedId, "a")
        await model.select("unknown")
        XCTAssertEqual(model.selectedId, "a", "一覧に無いアプリは選ばない")
    }

    // MARK: - stat

    func testStatsUseFirstStatComponents() async throws {
        let source = FakeMenuBarSource(dsl: try kakeibo())
        source.records["expense"] = [expense("1", amount: 1200), expense("2", amount: 300),
                                     expense("old", amount: 9999, date: Date(timeIntervalSinceNow: -86400 * 400))]
        let model = MacMenuBarModel(source: source, settings: MacMenuBarSettings(defaults: defaults))
        await model.reload()
        XCTAssertEqual(model.stats.map(\.label), ["今月の支出", "今月の件数"])
        XCTAssertEqual(model.stats.first?.value, FieldCoding.currency(1500), "今月の分だけ足す(stat と同じ集計)")
        XCTAssertTrue(model.stats.last?.value.hasPrefix("2") == true)
    }

    func testStatsRefreshWhenRecordAdded() async throws {
        let source = FakeMenuBarSource(dsl: try kakeibo())
        source.records["expense"] = [expense("1", amount: 100)]
        let model = MacMenuBarModel(source: source, settings: MacMenuBarSettings(defaults: defaults))
        await model.reload()
        XCTAssertEqual(model.stats.first?.value, FieldCoding.currency(100))
        source.records["expense"]?.append(expense("2", amount: 50))
        await model.recordsChanged(appId: "b")
        XCTAssertEqual(model.stats.first?.value, FieldCoding.currency(100), "選んでいないアプリの記録では数え直さない")
        await model.recordsChanged(appId: "a")
        XCTAssertEqual(model.stats.first?.value, FieldCoding.currency(150))
    }

    func testStatsWithoutRecordsShowDash() throws {
        let stats = MacMenuBarStats.make(dsl: try kakeibo(), records: [:])
        XCTAssertEqual(stats.count, 2)
        XCTAssertTrue(stats.allSatisfy { $0.value == "—" }, "記録をまだ読めていない")
        XCTAssertEqual(MacMenuBarStats.entities(in: try kakeibo()), ["expense"])
    }

    func testNoAppsShowsNothing() async {
        let source = FakeMenuBarSource(dsl: nil)
        source.apps = []
        let model = MacMenuBarModel(source: source, settings: MacMenuBarSettings(defaults: defaults))
        await model.reload()
        XCTAssertNil(model.selectedId)
        XCTAssertTrue(model.stats.isEmpty)
    }
}

// MARK: - 偽物

@MainActor
private final class FakeMenuBarSource: MacMenuBarSource {
    var apps = [AppSummary(id: "a", name: "家計簿", currentVersion: 1), AppSummary(id: "b", name: "読書", currentVersion: 1)]
    var records: [String: [Record]] = [:]
    let dsl: AppDSL?

    init(dsl: AppDSL?) { self.dsl = dsl }

    func menuBarApps() async -> [AppSummary] { apps }
    func menuBarDocument(appId: String) async -> AppDocument? {
        dsl.map { AppDocument(appId: appId, version: 1, dsl: $0) }
    }
    func menuBarRecords(appId: String, entity: String) async -> [Record] { records[entity] ?? [] }
}

@MainActor
private final class FakeLoginItemService: MacLoginItemService {
    var status: SMAppService.Status = .notRegistered
    var fails = false
    var registered = 0
    var unregistered = 0

    struct Failure: Error {}

    func register() throws {
        if fails { throw Failure() }
        registered += 1
        status = .enabled
    }

    func unregister() throws {
        if fails { throw Failure() }
        unregistered += 1
        status = .notRegistered
    }
}
