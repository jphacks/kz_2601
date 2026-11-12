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

        func index(apps: [SpotlightAppItem]) async {
            indexCalls.append(apps.map(\.id))
            for app in apps { items[app.id] = app }
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
