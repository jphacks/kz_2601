import XCTest
import CoreSpotlight
import MetamonDSL
import MetamonServices
@testable import Metamon

/// Spotlight の索引(#1996): 一覧と合わせる・消す・ログアウトで全部消す。
@MainActor
final class SpotlightIndexTests: XCTestCase {
    private final class FakeIndex: SpotlightIndex {
        var items: [String: SpotlightAppItem] = [:]
        var indexCalls: [[String]] = []
        var removeCalls: [[String]] = []
        var removeAllCount = 0
        /// 索引に入れている途中に挟む(サインアウトの割り込み)
        var onIndex: (() async -> Void)?

        func index(apps: [SpotlightAppItem]) async {
            indexCalls.append(apps.map(\.id))
            for app in apps { items[app.id] = app }
            if let hook = onIndex { onIndex = nil; await hook() }
        }
        func remove(appIds: [String]) async {
            removeCalls.append(appIds)
            for id in appIds { items[id] = nil }
        }
        func removeAll() async {
            removeAllCount += 1
            items = [:]
        }
    }

    private var suite = ""
    private var defaults: UserDefaults!

    override func setUp() {
        suite = "spotlight-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() { defaults.removePersistentDomain(forName: suite) }

    private func indexer(_ fake: FakeIndex) -> SpotlightAppIndexer {
        SpotlightAppIndexer(index: fake, defaults: defaults, icon: { $0 == "kakeibo" ? "yensign.circle" : nil })
    }

    private func app(_ id: String, _ name: String, version: Int = 3, status: String = "active") -> AppSummary {
        AppSummary(id: id, name: name, currentVersion: version, status: status)
    }

    // MARK: - 一覧と合わせる

    func testSyncIndexesNamesAndIconsAndSkipsBlankAndArchived() async {
        let fake = FakeIndex()
        await indexer(fake).sync([app("kakeibo", "家計簿"), app("blank", "", version: 1), app("old", "古い", status: "archived")])

        XCTAssertEqual(fake.items, ["kakeibo": SpotlightAppItem(id: "kakeibo", name: "家計簿", icon: "yensign.circle")])
    }

    func testSyncRemovesIdsNotInListAndOnlyReindexesChanges() async {
        let fake = FakeIndex()
        let sut = indexer(fake)
        await sut.sync([app("a", "家計簿"), app("b", "筋トレ")])
        fake.indexCalls = []

        await sut.sync([app("a", "家計簿"), app("b", "筋トレ")])
        XCTAssertEqual(fake.indexCalls, [], "変わっていなければ入れ直さない")

        await sut.sync([app("a", "おこづかい帳"), app("c", "読書")])
        XCTAssertEqual(fake.removeCalls, [["b"]], "一覧に無い id を消す")
        XCTAssertEqual(fake.indexCalls.flatMap { $0 }.sorted(), ["a", "c"])
        XCTAssertEqual(fake.items["a"]?.name, "おこづかい帳")
        XCTAssertEqual(Set(fake.items.keys), ["a", "c"])
    }

    func testSyncReindexesWhenAppWasRegenerated() async {
        let fake = FakeIndex()
        let sut = indexer(fake)
        await sut.sync([app("a", "家計簿", version: 3)])
        fake.indexCalls = []
        await sut.sync([app("a", "家計簿", version: 4)])
        XCTAssertEqual(fake.indexCalls, [["a"]], "作り直すとアイコンが変わり得る")
    }

    // MARK: - 消す

    func testDeletingAppRemovesItFromIndex() async throws {
        let fake = FakeIndex()
        var services = BuildConfig.make(environment: .mock)
        let client = InMemoryAppConfigClient()
        services.appConfig = client
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: services)
        rt.spotlight = indexer(fake)
        let target = app("other", "別アプリ")
        await client.setApps([target, app("keep", "残す")])
        await rt.appsList()
        XCTAssertEqual(Set(fake.items.keys), ["other", "keep"], "一覧を読み直すと索引に入る")

        let deleted = await rt.deleteApp(target)

        XCTAssertTrue(deleted)
        XCTAssertEqual(fake.removeCalls.last, ["other"])
        XCTAssertEqual(Set(fake.items.keys), ["keep"])

        // 覚えた id からも消えている(次の一覧で消し直さない)
        fake.removeCalls = []
        await rt.appsList()
        XCTAssertEqual(fake.removeCalls, [])
    }

    func testSignOutWipeRemovesEverything() async throws {
        let fake = FakeIndex()
        let services = BuildConfig.make(environment: .mock)
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: services)
        rt.spotlight = indexer(fake)
        await rt.spotlight.sync([app("a", "前の人のアプリ")])
        XCTAssertFalse(fake.items.isEmpty)

        let session = AuthSession(service: services.auth)
        _ = try await session.signOutAndWipe(runtime: rt, choice: .sendThenWipe)

