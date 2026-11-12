import XCTest
import MetamonDSL
@testable import MetamonServices

/// #1284 端末だけの appconfig(Mac の作ったアプリの記録): 作り直しても残る・再送は同じ行・アプリを消すと消える。
final class LocalAppConfigClientTests: XCTestCase {
    private var dir: URL!

    override func setUp() {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("local-appconfig-\(UUID().uuidString)", isDirectory: true)
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
    }

    func testRecordsSurviveNewClient() async throws {
        let first = LocalAppConfigClient(directory: dir)
        let made = try await first.create(appId: "a", entity: "memo", data: ["note": "牛乳"])
        _ = try await first.update(appId: "a", entity: "memo", id: made.id, data: ["done": true])

        let reopened = LocalAppConfigClient(directory: dir)
        let rows = try await reopened.list(appId: "a", entity: "memo")
        XCTAssertEqual(rows.map(\.id), [made.id])
        XCTAssertEqual(rows.first?.data["note"], "牛乳")
        XCTAssertEqual(rows.first?.data["done"], true)
    }

    func testCreateWithSameClientIdReturnsSameRecord() async throws {
        let client = LocalAppConfigClient(directory: dir)
        let a = try await client.create(appId: "a", entity: "memo", data: ["note": "x"], clientId: "c1")
        let b = try await client.create(appId: "a", entity: "memo", data: ["note": "x"], clientId: "c1")
        XCTAssertEqual(a.id, b.id)
        let rows = try await client.list(appId: "a", entity: "memo")
        XCTAssertEqual(rows.count, 1)
    }

    func testDeleteAndRemoveAll() async throws {
        let client = LocalAppConfigClient(directory: dir)
        let keep = try await client.create(appId: "a", entity: "memo", data: ["note": "残す"])
        let drop = try await client.create(appId: "a", entity: "memo", data: ["note": "消す"])
        _ = try await client.create(appId: "b", entity: "memo", data: ["note": "別のアプリ"])
        try await client.delete(appId: "a", entity: "memo", id: drop.id)
        let afterDelete = try await client.list(appId: "a", entity: "memo")
        XCTAssertEqual(afterDelete.map(\.id), [keep.id])

        await client.removeAll(appId: "a")
        let gone = try await LocalAppConfigClient(directory: dir).list(appId: "a", entity: "memo")
        let other = try await LocalAppConfigClient(directory: dir).list(appId: "b", entity: "memo")
        XCTAssertTrue(gone.isEmpty)
        XCTAssertEqual(other.count, 1)
    }

    func testMarkIsOnePerson() async throws {
        let client = LocalAppConfigClient(directory: dir)
        let r = try await client.create(appId: "a", entity: "recipes", data: [:])
        let on = try await client.setMark(appId: "a", entity: "recipes", id: r.id, mark: "save", on: true)
        let again = try await client.setMark(appId: "a", entity: "recipes", id: r.id, mark: "save", on: true)
        XCTAssertEqual(on, MarkState(count: 1, mine: true))
        XCTAssertEqual(again, on)
        let off = try await client.setMark(appId: "a", entity: "recipes", id: r.id, mark: "save", on: false)
        XCTAssertEqual(off, MarkState(count: 0, mine: false))
    }

    /// 同期エンジンを通して書き、作り直したエンジンで読み直しても残る(Mac のアプリを終えて開き直す)
    @MainActor
    func testEngineRoundTripAcrossRelaunch() async throws {
        func engine() -> RecordSyncEngine {
            RecordSyncEngine(client: LocalAppConfigClient(directory: dir.appendingPathComponent("server", isDirectory: true)),
                             connectivity: ManualConnectivity(online: true), directory: dir.appendingPathComponent("local", isDirectory: true))
        }
        let first = engine()
        _ = await first.save(appId: "a", entity: "memo", recordId: nil, data: ["note": "残る"])
        _ = await first.flush()

        let second = engine()
        let pulled = try await second.pull(appId: "a", entity: "memo")
        XCTAssertEqual(pulled.map { $0.data["note"] }, ["残る"])
        XCTAssertFalse(pulled.first?.id.hasPrefix("local-") ?? true)
    }
}
