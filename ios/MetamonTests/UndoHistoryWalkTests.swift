import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 続けて「元に戻す」を押すと版の履歴を遡る(直前 2 つの往復にならない)。本番ログで 7 連打が往復していた不具合。
@MainActor
final class UndoHistoryWalkTests: XCTestCase {
    /// appconfig と同じく restore で新しい版(source.kind=restore, restoredFrom)を積む。
    actor VersionedClient: AppConfigClient {
        struct Entry { var summary: VersionSummary; var doc: AppDocument }
        private(set) var entries: [Entry] = []
        private(set) var restoreCalls: [Int] = []
        let inner = InMemoryAppConfigClient()

        /// `names[i]` の中身で v(i+1) を作る(親は 1 つ前)。
        init(appId: String, names: [String]) throws {
            for (i, name) in names.enumerated() {
                var doc = try DSLExamples.document("kakeibo", version: i + 1)
                doc.appId = appId
                doc.dsl.app.name = name
                entries.append(Entry(summary: VersionSummary(appId: appId, version: i + 1, parentVersion: i == 0 ? nil : i,
                                                             source: .init(kind: i == 0 ? "create" : "edit")), doc: doc))
            }
        }

        var current: AppDocument { entries.last!.doc }

        /// 声の修正を 1 つ積む(戻した後の新しい修正)。
        func edit(name: String) -> AppDocument {
            var doc = current
            doc.version += 1
            doc.dsl.app.name = name
            entries.append(Entry(summary: VersionSummary(appId: doc.appId, version: doc.version, parentVersion: doc.version - 1,
                                                         source: .init(kind: "edit")), doc: doc))
            return doc
        }

        func fetchDocument(appId: String) async throws -> AppDocument { current }
        func listVersions(appId: String) async throws -> [VersionSummary] { entries.reversed().map(\.summary) }
        func restore(appId: String, toVersion version: Int) async throws -> AppDocument {
            restoreCalls.append(version)
            guard let target = entries.first(where: { $0.summary.version == version }) else {
                throw APIError.server(status: 404, code: "not_found", message: "version not found")
            }
            var doc = target.doc
            doc.version = current.version + 1
            entries.append(Entry(summary: VersionSummary(appId: appId, version: doc.version, parentVersion: current.version,
                                                         source: .init(kind: "restore", mode: "full", restoredFrom: version)), doc: doc))
            return doc
        }
        func list(appId: String, entity: String) async throws -> [Record] { [] }
        func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { try await inner.create(appId: appId, entity: entity, data: data) }
        func create(appId: String, entity: String, data: [String: JSONValue], clientId: String) async throws -> Record { try await inner.create(appId: appId, entity: entity, data: data) }
        func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { try await inner.update(appId: appId, entity: entity, id: id, data: data) }
        func delete(appId: String, entity: String, id: String) async throws {}
    }

    private func makeRuntime(_ names: [String]) async throws -> (AppRuntime, VersionedClient) {
        let client = try VersionedClient(appId: "walk-app", names: names)
        var services = BuildConfig.make(environment: .mock)
        services.appConfig = client
        services.isLive = true
        let rt = AppRuntime(document: await client.current, services: services)
        rt.clarifyEnabled = false
        return (rt, client)
    }

    // MARK: 選び方(純粋関数)

    func testTargetSkipsRestoreVersions() {
        let vs = [
            VersionSummary(version: 1, source: .init(kind: "create")),
            VersionSummary(version: 2, parentVersion: 1, source: .init(kind: "edit")),
            VersionSummary(version: 3, parentVersion: 2, source: .init(kind: "edit")),
            VersionSummary(version: 4, parentVersion: 3, source: .init(kind: "restore", restoredFrom: 2)),
        ]
        XCTAssertEqual(UndoWalk.undoTarget(current: 3, versions: vs), 2)
        XCTAssertEqual(UndoWalk.undoTarget(current: 4, versions: vs), 1, "v4 は v2 の中身なので、その 1 つ前の v1 へ")
        XCTAssertNil(UndoWalk.undoTarget(current: 1, versions: vs), "最初の版より前は無い")
    }

