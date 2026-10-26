import XCTest
@testable import MetamonServices

/// #268 新着: 自分以外・前回より後だけ数える / 初回は基準だけ / 取れない時は基準を進めない。
final class NewItemsTests: XCTestCase {
    private let me = "uid-me"
    private let since = ISO8601DateFormatter().date(from: "2026-10-26T00:00:00Z")!

    private var defaults: UserDefaults!
    private var store: LastOpenedStore!

    override func setUp() {
        defaults = UserDefaults(suiteName: "NewItemsTests-\(UUID().uuidString)")
        store = LastOpenedStore(defaults: defaults)
    }

    // MARK: NewItemsCounter

    func testCountsOnlyOthersChangedAfterSince() {
        let stamps = [
            RecordStamp(id: "a", author: "uid-other", createdAt: "2026-10-26T01:00:00Z"),            // 新規
            RecordStamp(id: "b", author: "uid-other", createdAt: "2026-10-25T01:00:00Z",
                        updatedAt: "2026-10-26T02:00:00.123456789Z"),                                 // 更新(小数 9 桁)
            RecordStamp(id: "c", author: "uid-other", createdAt: "2026-10-25T23:59:59Z"),            // 前回より前
            RecordStamp(id: "d", author: me, createdAt: "2026-10-26T01:00:00Z"),                      // 自分
            RecordStamp(id: "e", author: nil, createdAt: "2026-10-26T01:00:00Z"),                     // private / author 無し
            RecordStamp(id: "f", author: "uid-other", createdAt: nil),                                // 時刻なし
        ]
        XCTAssertEqual(NewItemsCounter.count(stamps, me: me, since: since), 2)
    }

    func testParsesNanosecondTimestamps() {
        XCTAssertNotNil(RecordStamp.parseDate("2026-10-26T02:00:00.123456789Z"))
        XCTAssertNotNil(RecordStamp.parseDate("2026-10-26T02:00:00.5+09:00"))
        XCTAssertNotNil(RecordStamp.parseDate("2026-10-26T02:00:00Z"))
        XCTAssertNil(RecordStamp.parseDate("yesterday"))
    }

    func testStampDecodesRecordJSONWithAuthor() throws {
        let json = #"{"id":"r1","entity":"post","data":{"text":"hi"},"author":"uid-x","authorName":"匿名","createdAt":"2026-10-26T01:00:00Z"}"#
        let s = try JSONDecoder().decode(RecordStamp.self, from: Data(json.utf8))
        XCTAssertEqual(s, RecordStamp(id: "r1", author: "uid-x", createdAt: "2026-10-26T01:00:00Z"))
    }

    // MARK: NewItemsCheck

    func testFirstOpenOnlySetsBaseline() async {
        let check = NewItemsCheck(source: FakeSource(stamps: [RecordStamp(id: "a", author: "x", createdAt: "2026-10-26T01:00:00Z")]), store: store)
        let now = since.addingTimeInterval(7200)
        let first = await check.run(appId: "app", entities: ["post"], uid: me, now: now)
        XCTAssertNil(first)
        XCTAssertEqual(store.lastOpened(appId: "app"), now)
    }

    func testCountsSinceLastOpenAndAdvancesBaseline() async {
        store.mark(appId: "app", at: since)
        let source = FakeSource(stamps: [RecordStamp(id: "a", author: "x", createdAt: "2026-10-26T01:00:00Z")])
        let check = NewItemsCheck(source: source, store: store)
        let now = since.addingTimeInterval(7200)
        let count = await check.run(appId: "app", entities: ["post", "comment"], uid: me, now: now)
        XCTAssertEqual(count, 2) // entity ごとに 1 件
        XCTAssertEqual(store.lastOpened(appId: "app"), now)
        let again = await check.run(appId: "app", entities: ["post"], uid: me, now: now.addingTimeInterval(60))
        XCTAssertEqual(again, 0, "同じものは 2 回数えない")
    }

    func testFailureKeepsBaseline() async {
        store.mark(appId: "app", at: since)
        let check = NewItemsCheck(source: FakeSource(stamps: [], fails: true), store: store)
        let count = await check.run(appId: "app", entities: ["post"], uid: me, now: since.addingTimeInterval(60))
        XCTAssertNil(count)
        XCTAssertEqual(store.lastOpened(appId: "app"), since)
    }

    func testNoEntitiesDoesNothing() async {
        let check = NewItemsCheck(source: FakeSource(stamps: []), store: store)
        let count = await check.run(appId: "app", entities: [], uid: me)
        XCTAssertNil(count)
        XCTAssertNil(store.lastOpened(appId: "app"))
    }

    func testBaselineIsPerApp() {
        store.mark(appId: "a", at: since)
        XCTAssertEqual(store.lastOpened(appId: "a"), since)
        XCTAssertNil(store.lastOpened(appId: "b"))
    }
}

private struct FakeSource: SharedRecordStampSource {
    var stamps: [RecordStamp]
    var fails = false

    func sharedStamps(appId: String, entity: String, notBy uid: String) async throws -> [RecordStamp] {
        if fails { throw URLError(.notConnectedToInternet) }
        return stamps.map { RecordStamp(id: "\(entity)-\($0.id)", author: $0.author, createdAt: $0.createdAt, updatedAt: $0.updatedAt) }
    }
}
