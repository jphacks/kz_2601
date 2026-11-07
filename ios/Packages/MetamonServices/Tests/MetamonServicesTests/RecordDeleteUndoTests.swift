import XCTest
import MetamonDSL
@testable import MetamonServices

/// 一覧の「削除 → 元に戻す」(M11)。削除の送信を数秒遅らせ、その間に戻せばサーバーには送らない。
@MainActor
final class RecordDeleteUndoTests: XCTestCase {
    private var dir: URL!
    private var client: OfflineSimulatingAppConfigClient!
    private var connectivity: ManualConnectivity!
    private var store: RecordStore!
    private var queue: SyncQueue!
    private var engine: RecordSyncEngine!

    override func setUp() async throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-undo-\(UUID().uuidString)")
        client = OfflineSimulatingAppConfigClient(inner: InMemoryAppConfigClient())
        startEngine(online: true)
    }

    private func startEngine(online: Bool) {
        connectivity = ManualConnectivity(online: online)
        store = RecordStore(directory: dir)
        queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        engine = RecordSyncEngine(store: store, queue: queue, client: client, connectivity: connectivity)
        engine.start()
    }

    override func tearDown() async throws {
        engine.stop()
        try? FileManager.default.removeItem(at: dir)
    }

    /// サーバーに 1 件作り、ローカルにも同期済みで置く。
    private func seed() async throws -> Record {
        let server = try await client.inner.create(appId: "a", entity: "memo", data: ["title": "牛乳"])
        _ = try await engine.pull(appId: "a", entity: "memo")
        return server
    }

    private var sentDeletes: [String] { client.log.filter { $0.hasPrefix("pass delete") } }

    func testUndoBeforeGraceNeverSendsAndKeepsServerRecord() async throws {
        let rec = try await seed()
        let (shown, ticket) = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: rec.id, grace: .seconds(30))
        XCTAssertTrue(shown.isEmpty, "一覧からはすぐ消える")
        let hidden = await engine.cached(appId: "a", entity: "memo")
        XCTAssertTrue(hidden.isEmpty, "読み直しても出ない")
        let queued = await queue.count
        XCTAssertEqual(queued, 0, "待ちの間は送信の列に積まない")

        let restored = await engine.undoDelete(ticket)
        XCTAssertEqual(restored?.map(\.id), [rec.id], "同じ id のまま戻る")
        XCTAssertEqual(restored?.first?.createdAt, rec.createdAt, "作成日時も変わらない")
        XCTAssertTrue(engine.pendingDeletions.isEmpty)
        let again = await engine.undoDelete(ticket)
        XCTAssertNil(again, "2 回目は何もしない")

        await engine.commitPendingDeletions()
        XCTAssertTrue(sentDeletes.isEmpty, "戻した削除はサーバーに送らない")
        let server = try await client.inner.list(appId: "a", entity: "memo")
        XCTAssertEqual(server.map(\.id), [rec.id])
    }

    func testDeleteIsSentAfterGrace() async throws {
        let rec = try await seed()
        let (_, ticket) = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: rec.id, grace: .milliseconds(50))
        try await waitUntil("sent") { !self.sentDeletes.isEmpty && self.engine.status == .synced }
        let server = try await client.inner.list(appId: "a", entity: "memo")
        XCTAssertTrue(server.isEmpty)
        let late = await engine.undoDelete(ticket)
        XCTAssertNil(late, "送った後は戻せない")
        let local = await store.row(appId: "a", entity: "memo", recordId: rec.id)
        XCTAssertNil(local)
    }

    func testOfflineUndoAndOfflineGrace() async throws {
        let kept = try await seed()
        let gone = try await client.inner.create(appId: "a", entity: "memo", data: ["title": "卵"])
        _ = try await engine.pull(appId: "a", entity: "memo")
        client.setOffline(true)
        connectivity.set(online: false)

        // 圏外で消して戻す: 何も積まない
        let (_, t1) = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: kept.id, grace: .seconds(30))
        _ = await engine.undoDelete(t1)
        // 圏外で消して時間が過ぎる: 列に積まれ、送れないまま待つ
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: gone.id, grace: .milliseconds(50))
        try await waitUntil("queued") { self.engine.status == .offline(pending: 1) }
        let shown = await engine.cached(appId: "a", entity: "memo")
        XCTAssertEqual(shown.map(\.id), [kept.id])
        XCTAssertTrue(sentDeletes.isEmpty)

        // 復帰で送られる(戻した方は送らない)
        client.setOffline(false)
        connectivity.set(online: true)
        try await waitUntil("synced") { self.engine.status == .synced }
        XCTAssertEqual(sentDeletes, ["pass delete memo/\(gone.id)"])
        let server = try await client.inner.list(appId: "a", entity: "memo")
        XCTAssertEqual(server.map(\.id), [kept.id])
    }

    func testBackgroundCommitsWithoutWaiting() async throws {
        let rec = try await seed()
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: rec.id, grace: .seconds(30))
        engine.setForeground(false)
        try await waitUntil("sent on background") { !self.sentDeletes.isEmpty }
        XCTAssertTrue(engine.pendingDeletions.isEmpty)
        engine.setForeground(true)
    }

    func testUnsentCreateUndoKeepsRowAndCommitCancelsCreate() async throws {
        // 走行中に切ると、エンジンが知る前の保存が送信中になり、作成を畳めない。最初から圏外で始める
        engine.stop()
        client.setOffline(true)
        startEngine(online: false)
        let made = await engine.save(appId: "a", entity: "memo", recordId: nil, data: ["title": "パン"])
        let (_, t) = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: made[0].id, grace: .seconds(30))
        let restored = await engine.undoDelete(t)
        XCTAssertEqual(restored?.map(\.id), [made[0].id])

        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: made[0].id, grace: .seconds(30))
        await engine.commitPendingDeletions()
        let queued = await queue.count
        XCTAssertEqual(queued, 0, "まだ送っていない作成は取り消すだけ")
        client.setOffline(false)
        connectivity.set(online: true)
        try await waitUntil("synced") { self.engine.status == .synced }
        XCTAssertFalse(client.log.contains { $0.hasPrefix("pass create") || $0.hasPrefix("pass delete") })
    }

    func testCreateSentDuringGraceStaysHiddenAndIsDeleted() async throws {
        // 作成の送信が待ちの間に終わり、仮 id がサーバー id に替わっても同じ行として隠し、時間が来たらサーバー id で消す
        await store.upsert(LocalRecord(appId: "a", entity: "memo", recordId: "local-c1", data: ["title": "x"], createdAt: "9",
                                       pendingOp: .create, clientId: "c1"))
        let (_, t) = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: "local-c1", grace: .seconds(30))
        await store.markSynced(appId: "a", entity: "memo", localId: "local-c1", server: Record(id: "srv1", data: ["title": "x"], createdAt: "9"))
        let shown = await engine.cached(appId: "a", entity: "memo")
        XCTAssertTrue(shown.isEmpty)
        XCTAssertEqual(t.recordId, "local-c1")
        await engine.commitPendingDeletions()
        try await waitUntil("sent") { self.sentDeletes.contains("pass delete memo/srv1") }
    }

    // MARK: - サインアウト・アカウント削除(wipe)と取り消し待ち

    func testCommitPendingDeletionsCountsAsUnsentRightAway() async throws {
        // ログアウトの確認は pendingCount で「未送信あり」を出す。取り消し待ちの削除も積んだ直後から数える
        let rec = try await seed()
        client.setOffline(true)
        connectivity.set(online: false)
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: rec.id, grace: .seconds(30))
        XCTAssertEqual(engine.pendingCount, 0, "待ちの間は列に無い")
        await engine.commitPendingDeletions()
        XCTAssertEqual(engine.pendingCount, 1, "積んだ直後に未送信へ入る(確認を .plain で出して捨てない)")
        XCTAssertTrue(engine.pendingDeletions.isEmpty)
    }

    func testWipeWhileCommitIsInFlightLeavesNothingForTheNextAccount() async throws {
        // 時間切れの送信(列に積む途中)とサインアウトの wipe が重なる: wipe の後に積まれて次の人の列に残っていた
        let rec = try await seed()
        client.setOffline(true)
        connectivity.set(online: false)
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: rec.id, grace: .seconds(30))
        let commit = Task { await engine.commitPendingDeletions() }
        while !engine.pendingDeletions.isEmpty { await Task.yield() } // 待ちから外れ、積む途中に入った
        await engine.wipe()
        await commit.value

        let queued = await queue.count
        XCTAssertEqual(queued, 0, "前の人の削除を次の人の列に残さない")
        let rows = await store.rows(appId: "a", entity: "memo")
        XCTAssertTrue(rows.isEmpty)

        // 次の人がサインインしてオンラインに戻っても、前の人の削除は送られない
        engine.start()
        client.setOffline(false)
        connectivity.set(online: true)
        engine.syncNow()
        try await Task.sleep(for: .milliseconds(200))
        XCTAssertTrue(sentDeletes.isEmpty)
        XCTAssertEqual(engine.pendingCount, 0)
    }

    func testTimerAfterWipeNeverEnqueues() async throws {
        let rec = try await seed()
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: rec.id, grace: .milliseconds(50))
        await engine.wipe()
        engine.start()
        try await Task.sleep(for: .milliseconds(200))
        let queued = await queue.count
        XCTAssertEqual(queued, 0)
        XCTAssertTrue(sentDeletes.isEmpty, "wipe で捨てた削除を時間切れで送らない")
        XCTAssertTrue(engine.pendingDeletions.isEmpty)
    }

    func testSignOutFlushCommitsPendingDeleteUnderCurrentAccountBeforeWipe() async throws {
        let rec = try await seed()
        _ = await engine.deleteUndoably(appId: "a", entity: "memo", recordId: rec.id, grace: .seconds(30))
        let left = await engine.flush(timeout: .seconds(3))
        XCTAssertEqual(left, 0)
        XCTAssertEqual(sentDeletes, ["pass delete memo/\(rec.id)"], "wipe の前に送る(消えたはずの記録が次のサインインで戻らない)")
        await engine.wipe()
        let server = try await client.inner.list(appId: "a", entity: "memo")
        XCTAssertTrue(server.isEmpty)
    }

    private func waitUntil(_ what: String, timeout: Duration = .seconds(5), _ cond: @MainActor () -> Bool) async throws {
        let deadline = ContinuousClock.now + timeout
        while ContinuousClock.now < deadline {
            if cond() { return }
            try await Task.sleep(for: .milliseconds(20))
        }
        XCTFail("timed out waiting for \(what)")
    }
}
