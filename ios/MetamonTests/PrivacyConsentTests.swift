import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #246: 第三者 AI への送信の同意・音声モデルのダウンロード確認・Sign in with Apple のトークン失効。
@MainActor
final class PrivacyConsentTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        suite = "priv246-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    // MARK: - 同意の保存と取り消し

    func testConsentIsPersistedAndRevocable() {
        let store = AIConsentStore(defaults: defaults)
        XCTAssertFalse(store.isGranted)
        XCTAssertNil(store.grantedAt)

        let now = Date(timeIntervalSince1970: 1_800_000_000)
        store.grant(now: now)
        XCTAssertTrue(store.isGranted)
        XCTAssertEqual(store.grantedAt, now)
        // 再起動しても残る
        let reloaded = AIConsentStore(defaults: defaults)
        XCTAssertTrue(reloaded.isGranted)
        XCTAssertEqual(reloaded.grantedAt, now)
        XCTAssertTrue(AIConsentStore.isGranted(in: defaults))

        reloaded.revoke()
        XCTAssertFalse(reloaded.isGranted)
        XCTAssertFalse(AIConsentStore(defaults: defaults).isGranted)
        XCTAssertFalse(AIConsentStore.isGranted(in: defaults))
    }

    func testOlderConsentVersionIsNotEnough() {
        defaults.set(AIConsentStore.currentVersion - 1, forKey: "metamon.aiConsent.version")
        XCTAssertFalse(AIConsentStore(defaults: defaults).isGranted)
    }

    func testPromptedFlagAndReset() {
        let store = AIConsentStore(defaults: defaults)
        XCTAssertFalse(store.hasBeenPrompted)
        store.markPrompted()
        store.grant()
        XCTAssertTrue(store.hasBeenPrompted)
        store.resetForTesting()
        XCTAssertFalse(store.hasBeenPrompted)
        XCTAssertFalse(store.isGranted)
    }

    // MARK: - 同意前は生成を呼ばない

    func testGatedClientNeverCallsAIWithoutConsent() async {
        let base = RecordingAI()
        let defaults = self.defaults!
        let gated = ConsentGatedAIClient(base: base) { AIConsentStore.isGranted(in: defaults) }

        do {
            _ = try await gated.morph(transcript: "家計簿", appId: "a", currentVersion: 0, locale: "ja")
            XCTFail("must throw")
        } catch {
            XCTAssertEqual(error as? AIConsentError, .notGranted)
        }
        do {
            _ = try await gated.chat(appId: "a", messages: [], locale: "ja", mode: .create)
            XCTFail("must throw")
        } catch {
            XCTAssertEqual(error as? AIConsentError, .notGranted)
        }
        await gated.resetChat(appId: "a")
        XCTAssertEqual(base.calls, [])

        AIConsentStore(defaults: defaults).grant()
        _ = try? await gated.morph(transcript: "家計簿", appId: "a", currentVersion: 0, locale: "ja")
        XCTAssertEqual(base.calls, ["morph"])
    }

    func testRuntimeShowsConsentInsteadOfGenerating() async throws {
        let base = RecordingAI()
        var services = BuildConfig.make(environment: .mock)
        services.ai = base
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.aiConsent = AIConsentStore(defaults: defaults)

        await rt.generate("家計簿を作って")
        XCTAssertTrue(rt.showAIConsent)
        XCTAssertNil(rt.morphJob)
        _ = await rt.say("家計簿を作って")
        XCTAssertTrue(rt.chatBubbles.isEmpty)
        XCTAssertEqual(base.calls, [])

        rt.grantAIConsent()
        XCTAssertFalse(rt.showAIConsent)
        XCTAssertTrue(rt.hasAIConsent)
        await rt.generate("家計簿を作って")
        XCTAssertEqual(base.calls, ["morph"])

        rt.revokeAIConsent()
        XCTAssertFalse(rt.hasAIConsent)
    }

    // MARK: - 音声モデルのダウンロード確認

    func testModelDownloadAsksOnlyWhenMissingAndNotApproved() {
        XCTAssertEqual(ModelDownloadConsent.decide(needsDownload: false, defaults: defaults), .prepare)
        XCTAssertEqual(ModelDownloadConsent.decide(needsDownload: true, defaults: defaults), .ask)
        ModelDownloadConsent.approve(defaults)
        XCTAssertEqual(ModelDownloadConsent.decide(needsDownload: true, defaults: defaults), .prepare)
        XCTAssertEqual(ModelDownloadConsent.approximateMegabytes(model: "small"), 500)
        XCTAssertEqual(ModelDownloadConsent.approximateMegabytes(model: "openai_whisper-tiny"), 75)
    }

    // MARK: - Sign in with Apple のトークン失効

    func testAppleRevocationReauthenticatesThenRevokesWithCode() async {
        let auth = RecordingAuth()
        let outcome = await AppleTokenRevocation.run(auth: auth, authorizationCode: "code-1", idToken: "id-1", nonce: "n-1")
        XCTAssertEqual(auth.steps, ["reauth:id-1:n-1", "revoke:code-1"])
        XCTAssertEqual(outcome, .init(reauthenticated: true, revoked: true))
    }

    func testAppleRevocationFailureDoesNotStopDeletion() async {
        let auth = RecordingAuth()
        auth.failReauth = true
        auth.failRevoke = true
        let outcome = await AppleTokenRevocation.run(auth: auth, authorizationCode: "code-2", idToken: "id-2", nonce: "n-2")
        XCTAssertEqual(auth.steps, ["reauth:id-2:n-2", "revoke:code-2"]) // 再認証に失敗しても失効は試す
        XCTAssertEqual(outcome, .init(reauthenticated: false, revoked: false))
    }
}

/// 呼ばれたら記録して失敗する ai(結果の中身は要らない)。
private final class RecordingAI: AIClient, @unchecked Sendable {
    private(set) var calls: [String] = []
    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        calls.append("morph")
        throw URLError(.cannotConnectToHost)
    }
    func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
        calls.append("chat")
        throw URLError(.cannotConnectToHost)
    }
    func resetChat(appId: String) async { calls.append("reset") }
}

private final class RecordingAuth: AuthService, @unchecked Sendable {
    var steps: [String] = []
    var failReauth = false
    var failRevoke = false
    var currentUser: AuthUser? { nil }
    func stateChanges() -> AsyncStream<AuthUser?> { AsyncStream { $0.finish() } }
    func signInWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser { fatalError() }
    func signInAnonymously() async throws -> AuthUser { fatalError() }
    func signOut() throws {}
    func idToken(forceRefresh: Bool) async throws -> String? { nil }
    func reauthenticateWithApple(idToken: String, nonce: String) async throws {
        steps.append("reauth:\(idToken):\(nonce)")
        if failReauth { throw AuthError.invalidAppleCredential }
    }
    func revokeAppleToken(authorizationCode: String) async throws {
        steps.append("revoke:\(authorizationCode)")
        if failRevoke { throw URLError(.badServerResponse) }
    }
}