    func testTargetWithoutParentFallsBackToPreviousListed() {
        let vs = [VersionSummary(version: 5), VersionSummary(version: 7)]
        XCTAssertEqual(UndoWalk.undoTarget(current: 7, versions: vs), 5)
        XCTAssertNil(UndoWalk.undoTarget(current: 5, versions: vs))
    }

    // MARK: AppRuntime の往復

    /// 3 回直した後に 3 回戻すと最初の版の中身に戻る(往復しない)。
    func testConsecutiveUndosWalkBack() async throws {
        let (rt, client) = try await makeRuntime(["A", "B", "C", "D"])
        XCTAssertEqual(rt.document.version, 4)

        await rt.undo(expectedVersion: rt.document.version)
        XCTAssertEqual(rt.dsl.app.name, "C")
        await rt.undo(expectedVersion: rt.document.version)
        XCTAssertEqual(rt.dsl.app.name, "B", "直前の D に戻らず、さらに前へ")
        await rt.undo(expectedVersion: rt.document.version)
        XCTAssertEqual(rt.dsl.app.name, "A")
        let calls = await client.restoreCalls
        XCTAssertEqual(calls, [3, 2, 1], "restore の版番号は減っていく")
        XCTAssertEqual(rt.document.version, 7)

        await rt.undo(expectedVersion: rt.document.version)
        XCTAssertEqual(rt.toast, L10n.tr("これ以上前の状態はありません"))
        let after = await client.restoreCalls
        XCTAssertEqual(after.count, 3, "最初の版より前へは restore しない")
    }

    func testRedoWalksForwardThenUndoAgain() async throws {
        let (rt, client) = try await makeRuntime(["A", "B", "C", "D"])
        await rt.undo()
        await rt.undo()
        XCTAssertEqual(rt.dsl.app.name, "B")
        XCTAssertEqual(rt.redoStack.count, 2)

        await rt.redo(expectedVersion: rt.document.version)
        XCTAssertEqual(rt.dsl.app.name, "C")
        XCTAssertEqual(rt.redoStack.count, 1, "もう 1 つやり直せる")
        await rt.redo(expectedVersion: rt.document.version)
        XCTAssertEqual(rt.dsl.app.name, "D")
        XCTAssertNil(rt.redoPoint)

        // 元に戻す → やり直す → 元に戻す
        await rt.undo()
        XCTAssertEqual(rt.dsl.app.name, "C")
        await rt.redo()
        XCTAssertEqual(rt.dsl.app.name, "D")
        await rt.undo()
        XCTAssertEqual(rt.dsl.app.name, "C")
        let calls = await client.restoreCalls
        XCTAssertEqual(calls, [3, 2, 3, 4, 3, 4, 3])
    }

    /// 戻した後に新しく直したら、その修正の前(戻した先の中身)へ戻り、やり直すは消える。
    func testEditAfterUndoClearsRedoAndUndoesToRestoredContent() async throws {
        let (rt, client) = try await makeRuntime(["A", "B", "C"])
        await rt.undo()
        await rt.undo()
        XCTAssertEqual(rt.dsl.app.name, "A")
        XCTAssertNotNil(rt.redoPoint)

        let edited = await client.edit(name: "A2")
        rt.apply(edited)
        XCTAssertNil(rt.redoPoint, "新しい修正で「やり直す」は消える")

        await rt.undo()
        XCTAssertEqual(rt.dsl.app.name, "A", "修正の前の中身(A)へ")
        let calls = await client.restoreCalls
        XCTAssertEqual(calls.last, 1)
    }

    /// 同じ版で押された連打は 1 回だけ restore する(in-flight の二重 restore をしない)。
    func testRapidTapsRestoreOnce() async throws {
        let (rt, client) = try await makeRuntime(["A", "B", "C"])
        let tapped = rt.document.version
        async let first: Void = rt.undo(expectedVersion: tapped)
        async let second: Void = rt.undo(expectedVersion: tapped)
        async let third: Void = rt.undo(expectedVersion: tapped)
        _ = await (first, second, third)
        XCTAssertEqual(rt.dsl.app.name, "B")
        let calls = await client.restoreCalls
        XCTAssertEqual(calls, [2], "同じ版を見て押した連打は 1 回")

        // 画面が変わってから押し直せば、さらに前へ
        await rt.undo(expectedVersion: rt.document.version)
        XCTAssertEqual(rt.dsl.app.name, "A")
    }
}
