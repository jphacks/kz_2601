import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 生成したアプリの「元に戻す / やり直す」をサーバーの版につなぐ(#1279)。届かない時は端末の履歴
@MainActor
final class MacServerVersionsTests: XCTestCase {
    /// サーバーの版の偽物。restore は新しい版を積む(サーバーと同じく親の版と restore の元を返す)
    private actor FakeVersions: AppConfigClient {
        var docs: [Int: AppDocument] = [:]
        var summaries: [Int: VersionSummary] = [:]
        var latest = 0
        var reachable = true
        var restored: [Int] = []
        /// restore の途中に一度だけ走らせる(待つ間に新しい版が当たる競合を作る)
        var duringRestore: (@Sendable () async -> Void)?

        func setDuringRestore(_ hook: (@Sendable () async -> Void)?) { duringRestore = hook }

        func push(_ doc: AppDocument, restoredFrom: Int? = nil) {
            let parent = latest > 0 ? latest : nil
            let kind = restoredFrom != nil ? "restore" : (parent == nil ? "create" : "edit")
            summaries[doc.version] = VersionSummary(appId: doc.appId, version: doc.version, parentVersion: parent,
                                                    source: .init(kind: kind, restoredFrom: restoredFrom))
            latest = doc.version
            docs[doc.version] = doc
        }

        func setReachable(_ on: Bool) { reachable = on }

        func fetchDocument(appId: String) async throws -> AppDocument {
            guard reachable, let doc = docs[latest] else { throw URLError(.notConnectedToInternet) }
            return doc
        }

        func listVersions(appId: String) async throws -> [VersionSummary] {
            guard reachable else { throw URLError(.notConnectedToInternet) }
            return summaries.keys.sorted(by: >).compactMap { summaries[$0] }
        }

        func restore(appId: String, toVersion version: Int) async throws -> AppDocument {
            guard reachable else { throw URLError(.notConnectedToInternet) }
            if let hook = duringRestore {
                duringRestore = nil
                await hook()
            }
            guard var doc = docs[version] else { throw URLError(.notConnectedToInternet) }
            restored.append(version)
            doc.version = latest + 1
            push(doc, restoredFrom: version)
            return doc
        }

        func list(appId: String, entity: String) async throws -> [Record] { [] }
        func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record { Record(id: "x", data: data) }
        func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record { Record(id: id, data: data) }
        func delete(appId: String, entity: String, id: String) async throws {}
    }

    private let appId = "mac-versions"

    private func document(_ name: String, version: Int) throws -> AppDocument {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        var dsl = try AppDSL.load(contentsOf: url)
        dsl.app.name = name
        return AppDocument(appId: appId, version: version, dsl: dsl)
    }

    private func setUpServerApp() async throws -> (MacAppLibrary, FakeVersions) {
        let server = FakeVersions()
        let library = MacAppLibrary(store: nil, server: server, serverEnvelope: { _ in nil })
        let v1 = try document("家計簿", version: 1)
        let v2 = try document("家計簿 2", version: 2)
        await server.push(v1)
        await server.push(v2)
        library.apply(v1, created: true, onServer: true)
        library.apply(v2, created: false, onServer: true)
        return (library, server)
    }

    func testUndoRestoresPreviousServerVersionAndRedoRestoresBack() async throws {
        let (library, server) = try await setUpServerApp()
        XCTAssertTrue(library.canUndo(appId))

        let undone = await library.undo(appId)
        XCTAssertEqual(undone, .moved)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "家計簿")
        XCTAssertEqual(library.generated[appId]?.version, 3, "restore は新しい版を作る")
        XCTAssertTrue(library.canRedo(appId))

        let redone = await library.redo(appId)
        XCTAssertEqual(redone, .moved)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "家計簿 2")
        let restored = await server.restored
        XCTAssertEqual(restored, [1, 2], "やり直しは戻す前の版をもう一度 restore する")
        XCTAssertFalse(library.canRedo(appId))
    }

    /// 名前を変えた版を順に作る(names[0] が v1)。全部サーバーにもある
    private func setUpServerApp(_ names: [String]) async throws -> (MacAppLibrary, FakeVersions) {
        let server = FakeVersions()
        let library = MacAppLibrary(store: nil, server: server, serverEnvelope: { _ in nil })
        for (i, name) in names.enumerated() {
            let doc = try document(name, version: i + 1)
            await server.push(doc)
            library.apply(doc, created: i == 0, onServer: true)
        }
        return (library, server)
    }

    /// 3 回直した後に 3 回戻すと最初の版の中身に戻る(直前の 2 つを往復しない・#1883)
    func testConsecutiveUndosWalkBackThroughServerVersions() async throws {
        let (library, server) = try await setUpServerApp(["A", "B", "C", "D"])
        for expected in ["C", "B", "A"] {
            let step = await library.undo(appId)
            XCTAssertEqual(step, .moved)
            XCTAssertEqual(library.generated[appId]?.dsl.app.name, expected)
        }
        let restored = await server.restored
        XCTAssertEqual(restored, [3, 2, 1], "restore の版番号は減っていく")
        XCTAssertEqual(library.generated[appId]?.version, 7)

        let step = await library.undo(appId)
        XCTAssertEqual(step, .nothing, "最初の版より前は無い")
        let after = await server.restored
        XCTAssertEqual(after.count, 3)
    }

    /// 元に戻す → やり直す → 元に戻す: 続けて戻した分を 1 つずつやり直し、その後また戻せる
    func testRedoWalksForwardThenUndoAgain() async throws {
        let (library, server) = try await setUpServerApp(["A", "B", "C", "D"])
        _ = await library.undo(appId)
        _ = await library.undo(appId)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "B")

        let first = await library.redo(appId)
        XCTAssertEqual(first, .moved)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "C")
        XCTAssertTrue(library.canRedo(appId), "もう 1 つやり直せる")
        _ = await library.redo(appId)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "D")
        XCTAssertFalse(library.canRedo(appId))
        let none = await library.redo(appId)
        XCTAssertEqual(none, .nothing)

        _ = await library.undo(appId)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "C", "やり直した版(D の中身)の 1 つ前へ")
        let restored = await server.restored
        XCTAssertEqual(restored, [3, 2, 3, 4, 3])
    }

    /// 戻した後に新しい版が当たったら「やり直す」は消える
    func testNewVersionAfterUndoDropsRedoStack() async throws {
        let (library, server) = try await setUpServerApp(["A", "B", "C"])
        _ = await library.undo(appId)
        _ = await library.undo(appId)
        XCTAssertTrue(library.canRedo(appId))
        let latest = await server.latest
        let next = try document("E", version: latest + 1)
        await server.push(next)
        library.apply(next, created: false, onServer: true)
        XCTAssertFalse(library.canRedo(appId))
    }

    /// 戻す restore を待つ間に AI の結果が当たったら、戻した古い版で上書きしない
    func testUndoDoesNotOverwriteVersionAppliedDuringRestore() async throws {
        let (library, server) = try await setUpServerApp(["A", "B", "C", "D"])
        let e = try document("E", version: 5)
        await server.setDuringRestore {
            await server.push(e)
            await MainActor.run { library.apply(e, created: false, onServer: true) }
        }
        let step = await library.undo(appId)
        XCTAssertEqual(step, .failed)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "E")
        XCTAssertEqual(library.generated[appId]?.version, 5)
        XCTAssertFalse(library.canRedo(appId), "新しい版が当たったので「やり直す」は無い")
    }

    /// やり直す restore を待つ間に AI の結果が当たったら、古い「やり直す」を戻さない
    func testRedoDoesNotOverwriteVersionAppliedDuringRestore() async throws {
        let (library, server) = try await setUpServerApp(["A", "B", "C", "D"])
        _ = await library.undo(appId)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "C")
        let latest = await server.latest
        let e = try document("E", version: latest + 1)
        await server.setDuringRestore {
            await server.push(e)
            await MainActor.run { library.apply(e, created: false, onServer: true) }
        }
        let step = await library.redo(appId)
        XCTAssertEqual(step, .failed)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "E")
        XCTAssertFalse(library.canRedo(appId))
    }

    /// 競合(409)で取り直したら「やり直す」は消える(iOS は当てると redoStack を消す)
    func testConflictRefreshDropsServerRedo() async throws {
        let (library, server) = try await setUpServerApp(["A", "B", "C", "D"])
        _ = await library.undo(appId)
        _ = await library.undo(appId)
        XCTAssertTrue(library.canRedo(appId))
        let latest = await server.latest
        await server.push(try document("別の端末で直した", version: latest + 1))
        let version = await library.refreshFromServer(appId)
        XCTAssertEqual(version, latest + 1)
        XCTAssertFalse(library.canRedo(appId))
    }

    /// サーバーで 2 回戻した後に届かなくなっても、端末の履歴で先(C)へ飛ばない
    func testOfflineUndoAfterServerUndosDoesNotJumpForward() async throws {
        let (library, server) = try await setUpServerApp(["A", "B", "C", "D"])
        _ = await library.undo(appId)
        _ = await library.undo(appId)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "B")
        await server.setReachable(false)
        let step = await library.undo(appId)
        let name = library.generated[appId]?.dsl.app.name
        XCTAssertTrue(step == .nothing || name == "A", "got \(step) \(name ?? "nil")")
        XCTAssertNotEqual(name, "C")
    }

    /// 端末の履歴で戻す時も、続けて戻した分を順にやり直せる(#1883)
    func testLocalHistoryRedoesEachConsecutiveUndo() async throws {
        let library = MacAppLibrary(store: nil, server: nil, serverEnvelope: { _ in nil })
        for (i, name) in ["A", "B", "C", "D"].enumerated() {
            library.apply(try document(name, version: i + 1), created: i == 0)
        }
        for expected in ["C", "B", "A"] {
            _ = await library.undo(appId)
            XCTAssertEqual(library.generated[appId]?.dsl.app.name, expected)
        }
        let none = await library.undo(appId)
        XCTAssertEqual(none, .nothing)
        for expected in ["B", "C", "D"] {
            let step = await library.redo(appId)
            XCTAssertEqual(step, .moved)
            XCTAssertEqual(library.generated[appId]?.dsl.app.name, expected)
        }
        XCTAssertFalse(library.canRedo(appId))
        _ = await library.undo(appId)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "C")
    }

    func testFallsBackToLocalHistoryWhenServerUnreachable() async throws {
        let (library, server) = try await setUpServerApp()
        await server.setReachable(false)
        let undone = await library.undo(appId)
        XCTAssertEqual(undone, .moved)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "家計簿", "端末の履歴で戻す")
        XCTAssertEqual(library.generated[appId]?.version, 1)
    }

    func testNothingBeforeFirstServerVersion() async throws {
        let server = FakeVersions()
        let library = MacAppLibrary(store: nil, server: server, serverEnvelope: { _ in nil })
        let v1 = try document("家計簿", version: 1)
        await server.push(v1)
        library.apply(v1, created: true, onServer: true)
        XCTAssertFalse(library.canUndo(appId), "作った直後は戻せない")
        let step = await library.undo(appId)
        XCTAssertEqual(step, .nothing)
    }

    func testLocalOnlyAppUsesLocalHistory() async throws {
        let server = FakeVersions()
        let library = MacAppLibrary(store: nil, server: server, serverEnvelope: { _ in nil })
        library.apply(try document("家計簿", version: 1), created: true)
        library.apply(try document("家計簿 2", version: 2), created: false)
        let undone = await library.undo(appId)
        XCTAssertEqual(undone, .moved)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "家計簿")
        let restored = await server.restored
        XCTAssertTrue(restored.isEmpty, "サーバーに無いアプリはサーバーに聞かない")
    }

    func testConflictRefreshesFromServer() async throws {
        let (library, server) = try await setUpServerApp()
        await server.push(try document("別の端末で直した", version: 5))
        let version = await library.refreshFromServer(appId)
        XCTAssertEqual(version, 5)
        XCTAssertEqual(library.generated[appId]?.dsl.app.name, "別の端末で直した")
    }

    /// 直す会話: 競合ならサーバーの最新を取り直し、「もう一度」は取り直した版で送り直す
    func testComposeConflictRefreshesAndOffersRetry() async throws {
        let ai = ConflictOnceAI()
        let session = MacComposeSession(target: .edit(appId: appId, name: "家計簿", version: 2), ai: ai,
                                        reduceMotion: { true }, isConsentGranted: { true },
                                        clarifyEnabled: false, clarifyAutoBuild: false)
        var refreshed = false
        session.onConflict = {
            refreshed = true
            return 5
        }
        session.build("色を変えて")
        for _ in 0..<100 where session.failure == nil { try await Task.sleep(for: .milliseconds(20)) }
        XCTAssertTrue(refreshed)
        XCTAssertEqual(session.failure?.kind, .failed(message: ErrorCopy.conflictBanner))
        session.retryFailure()
        for _ in 0..<100 {
            if await ai.versions.count >= 2 { break }
            try await Task.sleep(for: .milliseconds(20))
        }
        let versions = await ai.versions
        XCTAssertEqual(versions, [2, 5])
        session.close()
    }

    private actor ConflictOnceAI: AIClient {
        var versions: [Int] = []
        func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
            versions.append(currentVersion)
            if versions.count == 1 { throw APIError.server(status: 409, code: "conflict", message: "version conflict") }
            throw CancellationError()
        }
        func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
            throw CancellationError()
        }
        func resetChat(appId: String) async {}
    }
}
