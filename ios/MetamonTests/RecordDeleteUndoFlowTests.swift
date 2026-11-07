import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 一覧の長押し「削除」→ トーストの「元に戻す」(M11)。
@MainActor
final class RecordDeleteUndoFlowTests: XCTestCase {
    private let appId = "test-undo-\(UUID().uuidString.lowercased())"

    private func runtime() throws -> AppRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"title","name":"題","type":"string"}]}],
         "screens":[{"id":"home","title":"メモ","components":[{"type":"list","entity":"memo","swipeToDelete":true}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        return AppRuntime(document: AppDocument(appId: appId, version: 1, dsl: dsl), services: BuildConfig.make(environment: .mock))
    }

    func testDeleteFromListThenUndoRestoresWithoutSending() async throws {
        let rt = try runtime()
        await rt.recordSync.store.upsert(LocalRecord(appId: appId, entity: "memo", recordId: "m1", data: ["title": "牛乳"], createdAt: "1"))

        let ctx = ActionContext(route: Route(screen: "home", stack: "main"), recordId: "m1")
        await rt.run(Action(type: .deleteRecord, entity: "memo"), ctx)
        XCTAssertEqual(rt.records["memo"]?.count, 0, "一覧からすぐ消える")
        let undo = try XCTUnwrap(rt.recordUndo, "削除しました・元に戻す が出る")
        XCTAssertEqual(undo.ticket.recordId, "m1")
        let queued = await rt.recordSync.queue.count
        XCTAssertEqual(queued, 0, "戻せる間は送信の列に積まない")

        await rt.undoRecordDelete()
        XCTAssertNil(rt.recordUndo)
        XCTAssertEqual(rt.records["memo"]?.map(\.id), ["m1"], "同じ id で戻る")
        await rt.recordSync.commitPendingDeletions()
        let after = await rt.recordSync.queue.count
        XCTAssertEqual(after, 0, "戻した削除は送らない")
    }

    func testUndoAfterCommitDoesNothing() async throws {
        let rt = try runtime()
        await rt.recordSync.store.upsert(LocalRecord(appId: appId, entity: "memo", recordId: "m2", data: [:], createdAt: "1"))
        await rt.run(Action(type: .deleteRecord, entity: "memo"), ActionContext(route: Route(screen: "home", stack: "main"), recordId: "m2"))
        await rt.recordSync.commitPendingDeletions() // 背面に回った・時間切れ
        await rt.undoRecordDelete()
        let shown = await rt.recordSync.cached(appId: appId, entity: "memo")
        XCTAssertTrue(shown.isEmpty, "送った後は戻らない")
    }

    // MARK: - 取り消せる間にログアウト・アカウント削除

    private func deletePending(_ rt: AppRuntime, _ id: String) async {
        await rt.recordSync.store.upsert(LocalRecord(appId: appId, entity: "memo", recordId: id, data: [:], createdAt: "1"))
        await rt.run(Action(type: .deleteRecord, entity: "memo"), ActionContext(route: Route(screen: "home", stack: "main"), recordId: id))
        XCTAssertNotNil(rt.recordUndo)
        XCTAssertEqual(rt.recordSync.pendingDeletions.count, 1)
    }

    func testSettleBeforeSignOutDismissesUndoAndCountsDeleteAsUnsent() async throws {
        let rt = try runtime()
        rt.recordSync.stop() // 送らずに列の件数を見る
        await deletePending(rt, "s1")
        await rt.settlePendingRecordDeletes()
        XCTAssertNil(rt.recordUndo, "「元に戻す」を閉じる")
        XCTAssertTrue(rt.recordSync.pendingDeletions.isEmpty, "待ちを残さない")
        let queued = await rt.recordSync.queue.count
        XCTAssertEqual(queued, 1, "列に積む(オンラインならいまのアカウントで送る)")
        XCTAssertEqual(SignOutFlow.prompt(pending: rt.recordSync.pendingCount, isOnline: false), .unsent(count: 1, canSend: false),
                       "ログアウトの確認に未送信として出る(黙って捨てない)")
    }

    func testDiscardSignOutWithPendingDeleteLeavesNothingForNextAccount() async throws {
        let rt = try runtime()
        rt.recordSync.stop()
        await deletePending(rt, "s2")
        let session = AuthSession(service: rt.services.auth)
        let outcome = try await session.signOutAndWipe(runtime: rt, choice: .discardAndWipe)
        XCTAssertEqual(outcome, .signedOut)
        XCTAssertNil(rt.recordUndo, "ログアウト後に「元に戻す」を出さない")
        XCTAssertTrue(rt.recordSync.pendingDeletions.isEmpty)
        let queued = await rt.recordSync.queue.count
        XCTAssertEqual(queued, 0, "前の人の削除を次の人の列に残さない")
        let rows = await rt.recordSync.store.rows(appId: appId, entity: "memo")
        XCTAssertTrue(rows.isEmpty)
    }

    func testAccountDeletionWithPendingDeleteLeavesNothingForNextAccount() async throws {
        let rt = try runtime()
        rt.recordSync.stop()
        await deletePending(rt, "s3")
        let session = AuthSession(service: rt.services.auth)
        let deleted = try await session.deleteAccount(account: NoopAccount(), runtime: rt)
        XCTAssertTrue(deleted)
        XCTAssertNil(rt.recordUndo)
        XCTAssertTrue(rt.recordSync.pendingDeletions.isEmpty)
        let queued = await rt.recordSync.queue.count
        XCTAssertEqual(queued, 0)
    }

    private final class NoopAccount: AccountClient, @unchecked Sendable {
        func deleteAccount() async throws {}
    }
}
