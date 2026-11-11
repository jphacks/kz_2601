import XCTest
import MetamonDSL
@testable import MetamonServices

/// 仲間探しのプロフィールの投稿が 400 age_unconfirmed になる appconfig(#507: 18 歳以上の申告が無い)。
private actor AgeUnconfirmedClient: AppConfigClient {
    var calls = 0
    let inner = InMemoryAppConfigClient()
    private let refused = APIError.server(status: 400, code: "bad_request", message: "confirm that you are 18 or older to connect with people",
                                          details: ["reason": .string("age_unconfirmed")])

    func fetchDocument(appId: String) async throws -> AppDocument { try await inner.fetchDocument(appId: appId) }
    func listVersions(appId: String) async throws -> [VersionSummary] { [] }
    func restore(appId: String, toVersion version: Int) async throws -> AppDocument { try await inner.restore(appId: appId, toVersion: version) }
    func list(appId: String, entity: String) async throws -> [Record] { [] }
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { calls += 1; throw refused }
    func create(appId: String, entity: String, data: [String: JSONValue], clientId: String) async throws -> Record { calls += 1; throw refused }
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { calls += 1; throw refused }
    func delete(appId: String, entity: String, id: String) async throws { calls += 1 }
}

final class SyncQueueAgeUnconfirmedTests: XCTestCase {
    private var dir: URL!

    override func setUp() {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-age-\(UUID().uuidString)")
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
    }

    /// 再送せず、楽観的に足した行を消して、送ろうとした値ごと知らせる(申告のあとに投稿し直す)。
    func testProfilePostNeedsDeclaration() async throws {
        let client = AgeUnconfirmedClient()
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        await store.upsert(LocalRecord(appId: "a", entity: "profile", recordId: "local-p1", data: ["hobby": "将棋"], pendingOp: .create, clientId: "p1"))
        let events = await queue.events()
        await queue.enqueue(SyncOperation(id: "p1", appId: "a", entity: "profile", recordId: "local-p1", kind: .create, data: ["hobby": "将棋"]))
        await queue.drain()

        var it = events.makeAsyncIterator()
        let first = await it.next()
        XCTAssertEqual(first, .ageUnconfirmed(appId: "a", entity: "profile", data: ["hobby": "将棋"], recordId: "local-p1"))
        let count = await queue.count
        XCTAssertEqual(count, 0, "申告が無いまま再送しない")
        let calls = await client.calls
        XCTAssertEqual(calls, 1)
        let rows = await store.records(appId: "a", entity: "profile")
        XCTAssertTrue(rows.isEmpty, "楽観的に足した行は戻す")
    }
}
