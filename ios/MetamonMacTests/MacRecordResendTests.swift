import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// オフラインの記録と送り直し(#976): 圏外・サーバーの失敗で積み、つながったら順に 1 回ずつ送る。消したら列も消える
@MainActor
final class MacRecordResendTests: XCTestCase {
    private var root: URL!

    override func setUp() async throws {
        root = FileManager.default.temporaryDirectory
            .appendingPathComponent("metamon-resend-test-\(UUID().uuidString)", isDirectory: true)
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: root)
    }

    private func waitUntil(_ timeout: Duration = .seconds(5), _ condition: () async -> Bool) async {
        let deadline = ContinuousClock.now + timeout
        while ContinuousClock.now < deadline {
            if await condition() { return }
            try? await Task.sleep(for: .milliseconds(20))
        }
    }

    private func save(_ engine: RecordSyncEngine, _ n: Int) async {
        _ = await engine.save(appId: "a", entity: "memo", recordId: nil, data: ["n": .number(Double(n))])
    }

    func testOfflineWriteIsQueuedAndSentOnReconnect() async throws {
        let server = FlakyAppConfigClient()
        let net = ManualConnectivity(online: false)
        let engine = MacRecordVault.recordSync(appId: "a", server: server, root: root, connectivity: net)
        engine.start()
        defer { engine.stop() }

        await save(engine, 1)
        await waitUntil { engine.status == .offline(pending: 1) }
        let queued = await engine.queue.count
        XCTAssertEqual(queued, 1, "圏外では送信待ちに積む")
        let sentWhileOffline = await server.creates
        XCTAssertTrue(sentWhileOffline.isEmpty)
        XCTAssertEqual(MacSyncStatusToolbar.label(engine.status), "オフライン・未送信 1 件")
        XCTAssertTrue(FileManager.default.fileExists(
            atPath: MacRecordVault.directory(appId: "a", root: root).appendingPathComponent("live/queue.json").path),
            "送信待ちはファイルに残る(終了しても消えない)")

        net.set(online: true)
        await waitUntil { await server.creates.count == 1 }
        await waitUntil { engine.status == .synced }
        let left = await engine.queue.count
        XCTAssertEqual(left, 0)
        let sent = await server.creates
        XCTAssertEqual(sent, [1])
    }

    func testQueuedWritesAreSentInOrder() async throws {
        let server = FlakyAppConfigClient()
        let net = ManualConnectivity(online: false)
        let engine = MacRecordVault.recordSync(appId: "a", server: server, root: root, connectivity: net)
        engine.start()
        defer { engine.stop() }
        for n in 1...5 { await save(engine, n) }

        net.set(online: true)
        await waitUntil { await server.creates.count == 5 }
        let sent = await server.creates
        XCTAssertEqual(sent, [1, 2, 3, 4, 5], "積んだ順に送る")
    }

    /// サーバーが落ちていて列が止まっても、送り直し(前面・スリープ明け・一定間隔)で送れる。送っている間だけ App Nap を止める
    func testServerErrorPausesThenResendSendsWithActivity() async throws {
        let server = FlakyAppConfigClient(failing: true)
        let engine = makeEngine(server)
        engine.start()
        defer { engine.stop() }
        await save(engine, 1)
        await waitUntil { await engine.queue.isPaused }
        let paused = await engine.queue.isPaused
        XCTAssertTrue(paused, "連続で失敗したら止まる")
        XCTAssertEqual(engine.pendingCount, 1)

        await server.setFailing(false)
        var began = 0, ended = 0
        let activity = MacSendActivity(begin: { began += 1; return NSObject() }, end: { _ in ended += 1 })
        let resend = MacRecordResend(engines: { [engine] }, activity: activity, timeout: .seconds(5))
        await resend.resend(engine)

        let sent = await server.creates
        XCTAssertEqual(sent, [1])
        let left = await engine.queue.count
        XCTAssertEqual(left, 0)
        XCTAssertEqual(began, 1)
        XCTAssertEqual(ended, 1, "送り終わったら App Nap を戻す")
        XCTAssertFalse(activity.isActive)
    }

    /// 合図が重なっても(前面 + スリープ明け + 書き込み)同じ記録を 2 回送らない
    func testOverlappingResendsDoNotSendTwice() async throws {
        let server = FlakyAppConfigClient(delay: .milliseconds(30))
        let net = ManualConnectivity(online: false)
        let engine = makeEngine(server, connectivity: net)
        engine.start()
        defer { engine.stop() }
        for n in 1...4 { await save(engine, n) }

        let resend = MacRecordResend(engines: { [engine] }, activity: MacSendActivity(begin: { NSObject() }, end: { _ in }))
        net.set(online: true)
        await waitUntil { engine.isOnline }
        async let first: Void = resend.resend(engine)
        async let second: Void = resend.resend(engine)
        resend.resendAll()
        engine.syncNow()
        _ = await (first, second)
        await waitUntil { await engine.queue.count == 0 }
        try await Task.sleep(for: .milliseconds(200))

        let sent = await server.creates
        XCTAssertEqual(sent, [1, 2, 3, 4], "1 件ずつ 1 回だけ")
        XCTAssertEqual(resend.resendCount, 1, "同じエンジンの送り直しは同時に 1 本")
    }

    func testOfflineResendDoesNothing() async {
        let server = FlakyAppConfigClient()
        let engine = makeEngine(server, connectivity: ManualConnectivity(online: false))
        engine.start()
        defer { engine.stop() }
        await save(engine, 1)
        let resend = MacRecordResend(engines: { [engine] }, activity: MacSendActivity(begin: { NSObject() }, end: { _ in }))
        await resend.resend(engine)
        XCTAssertEqual(resend.resendCount, 0)
        let sent = await server.creates
        XCTAssertTrue(sent.isEmpty)
    }

    /// サインアウト・アカウント削除(`MacLocalWipe` → `MacRecordEngines.purgeAll`)で送信待ちも消え、後から送らない
    func testWipeClearsQueue() async throws {
        let server = FlakyAppConfigClient()
        let net = ManualConnectivity(online: false)
        let engine = MacRecordVault.recordSync(appId: "a", server: server, root: root, connectivity: net)
        engine.start()
        await save(engine, 1)
        await save(engine, 2)

        await MacRecordEngines(root: root).purgeAll()
        let left = await engine.queue.count
        XCTAssertEqual(left, 0)
        XCTAssertFalse(FileManager.default.fileExists(atPath: root.path))

        net.set(online: true)
        engine.syncNow()
        try await Task.sleep(for: .milliseconds(200))
        let sent = await server.creates
        XCTAssertTrue(sent.isEmpty, "消した記録は送らない")
    }

    /// アプリを消したらそのアプリの送信待ちも消える
    func testDeletingAppClearsItsQueue() async throws {
        let server = FlakyAppConfigClient()
        let engine = MacRecordVault.recordSync(appId: "a", server: server, root: root,
                                               connectivity: ManualConnectivity(online: false))
        engine.start()
        await save(engine, 1)

        await MacRecordEngines(root: root).purge(appId: "a")
        let left = await engine.queue.count
        XCTAssertEqual(left, 0)
        XCTAssertFalse(FileManager.default.fileExists(atPath: MacRecordVault.directory(appId: "a", root: root).path))
    }

    func testSendActivityNestsIntoOne() {
        var began = 0, ended = 0
        let activity = MacSendActivity(begin: { began += 1; return NSObject() }, end: { _ in ended += 1 })
        activity.begin()
        activity.begin()
        activity.end()
        XCTAssertTrue(activity.isActive)
        activity.end()
        activity.end() // 余分に返しても崩れない
        XCTAssertEqual(began, 1)
        XCTAssertEqual(ended, 1)
        XCTAssertFalse(activity.isActive)
    }

    func testStatusLabelsMatchIOS() {
        XCTAssertEqual(MacSyncStatusToolbar.label(.synced), "最新")
        XCTAssertEqual(MacSyncStatusToolbar.label(.pending(3)), "未送信 3 件")
        XCTAssertEqual(MacSyncStatusToolbar.label(.syncing(2)), "送信中(2 件)")
        XCTAssertEqual(MacSyncStatusToolbar.label(.offline(pending: 0)), "オフライン")
    }

    /// 待ち時間なしで再試行するエンジン(止まるまでを速くする)
    private func makeEngine(_ server: FlakyAppConfigClient,
                            connectivity: any ConnectivityMonitor = ManualConnectivity(online: true)) -> RecordSyncEngine {
        let dir = root.appendingPathComponent(UUID().uuidString, isDirectory: true)
        let store = RecordStore(directory: dir)
        let queue = SyncQueue(directory: dir, store: store, client: server, policy: .immediate)
        return RecordSyncEngine(store: store, queue: queue, client: server, connectivity: connectivity)
    }
}

