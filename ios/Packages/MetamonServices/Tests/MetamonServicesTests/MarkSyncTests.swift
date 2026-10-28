import XCTest
import MetamonDSL
@testable import MetamonServices

/// #255: 印は押した瞬間にローカルへ反映し、サーバーの `{count, mine}` で確定する。失敗は元に戻す。
final class MarkSyncTests: XCTestCase {
    private var dir: URL!

    override func setUp() {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-marks-\(UUID().uuidString)")
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
    }

    @MainActor
    private func makeEngine(_ client: any AppConfigClient, online: Bool = true) -> (RecordSyncEngine, ManualConnectivity) {
        let connectivity = ManualConnectivity(online: online)
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: client, policy: .immediate)
        return (RecordSyncEngine(store: store, queue: queue, client: client, connectivity: connectivity), connectivity)
    }

    private func seeded() -> InMemoryAppConfigClient {
        InMemoryAppConfigClient(seed: ["recipes": [
            Record(id: "r1", data: ["title": "肉じゃが"], author: "other", marks: ["save": MarkState(count: 2, mine: false)]),
        ]])
    }

    @MainActor
    func testToggleOnAndOff() async throws {
        let client = seeded()
        let (engine, _) = makeEngine(client)
        _ = try await engine.pull(appId: "a", entity: "recipes")

        let on = try await engine.toggleMark(appId: "a", entity: "recipes", recordId: "r1", mark: "save")
        XCTAssertEqual(on, MarkState(count: 3, mine: true))
        var local = await engine.cached(appId: "a", entity: "recipes")
        XCTAssertEqual(local.first?.markState("save"), on)

        let off = try await engine.toggleMark(appId: "a", entity: "recipes", recordId: "r1", mark: "save")
        XCTAssertEqual(off, MarkState(count: 2, mine: false))
        let server = try await client.list(appId: "a", entity: "recipes")
        XCTAssertEqual(server.first?.markState("save"), off)

        // 次の pull(サーバーが正)でも同じ
        local = try await engine.pull(appId: "a", entity: "recipes")
        XCTAssertEqual(local.first?.markState("save"), off)
    }

    @MainActor
    func testFailureRevertsAndOfflineRefuses() async throws {
        let gated = OfflineSimulatingAppConfigClient(inner: seeded())
        let (engine, _) = makeEngine(gated)
        _ = try await engine.pull(appId: "a", entity: "recipes")

        // 端末はオンラインのつもりでもサーバーに届かない → 元に戻す
        gated.setOffline(true)
        do {
            try await engine.toggleMark(appId: "a", entity: "recipes", recordId: "r1", mark: "save")
            XCTFail("must throw")
        } catch {}
        let local = await engine.cached(appId: "a", entity: "recipes")
        XCTAssertEqual(local.first?.markState("save"), MarkState(count: 2, mine: false))

        // 圏外と分かっていれば送らずに断る(送信待ちには積まない)
        let (offline, _) = makeEngine(gated, online: false)
        do {
            try await offline.toggleMark(appId: "a", entity: "recipes", recordId: "r1", mark: "save")
            XCTFail("must throw")
        } catch {
            XCTAssertEqual(error as? MarkError, .offline)
        }
        let pending = await offline.queue.count
        XCTAssertEqual(pending, 0)
    }

    @MainActor
    func testUnsentOwnPostCannotBeMarkedYet() async throws {
        let gated = OfflineSimulatingAppConfigClient(inner: InMemoryAppConfigClient())
        gated.setOffline(true)
        let (engine, _) = makeEngine(gated)
        let shown = await engine.save(appId: "a", entity: "recipes", recordId: nil, data: ["title": "カレー"], author: ("me", nil))
        do {
            try await engine.toggleMark(appId: "a", entity: "recipes", recordId: shown[0].id, mark: "save")
            XCTFail("must throw")
        } catch {
            XCTAssertEqual(error as? MarkError, .notSynced)
        }
    }

    func testLocalRowKeepsMarksThroughCreateSync() async throws {
        let store = RecordStore(directory: dir)
        var row = LocalRecord(appId: "a", entity: "recipes", recordId: "local-x", data: ["title": "x"], pendingOp: .create, clientId: "x")
        row.marks = ["save": MarkState(count: 1, mine: true)]
        await store.upsert(row)
        await store.markSynced(appId: "a", entity: "recipes", localId: "local-x", server: Record(id: "rec_1", data: ["title": "x"]))
        let synced = await store.records(appId: "a", entity: "recipes")
        XCTAssertEqual(synced.first?.markState("save"), MarkState(count: 1, mine: true))
    }
}
