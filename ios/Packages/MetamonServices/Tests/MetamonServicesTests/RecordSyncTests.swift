import XCTest
import MetamonDSL
@testable import MetamonServices

/// 呼び出しを記録し、`failures` 回だけ通信断を返す appconfig(冪等 clientId も実装)。
private actor RecordingClient: AppConfigClient {
    var calls: [String] = []
    var failures = 0
    var byClientId: [String: Record] = [:]
    /// create で返す拒否(422 等)。entity ごと
    var rejectCreate: [String: APIError] = [:]
    let inner = InMemoryAppConfigClient()

    func setFailures(_ n: Int) { failures = n }
    func setRejectCreate(_ entity: String, _ e: APIError) { rejectCreate[entity] = e }

    private func gate(_ name: String) throws {
        calls.append(name)
        if failures > 0 {
            failures -= 1
            throw URLError(.networkConnectionLost)
        }
    }

    func fetchDocument(appId: String) async throws -> AppDocument { try await inner.fetchDocument(appId: appId) }
    func listVersions(appId: String) async throws -> [VersionSummary] { [] }
    func restore(appId: String, toVersion version: Int) async throws -> AppDocument { try await inner.restore(appId: appId, toVersion: version) }
    func list(appId: String, entity: String) async throws -> [Record] {
        try gate("list")
        return try await inner.list(appId: appId, entity: entity)
    }
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        try await create(appId: appId, entity: entity, data: data, clientId: UUID().uuidString)
    }
    func create(appId: String, entity: String, data: [String: JSONValue], clientId: String) async throws -> Record {
        try gate("create:\(clientId)")
        if let e = rejectCreate[entity] { throw e }
        if let existing = byClientId[clientId] { return existing }
        let r = try await inner.create(appId: appId, entity: entity, data: data)
        byClientId[clientId] = r
        return r
    }
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        try gate("update:\(id)")
        return try await inner.update(appId: appId, entity: entity, id: id, data: data)
    }
    func delete(appId: String, entity: String, id: String) async throws {
        try gate("delete:\(id)")
        try await inner.delete(appId: appId, entity: entity, id: id)
    }
}

/// バックオフの待ち時間を記録するだけの sleep。
private final class SleepLog: @unchecked Sendable {
    private let lock = NSLock()
    private(set) var delays: [Duration] = []
    func record(_ d: Duration) { lock.withLock { delays.append(d) } }
}

final class RecordSyncTests: XCTestCase {
    private var dir: URL!

    override func setUp() {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-records-\(UUID().uuidString)")
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
    }

    // MARK: - RecordStore

    func testRecordStoreCRUDAndPersistence() async throws {
        let store = RecordStore(directory: dir)
        let empty = await store.records(appId: "a", entity: "expense")
        XCTAssertTrue(empty.isEmpty)

        await store.upsert(LocalRecord(appId: "a", entity: "expense", recordId: "1", data: ["amount": 100], createdAt: "2026-10-24T01:00:00Z"))
        await store.upsert(LocalRecord(appId: "a", entity: "expense", recordId: "2", data: ["amount": 200], createdAt: "2026-10-24T02:00:00Z"))
        var rows = await store.records(appId: "a", entity: "expense")
        XCTAssertEqual(rows.map(\.id), ["2", "1"], "createdAt 降順")

        await store.upsert(LocalRecord(appId: "a", entity: "expense", recordId: "1", data: ["amount": 150, "extra": "keep"], createdAt: "2026-10-24T01:00:00Z"))
        let row = await store.row(appId: "a", entity: "expense", recordId: "1")
        XCTAssertEqual(row?.data["amount"], 150)
        XCTAssertEqual(row?.data["extra"], "keep", "DSL に無い項目も保持する")

        await store.remove(appId: "a", entity: "expense", recordId: "2")
        rows = await store.records(appId: "a", entity: "expense")
        XCTAssertEqual(rows.map(\.id), ["1"])

        // 別インスタンスで読み直せる(ファイル永続化)
        let reopened = RecordStore(directory: dir)
        let again = await reopened.records(appId: "a", entity: "expense")
        XCTAssertEqual(again.map(\.id), ["1"])
        // appId / entity で分かれている
        let other = await reopened.records(appId: "b", entity: "expense")
        XCTAssertTrue(other.isEmpty)
    }

