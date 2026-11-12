import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 他の端末の変化を Mac に出す(#2072): 変化の通知で取り直す・連投をまとめる・閉じたら購読をやめる・入力中は版の差し替えを待つ。
/// 購読は偽物の `DocumentSync`(`FakeDocumentSync`・通信しない)
@MainActor
final class MacRealtimeSyncTests: XCTestCase {
    private var fake: FakeDocumentSync!
    private var naps: NapCounter!
    private var hub: MacRealtimeSync!

    override func setUp() async throws {
        fake = FakeDocumentSync()
        naps = NapCounter()
        let naps = naps!
        hub = MacRealtimeSync(sync: fake, activity: MacSendActivity(begin: { naps.begin() }, end: { _ in naps.end() }),
                              debounce: .milliseconds(50), deferInterval: .milliseconds(20))
    }

    override func tearDown() async throws {
        hub.stopAll()
    }

    private func eventually(_ message: String, _ condition: () -> Bool) async {
        for _ in 0..<150 {
            if condition() { return }
            try? await Task.sleep(for: .milliseconds(20))
        }
        XCTFail(message)
    }

    private func kakeibo() throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return try AppDSL.load(contentsOf: url)
    }

    private func doc(_ version: Int, appId: String = "a") throws -> AppDocument {
        AppDocument(appId: appId, version: version, dsl: try kakeibo())
    }

    // MARK: - 記録の変化

    func testRecordsChangeIsDeliveredToEveryWindow() async throws {
        let one = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        let two = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        var got: [String] = []
        one.onRecordsChanged = { got.append("one:\($0)") }
        two.onRecordsChanged = { got.append("two:\($0)") }
        await eventually("購読が始まる") { fake.active(appId: "a") == 1 }

        fake.send(.records(entity: "memo"), appId: "a")
        await eventually("両方のウインドウに届く") { got.count == 2 }
        XCTAssertEqual(got.sorted(), ["one:memo", "two:memo"])
    }

    func testBurstOfRecordsChangesIsCoalesced() async throws {
        let lease = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        var got: [String] = []
        lease.onRecordsChanged = { got.append($0) }
        await eventually("購読が始まる") { fake.active(appId: "a") == 1 }

        for _ in 0..<5 { fake.send(.records(entity: "memo"), appId: "a") }
        fake.send(.records(entity: "todo"), appId: "a")
        await eventually("まとめて届く") { got.count >= 2 }
        try? await Task.sleep(for: .milliseconds(150))
        XCTAssertEqual(got.sorted(), ["memo", "todo"], "連投は entity ごとに 1 回にまとめる")
    }

    // MARK: - 購読の本数・やめる

    func testOneSubscriptionPerAppAndStopsWhenLastWindowCloses() async throws {
        let one = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        let two = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        let other = try XCTUnwrap(hub.acquire(appId: "b", version: 3))
        await eventually("アプリごとに 1 本") { fake.active(appId: "a") == 1 && fake.active(appId: "b") == 1 }
        XCTAssertEqual(fake.started(appId: "a"), [1], "同じアプリの 2 つ目のウインドウは張り直さない")
        XCTAssertEqual(fake.started(appId: "b"), [3])
        XCTAssertEqual(hub.activity.depth, 2, "購読している間は App Nap を止める")
        XCTAssertEqual(naps.depth, 1, "activity は 1 つにまとめる")

        one.release()
        one.release() // 2 回返しても 1 回だけ
        XCTAssertTrue(hub.isSubscribed("a"), "まだ開いているウインドウがある")
        XCTAssertEqual(hub.leaseCount(appId: "a"), 1)

        two.release()
        XCTAssertFalse(hub.isSubscribed("a"))
        await eventually("最後のウインドウを閉じたら購読をやめる") { fake.active(appId: "a") == 0 }
        XCTAssertEqual(hub.activity.depth, 1)
        XCTAssertTrue(hub.activity.isActive)

        other.release()
        await eventually("全部閉じた") { fake.active(appId: "b") == 0 }
        XCTAssertFalse(hub.activity.isActive, "購読が無ければ App Nap を止めない")
        XCTAssertEqual(naps.depth, 0)
    }

    func testClosedWindowNoLongerReceives() async throws {
        let keep = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        let closed = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        var kept = 0, leaked = 0
        keep.onRecordsChanged = { _ in kept += 1 }
        closed.onRecordsChanged = { _ in leaked += 1 }
        closed.release()
        await eventually("購読が始まる") { fake.active(appId: "a") == 1 }

        fake.send(.records(entity: "memo"), appId: "a")
        await eventually("開いている方には届く") { kept == 1 }
        XCTAssertEqual(leaked, 0)
    }

    func testNoSyncMeansNoSubscription() {
        let local = MacRealtimeSync(sync: nil)
        XCTAssertNil(local.acquire(appId: "a", version: 1), "appconfig が無い(端末だけ)なら購読しない")
    }

    // MARK: - 版の変化

    func testNewVersionIsAppliedWhenIdle() async throws {
        var applied: [Int] = []
        hub.onDocument = { applied.append($0.version) }
        let lease = try XCTUnwrap(hub.acquire(appId: "a", version: 2))
        lease.isInteracting = { false }
        await eventually("購読が始まる") { fake.active(appId: "a") == 1 }

        fake.send(.document(try doc(2)), appId: "a")
        fake.send(.document(try doc(3)), appId: "a")
        await eventually("新しい版を当てる") { applied == [3] }
        try? await Task.sleep(for: .milliseconds(60))
        XCTAssertEqual(applied, [3], "今の版以下は当てない")
    }

    func testNewVersionWaitsWhileInteracting() async throws {
        var applied: [Int] = []
        hub.onDocument = { applied.append($0.version) }
        let idle = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        let editing = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        var typing = true
        idle.isInteracting = { false }
        editing.isInteracting = { typing }
        await eventually("購読が始まる") { fake.active(appId: "a") == 1 }

        fake.send(.document(try doc(2)), appId: "a")
        fake.send(.document(try doc(3)), appId: "a")
        try? await Task.sleep(for: .milliseconds(150))
        XCTAssertEqual(applied, [], "どれかのウインドウで入力中なら待つ")

        typing = false
        await eventually("入力が終わったら当てる") { !applied.isEmpty }
        try? await Task.sleep(for: .milliseconds(60))
        XCTAssertEqual(applied, [3], "待っている間に来た版は最新の 1 件だけ当てる")
    }

    func testLibraryAppliesOnlyNewerVersionOfServerApp() throws {
        let library = MacAppLibrary(store: nil, serverEnvelope: { _ in nil })
        library.apply(try doc(2), created: true, onServer: true)
        XCTAssertTrue(library.applyRemoteVersion(try doc(3)))
        XCTAssertEqual(library.generated["a"]?.version, 3)
        XCTAssertFalse(library.applyRemoteVersion(try doc(3)), "同じ版は当てない(自分で直した版の通知)")
        XCTAssertFalse(library.applyRemoteVersion(try doc(2)))
        library.apply(try doc(1, appId: "local"), created: true)
        XCTAssertFalse(library.applyRemoteVersion(try doc(5, appId: "local")), "端末だけのアプリには当てない")
    }

    func testRuntimeInteractionDefersHotReload() async throws {
        let rt = MacAppRuntime(appId: "a", dsl: try kakeibo(), ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
        rt.attachRealtime(hub.acquire(appId: "a", version: 1))
        XCTAssertFalse(rt.isInteractingForHotReload)
        rt.sheet = Route(screen: "expense_form", stack: "sheet")
        XCTAssertTrue(rt.isInteractingForHotReload, "シートの入力中は差し替えない")
        rt.sheet = nil
        rt.paths[Route.tabStack("home")] = [Route(screen: "expense_form", stack: Route.tabStack("home"))]
        XCTAssertTrue(rt.isInteractingForHotReload, "push 先を開いている間も待つ")
        rt.realtimeLease?.release()
    }

    // MARK: - 張り直し・取りこぼし

    func testResumeResubscribesFromLatestVersionAndCatchesUp() async throws {
        hub.onDocument = { _ in }
        let lease = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        lease.isInteracting = { false }
        var catchUps = 0
        lease.onCatchUp = { catchUps += 1 }
        await eventually("購読が始まる") { fake.active(appId: "a") == 1 }
        fake.send(.realtime(true), appId: "a")
        fake.send(.document(try doc(4)), appId: "a")
        try? await Task.sleep(for: .milliseconds(60))

        hub.resume(force: false)
        XCTAssertEqual(fake.started(appId: "a"), [1], "購読が生きていれば前に来ただけでは張り直さない")
        XCTAssertEqual(catchUps, 0)

        hub.resume(force: true) // スリープ明け・回線の復帰
        await eventually("張り直す") { fake.started(appId: "a") == [1, 4] }
        await eventually("古い方は閉じる") { fake.active(appId: "a") == 1 }
        XCTAssertEqual(catchUps, 1, "表示中の entity を取り直す")
    }

    func testRealtimeRegainedCatchesUp() async throws {
        let lease = try XCTUnwrap(hub.acquire(appId: "a", version: 1))
        var catchUps = 0
        lease.onCatchUp = { catchUps += 1 }
        await eventually("購読が始まる") { fake.active(appId: "a") == 1 }
        fake.send(.realtime(true), appId: "a")
        fake.send(.realtime(false), appId: "a")
        try? await Task.sleep(for: .milliseconds(60))
        XCTAssertEqual(catchUps, 0)
        fake.send(.realtime(true), appId: "a")
        await eventually("切れて戻ったら取り直す") { catchUps == 1 }
    }

    // MARK: - ランタイム(表示中の entity だけ取り直す)

    func testRuntimeRefetchesShownEntityFromServer() async throws {
        let server = InMemoryAppConfigClient()
        let store = RecordStore(directory: FileManager.default.temporaryDirectory
            .appendingPathComponent("metamon-realtime-test-\(UUID().uuidString)", isDirectory: true))
        let engine = RecordSyncEngine(store: store, queue: SyncQueue(directory: nil, store: store, client: server),
                                      client: server, connectivity: ManualConnectivity(online: true))
        let rt = MacAppRuntime(appId: "a", dsl: try kakeibo(), recordSync: engine,
                               ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
        defer { engine.stop() }
        rt.attachRealtime(hub.acquire(appId: "a", version: 1))
        await rt.load(entities: ["支出"], force: true)
        XCTAssertEqual(rt.records["支出"]?.count, 0)
        await eventually("購読が始まる") { fake.active(appId: "a") == 1 }

        // iPhone で入れた記録
        _ = try await server.create(appId: "a", entity: "支出", data: ["amount": 500, "memo": "iPhone"])
        fake.send(.records(entity: "まだ開いていない"), appId: "a")
        fake.send(.records(entity: "支出"), appId: "a")
        await eventually("表示中の entity を取り直す") { rt.records["支出"]?.count == 1 }
        XCTAssertNil(rt.records["まだ開いていない"], "表示していない entity は読まない")
        rt.realtimeLease?.release()
    }
}

/// 偽物の購読。`events` を開いた版を覚え、`send` で流す
private final class FakeDocumentSync: DocumentSync, @unchecked Sendable {
    private let lock = NSLock()
    private var streams: [UUID: (appId: String, continuation: AsyncStream<SyncUpdate>.Continuation)] = [:]
    private var starts: [String: [Int]] = [:]

    func updates(appId: String, after version: Int) -> AsyncStream<AppDocument> { AsyncStream { $0.finish() } }

    func events(appId: String, after version: Int) -> AsyncStream<SyncUpdate> {
        AsyncStream { continuation in
            let id = UUID()
            lock.withLock {
                streams[id] = (appId, continuation)
                starts[appId, default: []].append(version)
            }
            continuation.onTermination = { [weak self] _ in
                guard let self else { return }
                _ = self.lock.withLock { self.streams.removeValue(forKey: id) }
            }
        }
    }

    func send(_ update: SyncUpdate, appId: String) {
        let targets = lock.withLock { streams.values.filter { $0.appId == appId }.map(\.continuation) }
        for continuation in targets { continuation.yield(update) }
    }

    func active(appId: String) -> Int { lock.withLock { streams.values.filter { $0.appId == appId }.count } }
    func started(appId: String) -> [Int] { lock.withLock { starts[appId] ?? [] } }
}

/// App Nap を止める印の数
@MainActor
private final class NapCounter {
    private(set) var depth = 0
    func begin() -> NSObjectProtocol {
        depth += 1
        return NSObject()
    }
    func end() { depth -= 1 }
}
