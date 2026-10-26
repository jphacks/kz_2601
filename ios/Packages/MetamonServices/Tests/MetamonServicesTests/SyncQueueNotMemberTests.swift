import XCTest
import MetamonDSL
@testable import MetamonServices

/// records の書き込みが常に 403 not_member になる appconfig(#254: 共有に参加していない)。
private actor NotMemberClient: AppConfigClient {
    var calls = 0
    let inner = InMemoryAppConfigClient()
    private let denied = APIError.server(status: 403, code: "forbidden", message: "join this app with a share link to see its shared data",
                                         details: ["reason": .string("not_member")])

    func fetchDocument(appId: String) async throws -> AppDocument { try await inner.fetchDocument(appId: appId) }
    func listVersions(appId: String) async throws -> [VersionSummary] { [] }
    func restore(appId: String, toVersion version: Int) async throws -> AppDocument { try await inner.restore(appId: appId, toVersion: version) }
    func list(appId: String, entity: String) async throws -> [Record] { throw denied }
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { calls += 1; throw denied }
    func create(appId: String, entity: String, data: [String: JSONValue], clientId: String) async throws -> Record { calls += 1; throw denied }
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { calls += 1; throw denied }
    func delete(appId: String, entity: String, id: String) async throws { calls += 1; throw denied }
}

final class SyncQueueNotMemberTests: XCTestCase {
    private var dir: URL!

    override func setUp() {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-notmember-\(UUID().uuidString)")
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
    }

    /// 送信待ちに残さず(再送しない)、楽観的に足したローカル行を消して、UI に not_member を知らせる。
    func testNotMemberDropsOperationAndEmitsDedicatedEvent() async throws {
        let client = NotMemberClient()
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        await store.upsert(LocalRecord(appId: "a", entity: "expense", recordId: "local-e1", data: ["amount": 1], pendingOp: .create, clientId: "e1"))
        let events = await queue.events()
        await queue.enqueue(SyncOperation(id: "e1", appId: "a", entity: "expense", recordId: "local-e1", kind: .create, data: ["amount": 1]))
        await queue.drain()

        var it = events.makeAsyncIterator()
        let first = await it.next()
        XCTAssertEqual(first, .notMember(appId: "a", entity: "expense"))
        let count = await queue.count
        XCTAssertEqual(count, 0, "参加していない操作は再送しない")
        let calls = await client.calls
        XCTAssertEqual(calls, 1)
        let rows = await store.records(appId: "a", entity: "expense")
        XCTAssertTrue(rows.isEmpty, "楽観的に足した行は戻す")
    }
}