    func testRecordStoreReplaceKeepsPendingAndHidesDeletes() async throws {
        let store = RecordStore(directory: dir)
        await store.upsert(LocalRecord(appId: "a", entity: "e", recordId: "old", data: [:], createdAt: "1"))
        await store.upsert(LocalRecord(appId: "a", entity: "e", recordId: "local-x", data: ["n": 1], createdAt: "9", pendingOp: .create, clientId: "x"))
        await store.upsert(LocalRecord(appId: "a", entity: "e", recordId: "del", data: [:], createdAt: "5", pendingOp: .delete))

        let shown = await store.replace(appId: "a", entity: "e", with: [Record(id: "s1", data: [:], createdAt: "3"), Record(id: "del", data: [:], createdAt: "5")])
        XCTAssertEqual(shown.map(\.id), ["local-x", "s1"], "サーバーに無い old は消え、未送信 create は残り、delete 待ちは表示しない")
        let pending = await store.pendingCount(appId: "a")
        XCTAssertEqual(pending, 2)

        await store.markSynced(appId: "a", entity: "e", localId: "local-x", server: Record(id: "srv", data: ["n": 1], createdAt: "9"))
        let after = await store.records(appId: "a", entity: "e")
        XCTAssertEqual(Set(after.map(\.id)), ["srv", "s1"])
        let synced = await store.row(appId: "a", entity: "e", recordId: "srv")
        XCTAssertEqual(synced?.pendingOp, PendingOp.none)
        XCTAssertEqual(synced?.clientId, "x")
    }

    // MARK: - SyncQueue

    func testSyncQueueSendsInOrder() async throws {
        let client = RecordingClient()
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        await queue.enqueue(SyncOperation(id: "c1", appId: "a", entity: "e", recordId: "local-c1", kind: .create, data: ["n": 1]))
        await queue.enqueue(SyncOperation(id: "c2", appId: "a", entity: "e", recordId: "local-c2", kind: .create, data: ["n": 2]))
        await queue.enqueue(SyncOperation(appId: "a", entity: "e", recordId: "local-c1", kind: .delete))
        await queue.drain()
        let calls = await client.calls
        XCTAssertEqual(calls, ["create:c2"], "create+delete は畳まれ、残りを FIFO で送る")
        let count = await queue.count
        XCTAssertEqual(count, 0)
    }

    func testSyncQueueCoalescesUpdateIntoCreateAndTranslatesIds() async throws {
        let client = RecordingClient()
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        await queue.enqueue(SyncOperation(id: "c1", appId: "a", entity: "e", recordId: "local-c1", kind: .create, data: ["n": 1]))
        let r = await queue.enqueue(SyncOperation(appId: "a", entity: "e", recordId: "local-c1", kind: .update, data: ["m": 2]))
        XCTAssertEqual(r, .merged)
        let ops = await queue.operations
        XCTAssertEqual(ops.count, 1)
        XCTAssertEqual(ops[0].data, ["n": 1, "m": 2])
        await queue.drain()

        // 送信後に同じ仮 id へ update が来たらサーバー id に引き直す
        await queue.enqueue(SyncOperation(appId: "a", entity: "e", recordId: "local-c1", kind: .update, data: ["m": 3]))
        await queue.drain()
        let calls = await client.calls
        let created = await client.byClientId
        let serverId = try XCTUnwrap(created["c1"]?.id)
        XCTAssertEqual(calls, ["create:c1", "update:\(serverId)"])
    }

