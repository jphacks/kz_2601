import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #276: 匿名で作ったアプリを Apple ID に引き継ぐ(uid は変わらない)・既に別アカウントにある Apple ID・1 回だけの案内。
@MainActor
final class AccountLinkTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        suite = "link276-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    // MARK: - リンク

    func testLinkKeepsUIDAndLeavesAnonymous() async {
        let session = AuthSession(service: MockAuthService(signedIn: true))
        let before = session.user?.uid
        XCTAssertEqual(session.user?.isAnonymous, true)

        let ok = await session.linkApple(idToken: "t", nonce: "n", fullName: nil)

        XCTAssertTrue(ok)
        XCTAssertEqual(session.user?.uid, before, "uid が変わらない = appconfig のアプリとデータがそのまま")
        XCTAssertEqual(session.user?.isAnonymous, false)
        XCTAssertFalse(session.linkConflict)
        XCTAssertNil(session.linkError)
    }

    func testAppleIDInUseAsksToSwitchThenSwitches() async {
        let auth = MockAuthService(signedIn: true)
        auth.appleIDInUse = true
        let session = AuthSession(service: auth)

        let ok = await session.linkApple(idToken: "t", nonce: "n", fullName: nil)
        XCTAssertFalse(ok)
        XCTAssertTrue(session.linkConflict, "切り替えるかを確認する")
        XCTAssertNil(session.linkError, "確認ダイアログで伝えるので赤字は出さない")
        XCTAssertEqual(session.user?.uid, MockAuthService.fixedUID, "確認するまでは匿名のまま")
        XCTAssertEqual(session.user?.isAnonymous, true)

        let switched = await session.switchToExistingAppleAccount()
        XCTAssertTrue(switched)
        XCTAssertEqual(session.user?.uid, MockAuthService.existingAppleUID)
        XCTAssertEqual(session.user?.isAnonymous, false)
    }

    func testSwitchWithoutConflictFails() async {
        let session = AuthSession(service: MockAuthService(signedIn: true))
        let switched = await session.switchToExistingAppleAccount()
        XCTAssertFalse(switched)
        XCTAssertEqual(session.linkError, AuthError.noPendingAppleAccount.localizedDescription)
        XCTAssertEqual(session.user?.uid, MockAuthService.fixedUID)
    }

    func testAlreadyAppleUserCannotLinkAgain() async {
        let auth = MockAuthService(signedIn: false)
        _ = try? await auth.signInWithApple(idToken: "t", nonce: "n", fullName: nil)
        let session = AuthSession(service: auth)
        let ok = await session.linkApple(idToken: "t", nonce: "n", fullName: nil)
        XCTAssertFalse(ok)
        XCTAssertEqual(session.linkError, AuthError.alreadyLinked.localizedDescription)
    }

    // MARK: - 1 回だけの案内

    private func makeRuntime(anonymous: Bool) throws -> AppRuntime {
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: BuildConfig.make(environment: .mock))
        rt.isAnonymousUser = { anonymous }
        rt.linkPromptDelay = .zero
        return rt
    }

    func testPromptOffersOnceForAnonymous() async throws {
        let rt = try makeRuntime(anonymous: true)
        rt.offerAccountLinkIfNeeded(defaults: defaults)
        try await waitUntil { rt.showLinkPrompt }
        XCTAssertFalse(AccountLinkPrompt.shouldOffer(defaults))

        rt.showLinkPrompt = false
        rt.offerAccountLinkIfNeeded(defaults: defaults)
        try await Task.sleep(for: .milliseconds(100))
        XCTAssertFalse(rt.showLinkPrompt, "2 回目は出さない")
    }

    func testPromptNotOfferedToAppleUser() async throws {
        let rt = try makeRuntime(anonymous: false)
        rt.offerAccountLinkIfNeeded(defaults: defaults)
        try await Task.sleep(for: .milliseconds(100))
        XCTAssertFalse(rt.showLinkPrompt)
        XCTAssertTrue(AccountLinkPrompt.shouldOffer(defaults), "Apple のユーザーには数えない")
    }

    // MARK: - 再インストール後に開くアプリ

    func testLatestAppSkipsBlankArchivedAndCurrent() {
        let apps = [
            AppSummary(id: "blank", name: "", currentVersion: 1, updatedAt: "2026-10-26T00:00:00Z"),
            AppSummary(id: "old", name: "家計簿", currentVersion: 3, updatedAt: "2026-10-20T00:00:00Z"),
            AppSummary(id: "new", name: "筋トレ", currentVersion: 5, updatedAt: "2026-10-25T00:00:00Z"),
            AppSummary(id: "gone", name: "日記", currentVersion: 4, status: "archived", updatedAt: "2026-10-26T00:00:00Z"),
            AppSummary(id: "device", name: "", currentVersion: 1, updatedAt: "2026-10-26T01:00:00Z"),
        ]
        XCTAssertEqual(AppRuntime.latestApp(in: apps, excluding: "device")?.id, "new")
        XCTAssertEqual(AppRuntime.latestApp(in: apps, excluding: "new")?.id, "old")
        XCTAssertNil(AppRuntime.latestApp(in: [], excluding: "device"))
    }

    private func waitUntil(timeout: Duration = .seconds(2), _ condition: () -> Bool) async throws {
        let deadline = ContinuousClock.now + timeout
        while !condition() {
            if ContinuousClock.now > deadline { XCTFail("timed out"); return }
            try await Task.sleep(for: .milliseconds(20))
        }
    }
}
