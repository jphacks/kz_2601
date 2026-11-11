import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// サイドバーの新着の数・未読の印と、共有されたアプリの区分(#969)
@MainActor
final class MacSidebarSectionsTests: XCTestCase {
    private struct FakeStamps: SharedRecordStampSource {
        var stamps: [String: [RecordStamp]] = [:]
        var failing: Set<String> = []
        func sharedStamps(appId: String, entity: String, notBy uid: String) async throws -> [RecordStamp] {
            if failing.contains(entity) { throw URLError(.notConnectedToInternet) }
            return stamps["\(appId)/\(entity)"] ?? []
        }
    }

    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        super.setUp()
        suite = "mac969-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        super.tearDown()
    }

    private let base = Date(timeIntervalSince1970: 1_800_000_000)
    private func at(_ offset: TimeInterval) -> String {
        ISO8601DateFormatter().string(from: base.addingTimeInterval(offset))
    }

    // MARK: - 数

    func testCountsOnlyOthersChangesSinceLastOpened() async {
        let store = LastOpenedStore(defaults: defaults)
        store.mark(appId: "a", at: base)
        let source = FakeStamps(stamps: ["a/memo": [
            RecordStamp(id: "1", author: "other", createdAt: at(10)),
            RecordStamp(id: "2", author: "other", createdAt: at(-10), updatedAt: at(20)),
            RecordStamp(id: "3", author: "other", createdAt: at(-10)), // 前回より前
            RecordStamp(id: "4", author: "me", createdAt: at(10)), // 自分
            RecordStamp(id: "5", author: nil, createdAt: at(10)), // private
        ], "a/todo": [RecordStamp(id: "6", author: "other2", createdAt: at(30))]])
        let badges = MacSidebarBadges()
        await badges.refresh([.init(appId: "a", entities: ["memo", "todo"])], openAppId: nil, uid: "me",
                             source: source, store: store)
        XCTAssertEqual(badges.count("a"), 3)
        // 数えるだけで基準は進めない(開いた時の `NewItemsCheck` が進める)
        XCTAssertEqual(store.lastOpened(appId: "a"), base)
    }

    func testNoBadgeWhenNeverOpenedOrNoEntitiesOrAllFailed() async {
        let store = LastOpenedStore(defaults: defaults)
        store.mark(appId: "b", at: base)
        store.mark(appId: "c", at: base)
        let stamp = [RecordStamp(id: "1", author: "other", createdAt: at(10))]
        let source = FakeStamps(stamps: ["a/memo": stamp, "b/memo": stamp], failing: ["broken"])
        let badges = MacSidebarBadges()
        await badges.refresh([.init(appId: "a", entities: ["memo"]), // 一度も開いていない
                              .init(appId: "b", entities: []), // entity が分からない
                              .init(appId: "c", entities: ["broken"])], // 取れない
                             openAppId: nil, uid: "me", source: source, store: store)
        XCTAssertEqual(badges.counts, [:])
    }

    func testOpenAppIsNotCountedAndOpeningClears() async {
        let store = LastOpenedStore(defaults: defaults)
        store.mark(appId: "a", at: base)
        store.mark(appId: "b", at: base)
        let stamp = [RecordStamp(id: "1", author: "other", createdAt: at(10))]
        let source = FakeStamps(stamps: ["a/memo": stamp, "b/memo": stamp])
        let badges = MacSidebarBadges()
        let targets: [MacSidebarBadges.Target] = [.init(appId: "a", entities: ["memo"]), .init(appId: "b", entities: ["memo"])]
        await badges.refresh(targets, openAppId: "a", uid: "me", source: source, store: store)
        XCTAssertEqual(badges.counts, ["b": 1])

        badges.markRead("b")
        XCTAssertEqual(badges.count("b"), 0)
    }

    func testSignedOutClearsBadges() async {
        let store = LastOpenedStore(defaults: defaults)
        store.mark(appId: "a", at: base)
        let source = FakeStamps(stamps: ["a/memo": [RecordStamp(id: "1", author: "other", createdAt: at(10))]])
        let badges = MacSidebarBadges()
        await badges.refresh([.init(appId: "a", entities: ["memo"])], openAppId: nil, uid: "me", source: source, store: store)
        XCTAssertEqual(badges.count("a"), 1)
        await badges.refresh([.init(appId: "a", entities: ["memo"])], openAppId: nil, uid: nil, source: source, store: store)
        XCTAssertEqual(badges.counts, [:])
    }

    private final class CountingStamps: SharedRecordStampSource, @unchecked Sendable {
        var calls: [String] = []
        let stamps: [RecordStamp]
        init(_ stamps: [RecordStamp]) { self.stamps = stamps }
        func sharedStamps(appId: String, entity: String, notBy uid: String) async throws -> [RecordStamp] {
            calls.append("\(appId)/\(entity)")
            return stamps
        }
    }

    /// 別のウインドウで開いているアプリも数えない。閉じたら数える
    func testAppOpenInAnyWindowIsNotCounted() async {
        let store = LastOpenedStore(defaults: defaults)
        store.mark(appId: "a", at: base)
        store.mark(appId: "b", at: base)
        let source = FakeStamps(stamps: ["a/memo": [RecordStamp(id: "1", author: "other", createdAt: at(10))],
                                         "b/memo": [RecordStamp(id: "2", author: "other", createdAt: at(10))]])
        let badges = MacSidebarBadges()
        let w1 = UUID(), w2 = UUID()
        badges.setOpen("a", window: w1)
        badges.setOpen("b", window: w2)
        let targets: [MacSidebarBadges.Target] = [.init(appId: "a", entities: ["memo"]), .init(appId: "b", entities: ["memo"])]
        await badges.refresh(targets, openAppId: nil, uid: "me", source: source, store: store)
        XCTAssertEqual(badges.counts, [:])

        badges.closeWindow(w2)
        await badges.refresh(targets, openAppId: nil, uid: "me", source: source, store: store)
        XCTAssertEqual(badges.counts, ["b": 1])
    }

    /// 前面に戻るたび・ウインドウごとに問い合わせない。一覧・アカウントが変わった時と、間隔が空いた時だけ
    func testRefreshIfDueThrottlesAcrossWindows() async {
        let store = LastOpenedStore(defaults: defaults)
        store.mark(appId: "a", at: base)
        store.mark(appId: "b", at: base)
        let source = CountingStamps([RecordStamp(id: "1", author: "other", createdAt: at(10))])
        var clock = base
        let badges = MacSidebarBadges(now: { clock })
        let targets: [MacSidebarBadges.Target] = [.init(appId: "a", entities: ["memo"])]
        await badges.refreshIfDue(targets, uid: "me", source: source, store: store)
        await badges.refreshIfDue(targets, uid: "me", source: source, store: store) // 別のウインドウ・前面に戻った
        XCTAssertEqual(source.calls, ["a/memo"])
        XCTAssertEqual(badges.count("a"), 1)

        clock = base.addingTimeInterval(MacSidebarBadges.minInterval + 1)
        await badges.refreshIfDue(targets, uid: "me", source: source, store: store)
        XCTAssertEqual(source.calls.count, 2)

        await badges.refreshIfDue(targets + [.init(appId: "b", entities: ["memo"])], uid: "me", source: source, store: store)
        XCTAssertEqual(source.calls.count, 4, "一覧が変わったらすぐ数え直す")

        await badges.refreshIfDue(targets, uid: nil, source: source, store: store)
        XCTAssertEqual(badges.counts, [:], "サインアウトはすぐ消す")
    }

    /// 共有の entity だけ問い合わせる(private の記録は他の人が書かない)
    func testTargetUsesOnlySharedEntities() throws {
        let entities = try JSONDecoder().decode([Entity].self, from: Data(#"""
        [{"id":"memo","name":"メモ","fields":[]},
         {"id":"board","name":"掲示板","scope":"shared","fields":[]},
         {"id":"news","name":"お知らせ","scope":"public","fields":[]}]
        """#.utf8))
        XCTAssertEqual(MacSidebarBadges.target(appId: "a", entities: entities)?.entities, ["board", "news"])
        XCTAssertNil(MacSidebarBadges.target(appId: "a", entities: [entities[0]]), "共有の entity が無いアプリは数えない")
    }

    func testBadgeTextAndVoiceOverLabel() {
        XCTAssertEqual(MacSidebarBadges.badgeText(3), "3")
        XCTAssertEqual(MacSidebarBadges.badgeText(99), "99")
        XCTAssertEqual(MacSidebarBadges.badgeText(100), "99+")
        let label = MacSidebarBadges.accessibilityLabel(name: "家計簿", count: 2)
        XCTAssertTrue(label.hasPrefix("家計簿, "), label)
        XCTAssertTrue(label.contains("2"), label)
        XCTAssertEqual(MacSidebarBadges.accessibilityLabel(name: "家計簿", count: 0), "家計簿")
    }

    // MARK: - 区分

    func testSplitKeepsListOrder() {
        let split = MacSidebarSections.split(["a", "s1", "b", "s2", "c"], isShared: { $0.hasPrefix("s") })
        XCTAssertEqual(split.own, ["a", "b", "c"])
        XCTAssertEqual(split.shared, ["s1", "s2"])
    }

    func testOriginsRememberSharedAndForgetOnDelete() {
        let origins = MacAppOrigins(defaults: defaults)
        XCTAssertFalse(origins.isShared("s1"))
        origins.markShared("s1")
        origins.markShared("s1")
        XCTAssertTrue(origins.isShared("s1"))
        XCTAssertEqual(defaults.stringArray(forKey: MacAppOrigins.key), ["s1"])
        origins.remove("s1")
        XCTAssertFalse(origins.isShared("s1"))
    }

    // MARK: - 並べ替え(区分の中)

    func testDragWithinSectionKeepsOtherSectionPositions() {
        let all = ["a", "s1", "b", "s2", "c"]
        // 作ったアプリ [a, b, c] の c を先頭へ
        let next = MacSidebarSections.moved(all, section: ["a", "b", "c"], fromOffsets: IndexSet(integer: 2), toOffset: 0)
        XCTAssertEqual(next, ["c", "s1", "a", "s2", "b"])
        XCTAssertEqual(MacSidebarSections.split(next, isShared: { $0.hasPrefix("s") }).shared, ["s1", "s2"])
    }

    func testUpDownWithinSharedSection() {
        let all = ["a", "s1", "b", "s2"]
        XCTAssertEqual(MacSidebarSections.moved(all, section: ["s1", "s2"], id: "s2", to: 0), ["a", "s2", "b", "s1"])
        XCTAssertEqual(MacSidebarSections.moved(all, section: ["s1", "s2"], id: "s1", to: 5), ["a", "s2", "b", "s1"])
    }

    func testLibrarySetOrderRejectsDifferentSet() throws {
        let library = MacAppLibrary(store: nil)
        for id in ["x", "y"] {
            let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
            let dsl = try AppDSL.load(contentsOf: url)
            library.apply(AppDocument(appId: id, version: 2, dsl: dsl), created: true)
        }
        let before = library.createdIds
        library.setOrder(["x"])
        XCTAssertEqual(library.createdIds, before)
        library.setOrder(before.reversed())
        XCTAssertEqual(library.createdIds, before.reversed())
    }
}