        XCTAssertEqual(fake.removeAllCount, 1)
        XCTAssertTrue(fake.items.isEmpty, "前の人のアプリ名を Spotlight に残さない")
        XCTAssertNil(defaults.object(forKey: SpotlightAppIndexer.indexedKey))
    }

    // MARK: - サインアウトとの競合

    func testSyncStartedBeforeWipeDoesNotReindex() async {
        let fake = FakeIndex()
        let sut = indexer(fake)
        let started = sut.generation // 一覧を取りに行く前
        await sut.removeAll()        // 取っている間にサインアウト

        await sut.sync([app("a", "前の人のアプリ")], since: started)

        XCTAssertEqual(fake.indexCalls, [], "前の人の一覧を入れ直さない")
        XCTAssertTrue(fake.items.isEmpty)
        XCTAssertNil(defaults.object(forKey: SpotlightAppIndexer.indexedKey))
    }

    func testWipeDuringIndexCallUndoesAndSkipsWrite() async {
        let fake = FakeIndex()
        let sut = indexer(fake)
        fake.onIndex = { await sut.removeAll() } // 索引に入れている最中にサインアウト

        await sut.sync([app("a", "前の人のアプリ")])

        XCTAssertTrue(fake.items.isEmpty, "入ってしまった分は消し直す")
        XCTAssertNil(defaults.object(forKey: SpotlightAppIndexer.indexedKey), "覚えた id を書き戻さない")

        // 次の人の同期は普通に入る
        await sut.sync([app("b", "次の人のアプリ")])
        XCTAssertEqual(Set(fake.items.keys), ["b"])
    }

    // MARK: - 作った時

    func testCreatingAppByGenerationIndexesIt() async throws {
        let fake = FakeIndex()
        var services = BuildConfig.make(environment: .mock)
        services.ai = MockAIClient(delay: .zero)
        var initial = try DSLExamples.document(DSLExamples.initial, version: 0)
        initial.appId = "new-\(UUID().uuidString.prefix(6))"
        let rt = AppRuntime(document: initial, services: services)
        rt.spotlight = indexer(fake)

        await rt.send("家計簿アプリを作って。", gatePaid: false)

        XCTAssertFalse(rt.dsl.isVoiceOnly, "生成でアプリができている")
        XCTAssertEqual(fake.indexCalls, [[initial.appId]], "一覧を取り直さずに作ったアプリを入れる")
        XCTAssertEqual(fake.items[initial.appId]?.name, rt.dsl.app.name)
        XCTAssertEqual(fake.items[initial.appId]?.icon, rt.dsl.app.icon)
    }

    func testEditingExistingAppDoesNotUpsert() async throws {
        let fake = FakeIndex()
        var services = BuildConfig.make(environment: .mock)
        services.ai = MockAIClient(delay: .zero)
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 3), services: services)
        rt.spotlight = indexer(fake)

        await rt.send("金額を大きくして", gatePaid: false)

        XCTAssertEqual(fake.indexCalls, [], "作り直し・名前の変更は一覧(appsList)の同期で拾う")
    }

    func testCreatedAppIsNotReindexedByNextListSync() async throws {
        let fake = FakeIndex()
        let sut = indexer(fake)
        await sut.upsert(app("a", "家計簿", version: 2), icon: "yensign.circle")
        XCTAssertEqual(fake.items["a"]?.icon, "yensign.circle")
        fake.indexCalls = []

        await sut.sync([app("a", "家計簿", version: 2)])

        XCTAssertEqual(fake.indexCalls, [], "同じ版・名前なら一覧の同期で入れ直さない")
    }

    // MARK: - 開く

    func testActivityFromSpotlightResolvesAppId() {
        let activity = NSUserActivity(activityType: CSSearchableItemActionType)
        activity.userInfo = [CSSearchableItemActivityIdentifier: SpotlightID.app("abc-123")]
        XCTAssertEqual(SpotlightID.appId(from: activity), "abc-123")

        let other = NSUserActivity(activityType: CSSearchableItemActionType)
        other.userInfo = [CSSearchableItemActivityIdentifier: "records.abc/r1"]
        XCTAssertNil(SpotlightID.appId(from: other), "記録など別の項目は開かない")
    }

    func testSearchableItemUsesAppsDomain() {
        let item = CoreSpotlightIndex.searchableItem(SpotlightAppItem(id: "abc", name: "家計簿", icon: "yensign.circle"))
        XCTAssertEqual(item.uniqueIdentifier, "apps/abc")
        XCTAssertEqual(item.domainIdentifier, SpotlightID.appsDomain)
        XCTAssertEqual(item.attributeSet.title, "家計簿")
        XCTAssertNotNil(item.attributeSet.thumbnailData)
        XCTAssertEqual(SpotlightID.recordsDomain(appId: "abc"), "records.abc")
    }
}