/// 落とせるサーバー(作った順を覚える)
private actor FlakyAppConfigClient: AppConfigClient {
    private let inner = InMemoryAppConfigClient()
    private var failing: Bool
    private let delay: Duration
    private(set) var creates: [Int] = []

    init(failing: Bool = false, delay: Duration = .zero) {
        self.failing = failing
        self.delay = delay
    }

    func setFailing(_ value: Bool) { failing = value }

    private func check() async throws {
        if delay > .zero { try await Task.sleep(for: delay) }
        if failing { throw APIError.server(status: 503, code: "unavailable", message: "down") }
    }

    func fetchDocument(appId: String) async throws -> AppDocument { try await inner.fetchDocument(appId: appId) }
    func listVersions(appId: String) async throws -> [VersionSummary] { [] }
    func restore(appId: String, toVersion version: Int) async throws -> AppDocument { try await inner.fetchDocument(appId: appId) }
    func list(appId: String, entity: String) async throws -> [Record] {
        try await check()
        return try await inner.list(appId: appId, entity: entity)
    }
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        try await check()
        if case .number(let n) = data["n"] { creates.append(Int(n)) }
        return try await inner.create(appId: appId, entity: entity, data: data)
    }
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        try await check()
        return try await inner.update(appId: appId, entity: entity, id: id, data: data)
    }
    func delete(appId: String, entity: String, id: String) async throws {
        try await check()
        try await inner.delete(appId: appId, entity: entity, id: id)
    }
}
