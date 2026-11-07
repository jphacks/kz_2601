import XCTest
import MetamonDSL
@testable import MetamonServices

/// 1 回だけ呼び出しを止める関門。テストが入ったのを待ち、好きな時に通す。
private actor Gate {
    private var entered = false
    private var entryWaiter: CheckedContinuation<Void, Never>?
    private var held: CheckedContinuation<Void, Never>?

    func hold() async {
        entered = true
        await withCheckedContinuation { (c: CheckedContinuation<Void, Never>) in
            held = c
            entryWaiter?.resume()
            entryWaiter = nil
        }
    }

    func waitUntilHeld() async {
        if held != nil { return }
        await withCheckedContinuation { entryWaiter = $0 }
    }

    func release() {
        held?.resume()
        held = nil
    }
}

/// create と delete の途中で止められる appconfig。送信中に次の操作が来る順序を確実に作る。
private final class HoldingClient: AppConfigClient, @unchecked Sendable {
    let inner = InMemoryAppConfigClient()
    let create = Gate()
    let delete = Gate()
    private let lock = NSLock()
    private var holdsDelete = false
    private var rejectDelete: APIError?

    func setHoldsDelete(_ on: Bool) { lock.withLock { holdsDelete = on } }
    func setRejectDelete(_ e: APIError?) { lock.withLock { rejectDelete = e } }

    func fetchDocument(appId: String) async throws -> AppDocument { try await inner.fetchDocument(appId: appId) }
    func listVersions(appId: String) async throws -> [VersionSummary] { [] }
    func restore(appId: String, toVersion version: Int) async throws -> AppDocument { try await inner.restore(appId: appId, toVersion: version) }
    func list(appId: String, entity: String) async throws -> [Record] { try await inner.list(appId: appId, entity: entity) }
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        let record = try await inner.create(appId: appId, entity: entity, data: data)
        await create.hold()
        return record
    }
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        try await inner.update(appId: appId, entity: entity, id: id, data: data)
    }
    func delete(appId: String, entity: String, id: String) async throws {
        let (holds, rejected) = lock.withLock { (holdsDelete, rejectDelete) }
        if holds { await delete.hold() }
        if let rejected { throw rejected }
        try await inner.delete(appId: appId, entity: entity, id: id)
    }
}

/// 作成の送信中に同じ行を消した時(取り消せる削除の時間切れ・背面への移動でも起きる)。
/// 作成は止められないので create → delete の順に送り、行は仮 id からサーバー id に替わる。
@MainActor
final class RecordDeleteDuringCreateTests: XCTestCase {
    private var dir: URL!
    private var client: HoldingClient!
    private var store: RecordStore!
    private var queue: SyncQueue!
    private var engine: RecordSyncEngine!

    override func setUp() async throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-delete-create-\(UUID().uuidString)")
        client = HoldingClient()
        store = RecordStore(directory: dir)
        queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        engine = RecordSyncEngine(store: store, queue: queue, client: client, connectivity: ManualConnectivity(online: true))
        engine.start()
    }

    override func tearDown() async throws {
        engine.stop()
        await client.create.release()
        await client.delete.release()
        try? FileManager.default.removeItem(at: dir)
    }

    private func rows() async -> [LocalRecord] { await store.rows(appId: "a", entity: "memo") }
    private func serverIds() async throws -> [String] { try await client.inner.list(appId: "a", entity: "memo").map(\.id) }

    /// 作成の送信を止めたところで保存を返す。
    private func saveWithCreateInFlight() async -> Record {
        let made = await engine.save(appId: "a", entity: "memo", recordId: nil, data: ["title": "パン"])
        await client.create.waitUntilHeld()
        return made[0]
    }

    func testDeletedRowStaysHiddenAndIsRemovedAfterCreateIsSent() async throws {
        client.setHoldsDelete(true)
        let made = await saveWithCreateInFlight()

        let shown = await engine.delete(appId: "a", entity: "memo", recordId: made.id)
        XCTAssertTrue(shown.isEmpty, "消した行はすぐ隠れる")

        await client.create.release()
        await client.delete.waitUntilHeld()
        let between = await engine.cached(appId: "a", entity: "memo")
        XCTAssertTrue(between.isEmpty, "作成が届いても、削除を送り終わるまで戻らない")

        await client.delete.release()
        try await waitUntilQueueIsEmpty()
        let left = await rows()
        XCTAssertTrue(left.isEmpty, "サーバーで消した行が端末に残らない")
        let server = try await serverIds()
        XCTAssertTrue(server.isEmpty)
    }

    func testRejectedDeleteAfterCreateShowsRowAgain() async throws {
        client.setRejectDelete(.server(status: 403, code: "forbidden", message: "no"))
        let made = await saveWithCreateInFlight()
        _ = await engine.delete(appId: "a", entity: "memo", recordId: made.id)

        await client.create.release()
        try await waitUntilQueueIsEmpty()
        let shown = await engine.cached(appId: "a", entity: "memo")
        let server = try await serverIds()
        XCTAssertEqual(shown.map(\.id), server, "断られた削除は、サーバーに残った行を隠したままにしない")
        let left = await rows()
        XCTAssertEqual(left.map(\.pendingOp), [PendingOp.none])
    }

    func testDeleteWithTemporaryIdAfterCreateWasSent() async throws {
        client.setHoldsDelete(true)
        let made = await saveWithCreateInFlight()
        await client.create.release()
        try await waitUntilQueueIsEmpty()

        // 画面の一覧がまだ仮 id のまま(書き換え前)に消す
        let shown = await engine.delete(appId: "a", entity: "memo", recordId: made.id)
        XCTAssertTrue(shown.isEmpty, "仮 id でも同じ行を隠す")
        await client.delete.waitUntilHeld()
        await client.delete.release()
        try await waitUntilQueueIsEmpty()
        let left = await rows()
        XCTAssertTrue(left.isEmpty)
        let server = try await serverIds()
        XCTAssertTrue(server.isEmpty)
    }

    /// 行の更新が終わってから列を空にするので、`status` より確実に送り終わりを待てる。
    private func waitUntilQueueIsEmpty(timeout: Duration = .seconds(5)) async throws {
        let deadline = ContinuousClock.now + timeout
        while ContinuousClock.now < deadline {
            if await queue.count == 0 { return }
            try await Task.sleep(for: .milliseconds(20))
        }
        XCTFail("timed out waiting for the queue")
    }
}
