import XCTest
@testable import MetamonServices

/// 呼び出し順を 1 本のログに記録する。
@MainActor
private final class Log {
    var steps: [String] = []
}

private struct RecordingAccount: AccountClient {
    let log: Log
    var fail = false
    func deleteAccount() async throws {
        await MainActor.run { log.steps.append("server") }
        if fail { throw URLError(.notConnectedToInternet) }
    }
}

/// `deleteCurrentUser` の結果を順に返す認証モック。
private final class RecordingAuth: AuthService, @unchecked Sendable {
    let log: Log
    var deleteResults: [Error?]
    init(log: Log, deleteResults: [Error?]) {
        self.log = log
        self.deleteResults = deleteResults
    }
    var currentUser: AuthUser? { nil }
    func stateChanges() -> AsyncStream<AuthUser?> { AsyncStream { $0.finish() } }
    func signInWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser { fatalError() }
    func signInAnonymously() async throws -> AuthUser { fatalError() }
    func signOut() throws { MainActor.assumeIsolated { log.steps.append("signOut") } }
    func idToken(forceRefresh: Bool) async throws -> String? { nil }
    func deleteCurrentUser() async throws {
        await MainActor.run { log.steps.append("authDelete") }
        if let e = deleteResults.isEmpty ? nil : deleteResults.removeFirst() { throw e }
    }
}

@MainActor
final class AccountDeletionFlowTests: XCTestCase {
    private func run(_ log: Log, account: RecordingAccount, auth: RecordingAuth, reauth: Bool = false) async throws {
        try await AccountDeletionFlow.run(
            account: account, auth: auth,
            reauthenticate: { log.steps.append("reauth"); return reauth },
            wipeLocal: { log.steps.append("wipe") }
        )
    }

    func testOrderServerThenFirebaseThenDevice() async throws {
        let log = Log()
        try await run(log, account: RecordingAccount(log: log), auth: RecordingAuth(log: log, deleteResults: [nil]))
        XCTAssertEqual(log.steps, ["server", "authDelete", "wipe", "signOut"])
    }

    func testServerFailureStopsBeforeAnythingElse() async {
        let log = Log()
        do {
            try await run(log, account: RecordingAccount(log: log, fail: true), auth: RecordingAuth(log: log, deleteResults: [nil]))
            XCTFail("must throw")
        } catch {}
        XCTAssertEqual(log.steps, ["server"], "サーバーが失敗したら Firebase も端末も触らない(やり直せる)")
    }

    func testRequiresRecentLoginReauthenticatesAndRetries() async throws {
        let log = Log()
        let auth = RecordingAuth(log: log, deleteResults: [AuthError.requiresRecentLogin, nil])
        try await run(log, account: RecordingAccount(log: log), auth: auth, reauth: true)
        XCTAssertEqual(log.steps, ["server", "authDelete", "reauth", "authDelete", "wipe", "signOut"])
    }

    func testFirebaseDeleteFailureStillWipesAndSignsOut() async throws {
        let log = Log()
        let auth = RecordingAuth(log: log, deleteResults: [AuthError.requiresRecentLogin])
        try await run(log, account: RecordingAccount(log: log), auth: auth, reauth: false) // 匿名・キャンセル
        XCTAssertEqual(log.steps, ["server", "authDelete", "reauth", "wipe", "signOut"])

        let log2 = Log()
        let other = RecordingAuth(log: log2, deleteResults: [URLError(.timedOut)])
        try await run(log2, account: RecordingAccount(log: log2), auth: other)
        XCTAssertEqual(log2.steps, ["server", "authDelete", "wipe", "signOut"])
    }

    func testMockAuthDeleteSignsOut() async throws {
        let mock = MockAuthService(signedIn: true)
        try await mock.deleteCurrentUser()
        XCTAssertNil(mock.currentUser)
    }
}
