import XCTest
import MetamonDSL
@testable import MetamonServices

/// #180 変身一覧からのアプリ削除: 使用中を消した時の切り替え先・楽観的 UI のロールバック・ローカルの後始末。
@MainActor
final class AppDeletionTests: XCTestCase {
    private let apps = [
        AppSummary(id: "a", name: "家計簿", currentVersion: 3, updatedAt: "2026-10-24T09:00:00Z"),
        AppSummary(id: "b", name: "買い物", currentVersion: 2, updatedAt: "2026-10-25T09:00:00Z"),
        AppSummary(id: "c", name: "日記", currentVersion: 4, updatedAt: "2026-10-23T09:00:00Z"),
        AppSummary(id: "blank", name: "未生成", currentVersion: 1, updatedAt: "2026-10-26T09:00:00Z"),
        AppSummary(id: "old", name: "削除済み", currentVersion: 5, status: "archived", updatedAt: "2026-10-27T09:00:00Z"),
    ]

    private struct Boom: Error {}

    // MARK: fallback

    func testDeletingOtherAppDoesNotSwitch() {
        XCTAssertNil(AppDeletion.fallback(deleting: "a", current: "b", apps: apps))
    }

    func testDeletingCurrentSwitchesToNewestRemaining() {
        // blank(未生成)と archived は候補にしない → 残りで最新の b
        XCTAssertEqual(AppDeletion.fallback(deleting: "a", current: "a", apps: apps), .app("b"))
        XCTAssertEqual(AppDeletion.fallback(deleting: "b", current: "b", apps: apps), .app("a"))
    }

    func testDeletingLastAppGoesHome() {
        let one = [apps[0], apps[3], apps[4]]
        XCTAssertEqual(AppDeletion.fallback(deleting: "a", current: "a", apps: one), .home)
        XCTAssertEqual(AppDeletion.fallback(deleting: "a", current: "a", apps: []), .home)
    }

    // MARK: optimistic UI

    func testHiddenWhileDeletingThenRemovedOnSuccess() async throws {
        let sut = OptimisticAppDeletion()
        var list = apps
        var seenDuringDelete: [String] = []
        try await sut.run("a", delete: {
            seenDuringDelete = sut.filter(list).map(\.id)
        }, onSuccess: {
            XCTAssertTrue(sut.hidden.contains("a"), "成功処理の間は隠したまま(カードが一瞬戻らない)")
            list.removeAll { $0.id == "a" }
        })
        XCTAssertFalse(seenDuringDelete.contains("a"), "サーバー応答を待たずに一覧から消える")
        XCTAssertTrue(sut.hidden.isEmpty)
        XCTAssertFalse(sut.filter(list).contains { $0.id == "a" })
    }

    func testRollbackOnFailure() async {
        let sut = OptimisticAppDeletion()
        var succeeded = false
        do {
            try await sut.run("a", delete: { throw Boom() }, onSuccess: { succeeded = true })
            XCTFail("失敗を投げるはず")
        } catch {
            XCTAssertTrue(error is Boom)
        }
        XCTAssertFalse(succeeded)
        XCTAssertTrue(sut.hidden.isEmpty)
        XCTAssertEqual(sut.filter(apps).map(\.id), apps.map(\.id), "失敗したら一覧に戻る")
    }

    // MARK: clients / local cleanup

    func testInMemoryClientDeleteRemovesFromList() async throws {
        let client = InMemoryAppConfigClient()
        await client.setApps(apps)
        try await client.deleteApp(appId: "a")
        let ids = try await client.listApps().map(\.id)
        XCTAssertFalse(ids.contains("a"))
        XCTAssertEqual(ids.count, apps.count - 1)
    }

    func testPurgeDropsQueueAndLocalRows() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("AppDeletionTests-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: dir) }
        let engine = RecordSyncEngine(client: InMemoryAppConfigClient(), connectivity: ManualConnectivity(online: false), directory: dir)
        _ = await engine.save(appId: "gone", entity: "expense", recordId: nil, data: ["amount": .number(100)])
        _ = await engine.save(appId: "kept", entity: "expense", recordId: nil, data: ["amount": .number(200)])
        let goneBefore = await engine.cached(appId: "gone", entity: "expense")
        XCTAssertEqual(goneBefore.count, 1)

        await engine.purge(appId: "gone")

        let gone = await engine.cached(appId: "gone", entity: "expense")
        let kept = await engine.cached(appId: "kept", entity: "expense")
        let ops = await engine.queue.operations
        XCTAssertTrue(gone.isEmpty)
        XCTAssertEqual(kept.count, 1, "他のアプリのデータは残す")
        XCTAssertEqual(ops.map(\.appId), ["kept"], "消したアプリの未送信は捨てる")
    }
}
