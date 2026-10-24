import XCTest
import MetamonDSL
@testable import MetamonServices

/// アカウント削除(#119)の端末側の後始末。
@MainActor
final class AccountWipeTests: XCTestCase {
    func testWipeDropsLocalRecordsAndPendingQueue() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-wipe-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: dir) }
        let engine = RecordSyncEngine(client: InMemoryAppConfigClient(), connectivity: ManualConnectivity(online: false), directory: dir)

        _ = await engine.save(appId: "a", entity: "expense", recordId: nil, data: ["amount": 100])
        _ = await engine.save(appId: "b", entity: "memo", recordId: nil, data: ["text": "x"])
        let queued = await engine.queue.count
        XCTAssertEqual(queued, 2, "オフラインなので未送信のまま")

        await engine.wipe()

        let left = await engine.queue.count
        XCTAssertEqual(left, 0)
        let a = await engine.cached(appId: "a", entity: "expense")
        XCTAssertTrue(a.isEmpty)
        XCTAssertEqual(engine.pendingCount, 0)
        XCTAssertFalse(FileManager.default.fileExists(atPath: dir.path), "ファイルも残さない")
        // 開き直しても何も無い(削除したユーザーのデータを後から送らない)
        let reopened = RecordSyncEngine(client: InMemoryAppConfigClient(), connectivity: ManualConnectivity(online: false), directory: dir)
        let reopenedCount = await reopened.queue.count
        XCTAssertEqual(reopenedCount, 0)
    }

    func testMockAccountClientSucceeds() async throws {
        try await MockAccountClient().deleteAccount()
    }
}
