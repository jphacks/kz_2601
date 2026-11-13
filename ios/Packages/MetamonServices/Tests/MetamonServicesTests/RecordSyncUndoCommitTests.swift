import XCTest
import MetamonDSL
@testable import MetamonServices

/// 取り消し待ちの削除を送る(commitPendingDeletions)・捨てる(dropPendingDeletions / purge)。
/// 取り消し自体の挙動は RecordDeleteUndoTests。ここは「送る・捨てる」の出口だけを確かめる。
@MainActor
final class RecordSyncUndoCommitTests: XCTestCase {
    private var dir: URL!
    private var client: OfflineSimulatingAppConfigClient!
    private var connectivity: ManualConnectivity!
    private var store: RecordStore!
    private var queue: SyncQueue!
    private var engine: RecordSyncEngine!

    override func setUp() async throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-undocommit-\(UUID().uuidString)")
        client = OfflineSimulatingAppConfigClient(inner: InMemoryAppConfigClient())
        // 圏外で始める: 列に入ったことを「送られずに残る」で数えられる
        client.setOffline(true)
        connectivity = ManualConnectivity(online: false)
        store = RecordStore(directory: dir)
        queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        engine = RecordSyncEngine(store: store, queue: queue, client: client, connectivity: connectivity)
        engine.start()
    }

    override func tearDown() async throws {
        engine.stop()
        try? FileManager.default.removeItem(at: dir)
    }

    private func seedSynced(appId: String, id: String) async {
        await store.upsert(LocalRecord(appId: appId, entity: "memo", recordId: id, data: ["title": "t"], createdAt: "2026-01-01T00:00:00Z",
                                       updatedAt: "2026-01-01T00:00:00Z"))
    }

    private var sentDeletes: [String] { client.log.filter { $0.hasPrefix("pass delete") } }

    // MARK: commitPendingDeletions

    func testCommitWithNothingPendingChangesNothing() async {
        await seedSynced(appId: "a", id: "r1")
        await engine.commitPendingDeletions()
        let queued = await queue.count
        XCTAssertEqual(queued, 0)
        let shown = await engine.cached(appId: "a", entity: "memo")
        XCTAssertEqual(shown.map(\.id), ["r1"])
        XCTAssertTrue(engine.pendingDeletions.isEmpty)
    }

    func testCommitMovesEveryWaitingDeletionIntoTheQueue() async {
        await seedSynced(appId: "a", id: "r1")
        await seedSynced(appId: "a", id: "r2")
        await seedSynced(appId: "b", id: "r3")
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: "r1", grace: .seconds(30))
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: "r2", grace: .seconds(30))
        _ = await engine.deleteUndoably(appId: "b", entity: "memo", recordId: "r3", grace: .seconds(30))
        let before = await queue.count
        XCTAssertEqual(before, 0, "待ちの間は列に積まない")
        XCTAssertEqual(engine.pendingDeletions.count, 3)

        await engine.commitPendingDeletions()
        let after = await queue.count
        XCTAssertEqual(after, 3, "時間を待たずに全部が列に入る")
        XCTAssertTrue(engine.pendingDeletions.isEmpty, "待ちは空になる")
        XCTAssertEqual(engine.pendingCount, 3, "未送信の件数にも入る")
        let rows = await store.rows(appId: "a", entity: "memo")
        XCTAssertEqual(rows.map(\.pendingOp), [.delete, .delete], "ローカルの行は削除待ちになる")
    }

    func testCommitAfterTimerAlreadyFiredDoesNotQueueTwice() async throws {
        await seedSynced(appId: "a", id: "r1")
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: "r1", grace: .milliseconds(30))
        let deadline = ContinuousClock.now + .seconds(5)
        while await queue.count == 0, ContinuousClock.now < deadline { try await Task.sleep(for: .milliseconds(20)) }
        await engine.commitPendingDeletions()
        let queued = await queue.count
        XCTAssertEqual(queued, 1, "時間切れで積んだ分を二重に積まない")
    }

    func testCommittedDeletionCannotBeUndone() async {
        await seedSynced(appId: "a", id: "r1")
        let (_, ticket) = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: "r1", grace: .seconds(30))
        await engine.commitPendingDeletions()
        let undone = await engine.undoDelete(ticket)
        XCTAssertNil(undone, "送った後は戻せない")
    }

    func testCommittedDeletionIsSentOnceOnline() async throws {
        let rec = try await client.inner.create(appId: "a", entity: "memo", data: ["title": "牛乳"])
        await store.upsert(LocalRecord(appId: "a", entity: "memo", server: rec))
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: rec.id, grace: .seconds(30))
        await engine.commitPendingDeletions()
        XCTAssertTrue(sentDeletes.isEmpty, "圏外の間は送らない")
        client.setOffline(false)
        connectivity.set(online: true)
        let deadline = ContinuousClock.now + .seconds(5)
        while sentDeletes.isEmpty, ContinuousClock.now < deadline { try await Task.sleep(for: .milliseconds(20)) }
        XCTAssertEqual(sentDeletes, ["pass delete memo/\(rec.id)"])
    }

    // MARK: dropPendingDeletions

    func testDropByAppRemovesOnlyThatAppsWaitingDeletions() async {
        await seedSynced(appId: "a", id: "r1")
        await seedSynced(appId: "b", id: "r2")
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: "r1", grace: .seconds(30))
        let (_, keep) = await engine.deleteUndoably(appId: "b", entity: "memo", recordId: "r2", grace: .seconds(30))
        engine.dropPendingDeletions { $0.appId == "a" }
        XCTAssertEqual(engine.pendingDeletions, [keep])
    }

    func testDropNeverSendsAnythingEvenAfterCommit() async {
        await seedSynced(appId: "a", id: "r1")
        let (_, ticket) = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: "r1", grace: .seconds(30))
        engine.dropPendingDeletions { _ in true }
        XCTAssertTrue(engine.pendingDeletions.isEmpty)
        await engine.commitPendingDeletions()
        let queued = await queue.count
        XCTAssertEqual(queued, 0, "捨てた待ちは送信の列に入らない")
        XCTAssertTrue(sentDeletes.isEmpty)
        let undone = await engine.undoDelete(ticket)
        XCTAssertNil(undone, "捨てた待ちは戻す対象でもない")
    }

    func testDroppedTimerNeverFires() async throws {
        await seedSynced(appId: "a", id: "r1")
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: "r1", grace: .milliseconds(50))
        engine.dropPendingDeletions { _ in true }
        try await Task.sleep(for: .milliseconds(250))
        let queued = await queue.count
        XCTAssertEqual(queued, 0, "タイマーも止まる")
    }

    func testPurgeDropsThatAppsWaitingDeletionAndSendsNothing() async {
        await seedSynced(appId: "a", id: "r1")
        await seedSynced(appId: "b", id: "r2")
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: "r1", grace: .seconds(30))
        _ = await engine.deleteUndoably(appId: "b", entity: "memo", recordId: "r2", grace: .seconds(30))
        await engine.purge(appId: "a")
        XCTAssertEqual(engine.pendingDeletions.map(\.appId), ["b"], "アプリを消した時、そのアプリの待ちだけ捨てる")

        await engine.commitPendingDeletions()
        let queued = await queue.count
        XCTAssertEqual(queued, 1, "他のアプリの待ちだけが列に入る")
        let aRows = await store.rows(appId: "a", entity: "memo")
        XCTAssertTrue(aRows.isEmpty)
    }
}
