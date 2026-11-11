import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 同じ作ったアプリを 2 つのウインドウで開いても、記録のエンジンは 1 つ(`MacRecordEngines`)。
/// 別々のエンジンだと後から保存したウインドウがもう片方の足した行を上書きしていた
@MainActor
final class MacRecordEnginesTests: XCTestCase {
    private var root: URL!
    private var engines: MacRecordEngines!

    override func setUp() {
        super.setUp()
        root = FileManager.default.temporaryDirectory.appendingPathComponent("mac-engines-\(UUID().uuidString)")
        engines = MacRecordEngines(root: root)
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: root)
        super.tearDown()
    }

    private func kakeibo() throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return try AppDSL.load(contentsOf: url)
    }

    private func window(_ appId: String) throws -> MacAppRuntime {
        MacAppRuntime(appId: appId, dsl: try kakeibo(), recordLease: engines.acquire(appId: appId),
                      ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    private func memos(_ records: [Record]) -> Set<String> {
        Set(records.compactMap { if case .string(let memo)? = $0.data["memo"] { memo } else { nil } })
    }

    func testTwoWindowsShareOneEngine() throws {
        let a = try window("shared-app")
        let b = try window("shared-app")
        XCTAssertTrue(a.recordSync === b.recordSync)
        XCTAssertEqual(engines.leaseCount(appId: "shared-app"), 2)
        let other = try window("other-app")
        XCTAssertFalse(other.recordSync === a.recordSync, "別のアプリは別のエンジン")
    }

    func testAddsFromBothWindowsSurvive() async throws {
        let a = try window("shared-app")
        let b = try window("shared-app")
        // 両方が表を読み込んでから足す(別々のエンジンだと、ここで持った空の表で上書きしていた)
        await a.load(entities: ["expense"], force: false)
        await b.load(entities: ["expense"], force: false)
        let first = await a.createRecord(entity: "expense", data: ["amount": .number(100), "memo": .string("A")])
        let second = await b.createRecord(entity: "expense", data: ["amount": .number(200), "memo": .string("B")])
        XCTAssertTrue(first)
        XCTAssertTrue(second)

        // ファイルに両方残っている(新しい RecordStore で読み直す)
        let disk = RecordStore(directory: MacRecordVault.directory(appId: "shared-app", root: root)
            .appendingPathComponent("local", isDirectory: true))
        let onDisk = await disk.records(appId: "shared-app", entity: "expense")
        XCTAssertEqual(memos(onDisk), ["A", "B"])
        XCTAssertEqual(memos(a.records(for: "expense")), ["A", "B"])
    }

    func testEventsReachEveryWindow() throws {
        let first = engines.acquire(appId: "shared-app")
        let second = engines.acquire(appId: "shared-app")
        var received: [String] = []
        first.onEvent = { _ in received.append("first") }
        second.onEvent = { _ in received.append("second") }
        first.engine.onEvent?(.drained)
        XCTAssertEqual(received.sorted(), ["first", "second"])

        first.release()
        first.engine.onEvent?(.drained)
        XCTAssertEqual(received.sorted(), ["first", "second", "second"], "返したウインドウには届かない")
    }

    func testLastReleaseFreesEngine() {
        let first = engines.acquire(appId: "shared-app")
        let second = engines.acquire(appId: "shared-app")
        weak var engine = first.engine
        first.release()
        first.release() // 2 回返しても数は 1 つだけ減る
        XCTAssertEqual(engines.leaseCount(appId: "shared-app"), 1)
        XCTAssertNotNil(engines.engine(appId: "shared-app"))
        second.release()
        XCTAssertNil(engines.engine(appId: "shared-app"))
        XCTAssertNil(engine?.onEvent, "止めて通知の配り先も外す")

        let again = engines.acquire(appId: "shared-app")
        XCTAssertEqual(engines.leaseCount(appId: "shared-app"), 1)
        XCTAssertFalse(again.engine === first.engine, "返し終えた後は作り直す")
    }

    func testRuntimeReleasesLeaseWhenClosed() async throws {
        var a: MacAppRuntime? = try window("shared-app")
        let b = try window("shared-app")
        XCTAssertNotNil(a)
        a = nil
        for _ in 0..<100 where engines.leaseCount(appId: "shared-app") != 1 {
            try await Task.sleep(for: .milliseconds(10))
        }
        XCTAssertEqual(engines.leaseCount(appId: "shared-app"), 1, "閉じたウインドウは返す")
        XCTAssertNotNil(engines.engine(appId: "shared-app"), "開いているウインドウがあるうちは残る")
        _ = b
    }

    func testPurgeStopsAndReleasesBeforeRemovingFolder() async throws {
        let a = try window("shared-app")
        _ = await a.createRecord(entity: "expense", data: ["amount": .number(100)])
        let folder = MacRecordVault.directory(appId: "shared-app", root: root)
        XCTAssertTrue(FileManager.default.fileExists(atPath: folder.path))

        await engines.purge(appId: "shared-app")
        XCTAssertNil(engines.engine(appId: "shared-app"))
        XCTAssertFalse(FileManager.default.fileExists(atPath: folder.path), "送信待ちの列を空にした後でフォルダを消す")

        // 開いていたウインドウが後から返しても、作り直したエンジンには触れない
        let reopened = engines.acquire(appId: "shared-app")
        XCTAssertFalse(reopened.engine === a.recordSync)
        XCTAssertEqual(engines.leaseCount(appId: "shared-app"), 1)
    }
}
