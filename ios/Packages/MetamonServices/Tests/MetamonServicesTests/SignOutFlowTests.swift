import XCTest
import MetamonDSL
@testable import MetamonServices

/// サインアウトで端末のデータを消す(アカウント削除と同じ後片付け)・未送信の扱い。
@MainActor
final class SignOutFlowTests: XCTestCase {
    private final class Log { var steps: [String] = [] }

    private final class RecordingAuth: AuthService, @unchecked Sendable {
        let log: Log
        var fail = false
        init(log: Log) { self.log = log }
        var currentUser: AuthUser? { nil }
        func stateChanges() -> AsyncStream<AuthUser?> { AsyncStream { $0.finish() } }
        func signInWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser { fatalError() }
        func signInAnonymously() async throws -> AuthUser { fatalError() }
        func signOut() throws {
            MainActor.assumeIsolated { log.steps.append("signOut") }
            if fail { throw URLError(.unknown) }
        }
        func idToken(forceRefresh: Bool) async throws -> String? { nil }
    }

    private func run(_ choice: SignOutFlow.Choice, left: Int, log: Log, auth: RecordingAuth? = nil) async throws -> SignOutFlow.Outcome {
        try await SignOutFlow.run(
            choice: choice, auth: auth ?? RecordingAuth(log: log),
            flush: { log.steps.append("flush"); return left },
            wipeLocal: { log.steps.append("wipe") }
        )
    }

    func testSendThenWipeSendsFirstThenWipesThenSignsOut() async throws {
        let log = Log()
        let outcome = try await run(.sendThenWipe, left: 0, log: log)
        XCTAssertEqual(outcome, .signedOut)
        XCTAssertEqual(log.steps, ["flush", "wipe", "signOut"])
    }

    func testUnsentLeftStopsWithoutWipingOrSigningOut() async throws {
        let log = Log()
        let outcome = try await run(.sendThenWipe, left: 2, log: log)
        XCTAssertEqual(outcome, .unsent(2))
        XCTAssertEqual(log.steps, ["flush"], "送り切れなければ何も消さない(サインインしたまま確認し直す)")
    }

    func testDiscardWipesWithoutSending() async throws {
        let log = Log()
        let outcome = try await run(.discardAndWipe, left: 3, log: log)
        XCTAssertEqual(outcome, .signedOut)
        XCTAssertEqual(log.steps, ["wipe", "signOut"], "捨てる時は送らない")
    }

    func testSignOutFailureThrowsAfterWipe() async {
        let log = Log()
        let auth = RecordingAuth(log: log)
        auth.fail = true
        do {
            _ = try await run(.discardAndWipe, left: 0, log: log, auth: auth)
            XCTFail("must throw")
        } catch {}
        XCTAssertEqual(log.steps, ["wipe", "signOut"])
    }

    func testPromptBranches() {
        XCTAssertEqual(SignOutFlow.prompt(pending: 0, isOnline: true), .plain)
        XCTAssertEqual(SignOutFlow.prompt(pending: 0, isOnline: false), .plain)
        XCTAssertEqual(SignOutFlow.prompt(pending: 2, isOnline: true), .unsent(count: 2, canSend: true))
        XCTAssertEqual(SignOutFlow.prompt(pending: 1, isOnline: false), .unsent(count: 1, canSend: false))
    }

    // MARK: - flush

    func testFlushSendsPendingWhenOnline() async {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-flush-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: dir) }
        let client = InMemoryAppConfigClient()
        let engine = RecordSyncEngine(client: client, connectivity: ManualConnectivity(online: true), directory: dir)
        _ = await engine.save(appId: "a", entity: "memo", recordId: nil, data: ["text": "x"])
        _ = await engine.save(appId: "a", entity: "memo", recordId: nil, data: ["text": "y"])

        let left = await engine.flush(timeout: .seconds(5))

        XCTAssertEqual(left, 0)
        XCTAssertEqual(engine.pendingCount, 0)
        let server = try? await client.list(appId: "a", entity: "memo")
        XCTAssertEqual(server?.count, 2, "消す前に送り切る")
    }

    func testFlushOfflineReturnsPendingImmediately() async {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-flush-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: dir) }
        let engine = RecordSyncEngine(client: InMemoryAppConfigClient(), connectivity: ManualConnectivity(online: false), directory: dir)
        _ = await engine.save(appId: "a", entity: "memo", recordId: nil, data: ["text": "x"])

        let start = ContinuousClock.now
        let left = await engine.flush(timeout: .seconds(5))

        XCTAssertEqual(left, 1)
        XCTAssertLessThan(ContinuousClock.now - start, .seconds(1), "オフラインなら待たない")
    }
}
