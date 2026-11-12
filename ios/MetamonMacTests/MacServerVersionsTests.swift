import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 生成したアプリの「元に戻す / やり直す」をサーバーの版につなぐ(#1279)。届かない時は端末の履歴
@MainActor
final class MacServerVersionsTests: XCTestCase {
    /// サーバーの版の偽物。restore は新しい版を積む(サーバーと同じ)
    private actor FakeVersions: AppConfigClient {
        var docs: [Int: AppDocument] = [:]
        var latest = 0
        var reachable = true
        var restored: [Int] = []

        func push(_ doc: AppDocument) {
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
            return docs.keys.sorted(by: >).map { VersionSummary(appId: appId, version: $0) }
        }

        func restore(appId: String, toVersion version: Int) async throws -> AppDocument {
            guard reachable, var doc = docs[version] else { throw URLError(.notConnectedToInternet) }
            restored.append(version)
            doc.version = latest + 1
            push(doc)
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