    func testSyncQueueRetriesWithSameClientIdAndBackoff() async throws {
        let client = RecordingClient()
        await client.setFailures(2)
        let log = SleepLog()
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: client,
                              policy: BackoffPolicy(base: .milliseconds(100), multiplier: 2, maxAttempts: 5),
                              sleep: { log.record($0) })
        await queue.enqueue(SyncOperation(id: "c1", appId: "a", entity: "e", recordId: "local-c1", kind: .create, data: ["n": 1]))
        await queue.drain()
        let calls = await client.calls
        XCTAssertEqual(calls, ["create:c1", "create:c1", "create:c1"], "再送は同じ clientId")
        XCTAssertEqual(log.delays, [.milliseconds(100), .milliseconds(200)], "指数バックオフ")
        let created = await client.byClientId.count
        XCTAssertEqual(created, 1, "サーバー側は 1 件だけ")
        let remaining = await queue.count
        XCTAssertEqual(remaining, 0)
    }

    func testSyncQueuePausesAfterMaxAttemptsAndResumes() async throws {
        let client = RecordingClient()
        await client.setFailures(10)
        let log = SleepLog()
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: client,
                              policy: BackoffPolicy(base: .milliseconds(10), multiplier: 2, maxAttempts: 5), sleep: { log.record($0) })
        await queue.enqueue(SyncOperation(id: "c1", appId: "a", entity: "e", recordId: "local-c1", kind: .create, data: [:]))
        await queue.drain()
        var calls = await client.calls
        XCTAssertEqual(calls.count, 5, "最大 5 回で停止")
        XCTAssertEqual(log.delays.count, 4)
        let paused = await queue.isPaused
        XCTAssertTrue(paused)
        let remaining = await queue.count
        XCTAssertEqual(remaining, 1, "諦めずに残す")

        // 再開(オンライン復帰相当)で成功
        await client.setFailures(0)
        await queue.resume()
        calls = await client.calls
        XCTAssertEqual(calls.count, 6)
        let left = await queue.count
        XCTAssertEqual(left, 0)
    }

    func testSyncQueueConflictServerWins() async throws {
        let client = RecordingClient()
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        await store.upsert(LocalRecord(appId: "a", entity: "e", recordId: "gone", data: ["n": 1], pendingOp: .update))
        let events = await queue.events()
        await queue.enqueue(SyncOperation(appId: "a", entity: "e", recordId: "gone", kind: .update, data: ["n": 2]))
        await queue.drain() // InMemory の update は 404 を投げる
        var it = events.makeAsyncIterator()
        let first = await it.next()
        guard case .conflict(let appId, let entity, _)? = first else { return XCTFail("expected conflict, got \(String(describing: first))") }
        XCTAssertEqual(appId, "a")
        XCTAssertEqual(entity, "e")
        let row = await store.row(appId: "a", entity: "e", recordId: "gone")
        XCTAssertEqual(row?.pendingOp, PendingOp.none, "未送信印を落とし、次の pull でサーバー版に置き換わる")
        let count = await queue.count
        XCTAssertEqual(count, 0)
    }

    /// 旅行の割り勘: 未送信のメンバーを「払った人」に選んだ支出は、メンバーの送信後にサーバー id で送る。
    func testSyncQueueResolvesLocalRefIdAfterReferencedCreate() async throws {
        let client = RecordingClient()
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        await queue.enqueue(SyncOperation(id: "m1", appId: "a", entity: "members", recordId: "local-m1", kind: .create, data: ["name": "たろう"]))
        await queue.enqueue(SyncOperation(id: "e1", appId: "a", entity: "expenses", recordId: "local-e1", kind: .create,
                                          data: ["amount": 3000, "payer": "local-m1", "memo": "local-は普通の文字"]))
        await queue.drain()
        let created = await client.byClientId
        let memberId = try XCTUnwrap(created["m1"]?.id)
        XCTAssertFalse(memberId.hasPrefix("local-"))
        XCTAssertEqual(created["e1"]?.data["payer"], .string(memberId), "仮 id はサーバー id に差し替えて送る")
        XCTAssertEqual(created["e1"]?.data["memo"], .string("local-は普通の文字"), "別名に無い文字列はそのまま")
    }

    /// 422 で弾かれたら操作とローカル行を捨て、どの項目か(details.field)を UI に渡す。
    func testSyncQueueRejectedEmitsFieldAndDropsCreate() async throws {
        let client = RecordingClient()
        await client.setRejectCreate("expenses", APIError.server(status: 422, code: "invalid_record",
                                                                message: "field \"payer\" is required", details: ["field": .string("payer")]))
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        await store.upsert(LocalRecord(appId: "a", entity: "expenses", recordId: "local-e1", data: ["amount": 1], pendingOp: .create, clientId: "e1"))
        let events = await queue.events()
        await queue.enqueue(SyncOperation(id: "e1", appId: "a", entity: "expenses", recordId: "local-e1", kind: .create, data: ["amount": 1]))
        await queue.drain()
        var it = events.makeAsyncIterator()
        let first = await it.next()
        guard case .rejected(let appId, let entity, let message, let field)? = first else {
            return XCTFail("expected rejected, got \(String(describing: first))")
        }
        XCTAssertEqual([appId, entity, field], ["a", "expenses", "payer"])
        XCTAssertEqual(message, "field \"payer\" is required")
        let count = await queue.count
        XCTAssertEqual(count, 0, "拒否された操作は再送しない(キューに詰まらない)")
        let rows = await store.records(appId: "a", entity: "expenses")
        XCTAssertTrue(rows.isEmpty, "拒否された create のローカル行は消える")
    }

    func testClassify() {
        XCTAssertEqual(SyncQueue.classify(URLError(.notConnectedToInternet)), .transient)
        XCTAssertEqual(SyncQueue.classify(APIError.http(status: 503, body: "")), .transient)
        XCTAssertEqual(SyncQueue.classify(APIError.http(status: 401, body: "")), .transient)
        XCTAssertEqual(SyncQueue.classify(APIError.server(status: 409, code: "conflict", message: "")), .conflict)
        XCTAssertEqual(SyncQueue.classify(APIError.http(status: 404, body: "")), .conflict)
        XCTAssertEqual(SyncQueue.classify(APIError.server(status: 422, code: "invalid_record", message: "bad amount")), .rejected("bad amount"))
    }

    // MARK: - Engine(オフライン → オンライン)

    @MainActor
    func testEngineSavesLocallyOfflineThenSendsWhenOnline() async throws {
        let gated = OfflineSimulatingAppConfigClient(inner: InMemoryAppConfigClient())
        gated.setOffline(true)
        let connectivity = ManualConnectivity(online: false)
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: gated, policy: .immediate)
        let engine = RecordSyncEngine(store: store, queue: queue, client: gated, connectivity: connectivity)
        engine.start()

        // 圏外で保存 → 即ローカルに出る・サーバーには行かない
        let shown = await engine.save(appId: "a", entity: "expense", recordId: nil, data: ["amount": 980])
        XCTAssertEqual(shown.count, 1)
        XCTAssertTrue(shown[0].id.hasPrefix("local-"))
        let cached = await engine.cached(appId: "a", entity: "expense")
        XCTAssertEqual(cached.map(\.id), shown.map(\.id))
        XCTAssertFalse(gated.log.contains { $0.hasPrefix("pass create") })
        try await waitUntil("offline status") { engine.status == .offline(pending: 1) }

        // 復帰 → キューが流れ、サーバー id に差し替わる
        gated.setOffline(false)
        connectivity.set(online: true)
        try await waitUntil("synced") { engine.status == .synced }
        let server = try await gated.inner.list(appId: "a", entity: "expense")
        XCTAssertEqual(server.count, 1)
        XCTAssertEqual(server[0]["amount"], 980)
        let local = await engine.cached(appId: "a", entity: "expense")
        XCTAssertEqual(local.map(\.id), server.map(\.id), "仮 id がサーバー id に置き換わる")

        // pull で置換しても同じ
        let pulled = try await engine.pull(appId: "a", entity: "expense")
        XCTAssertEqual(pulled.map(\.id), server.map(\.id))
        engine.stop()
    }

    @MainActor
    func testEngineDeleteOfPendingCreateNeverHitsServer() async throws {
        let gated = OfflineSimulatingAppConfigClient(inner: InMemoryAppConfigClient())
        gated.setOffline(true)
        let connectivity = ManualConnectivity(online: false)
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: gated, policy: .immediate)
        let engine = RecordSyncEngine(store: store, queue: queue, client: gated, connectivity: connectivity)
        engine.start()
        let shown = await engine.save(appId: "a", entity: "e", recordId: nil, data: ["n": 1])
        let afterDelete = await engine.delete(appId: "a", entity: "e", recordId: shown[0].id)
        XCTAssertTrue(afterDelete.isEmpty)
        let pending = await queue.count
        XCTAssertEqual(pending, 0)
        gated.setOffline(false)
        connectivity.set(online: true)
        try await waitUntil("synced") { engine.status == .synced }
        XCTAssertFalse(gated.log.contains { $0.hasPrefix("pass create") || $0.hasPrefix("pass delete") })
        engine.stop()
    }

    private func waitUntil(_ what: String, timeout: Duration = .seconds(5), _ cond: @MainActor () -> Bool) async throws {
        let deadline = ContinuousClock.now + timeout
        while ContinuousClock.now < deadline {
            if await cond() { return }
            try await Task.sleep(for: .milliseconds(20))
        }
        XCTFail("timed out waiting for \(what)")
    }
}
