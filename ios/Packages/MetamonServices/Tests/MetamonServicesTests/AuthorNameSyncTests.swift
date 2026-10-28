import XCTest
import MetamonDSL
@testable import MetamonServices

/// #250: 共有 entity に作るレコードは、ローカル行に自分の uid と表示名を入れ、POST で表示名を送る。
final class AuthorNameSyncTests: XCTestCase {
    private var dir: URL!

    override func setUp() {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-author-\(UUID().uuidString)")
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
    }

    @MainActor
    func testCreateCarriesAuthorLocallyAndSendsName() async throws {
        let gated = OfflineSimulatingAppConfigClient(inner: InMemoryAppConfigClient())
        gated.setOffline(true)
        let connectivity = ManualConnectivity(online: false)
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: gated, policy: .immediate)
        let engine = RecordSyncEngine(store: store, queue: queue, client: gated, connectivity: connectivity)
        engine.start()

        let shown = await engine.save(appId: "a", entity: "posts", recordId: nil, data: ["title": "傘"], author: ("u1", "さくら"))
        XCTAssertEqual(shown.first?.author, "u1", "未送信のうちから自分の投稿(作者判定・自分の投稿だけ)")
        XCTAssertEqual(shown.first?[RecordValue.author], "さくら")
        let ops = await queue.operations
        XCTAssertEqual(ops.map(\.authorName), ["さくら"])

        // 更新では表示名を送らない(backend は create の時だけ受け取る)
        _ = await engine.save(appId: "a", entity: "posts", recordId: shown[0].id, data: ["title": "青い傘"], author: ("u1", "さくら"))
        let merged = await queue.operations
        XCTAssertEqual(merged.count, 1, "create に畳まれる")
        XCTAssertEqual(merged.first?.authorName, "さくら")

        gated.setOffline(false)
        connectivity.set(online: true)
        try await waitUntil { engine.status == .synced }
        let server = try await gated.inner.list(appId: "a", entity: "posts")
        XCTAssertEqual(server.first?.authorName, "さくら")
        let local = await engine.cached(appId: "a", entity: "posts")
        XCTAssertEqual(local.first?.authorName, "さくら", "サーバーの行に差し替わっても名前が残る")
        engine.stop()
    }

    @MainActor
    func testAnonymousSendsNoNameAndPrivateHasNoAuthor() async throws {
        let engine = RecordSyncEngine(client: InMemoryAppConfigClient(), connectivity: ManualConnectivity(online: false), directory: dir)
        let anon = await engine.save(appId: "a", entity: "posts", recordId: nil, data: ["title": "鍵"], author: ("u1", nil))
        XCTAssertEqual(anon.first?[RecordValue.author], .string(RecordSyncEngine.anonymousName), "backend と同じく「匿名」と出す")
        let ops = await engine.queue.operations
        XCTAssertEqual(ops.count, 1)
        XCTAssertNil(ops.first?.authorName, "名前が無ければ送らない")

        let mine = await engine.save(appId: "a", entity: "diary", recordId: nil, data: ["text": "日記"])
        XCTAssertNil(mine.first?.author)
        XCTAssertEqual(mine.first?[RecordValue.author], .null)
    }

    /// 以前のビルドが書いた queue.json(authorName キー無し)も読める。
    func testOldQueueFileDecodes() throws {
        let json = #"{"id":"c1","appId":"a","entity":"e","recordId":"local-c1","kind":"create","data":{},"attempts":0,"enqueuedAt":0}"#
        let op = try JSONDecoder().decode(SyncOperation.self, from: Data(json.utf8))
        XCTAssertNil(op.authorName)
    }

    private func waitUntil(timeout: Duration = .seconds(5), _ cond: @MainActor () -> Bool) async throws {
        let deadline = ContinuousClock.now + timeout
        while ContinuousClock.now < deadline {
            if await cond() { return }
            try await Task.sleep(for: .milliseconds(20))
        }
        XCTFail("timed out")
    }
}
