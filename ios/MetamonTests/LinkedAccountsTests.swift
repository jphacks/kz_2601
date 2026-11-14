import XCTest
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// #361: 既存のアカウントに Apple をつなぐ・別のアカウントにある Apple ID はまとめない・つながっている方法の表示。
@MainActor
final class LinkedAccountsTests: XCTestCase {
    func testProvidersFromFirebaseIDsKeepOnlyKnownOnes() {
        XCTAssertEqual(SignInProvider.from(providerIDs: ["password", "google.com", "apple.com"]), [.apple, .google])
        XCTAssertEqual(SignInProvider.from(providerIDs: []), [])
    }

    func testAppleSignInShowsAppleLinked() async throws {
        let auth = MockAuthService(signedIn: false)
        _ = try await auth.signInWithApple(idToken: "t", nonce: "n", fullName: nil)
        XCTAssertEqual(auth.currentUser?.providers, [.apple])
    }

    /// Android の Google で作ったアカウントに Apple を足す: uid はそのまま・両方がつながる
    func testLinkAppleToGoogleAccountKeepsUID() async {
        let auth = MockAuthService(signedIn: false)
        auth.signIn(providers: [.google], uid: "android-uid")
        let session = AuthSession(service: auth)

        let outcome = await session.linkAppleToCurrentAccount(idToken: "t", nonce: "n", fullName: nil)

        XCTAssertEqual(outcome, .linked)
        XCTAssertEqual(session.user?.uid, "android-uid")
        XCTAssertEqual(session.user?.providers, [.apple, .google], "Firebase と同じ並び")
        XCTAssertEqual(session.user?.isAnonymous, false)
    }

    /// 別のアカウントにある Apple ID: 伝えるだけで、切り替えもまとめもしない
    func testAppleIDInUseIsReportedWithoutMergeOrSwitch() async throws {
        let auth = MockAuthService(signedIn: false)
        auth.signIn(providers: [.google], uid: "android-uid")
        auth.appleIDInUse = true
        let session = AuthSession(service: auth)

        let outcome = await session.linkAppleToCurrentAccount(idToken: "t", nonce: "n", fullName: nil)

        XCTAssertEqual(outcome, .inUseByOtherAccount)
        XCTAssertEqual(session.user?.uid, "android-uid", "いまのアカウントのまま")
        XCTAssertEqual(session.user?.providers, [.google], "Apple はつながっていない")
        XCTAssertFalse(session.linkConflict, "匿名向けの切り替えの確認は出さない")
        do {
            _ = try await auth.switchToPendingAppleAccount()
            XCTFail("既存アカウントでは切り替え先を残さない")
        } catch AuthError.noPendingAppleAccount {}
    }

    /// 既に Apple がつながっている(alreadyLinked)は失敗にせず「つながっています」
    func testAppleAccountLinkingAgainIsTreatedAsLinked() async throws {
        let auth = MockAuthService(signedIn: false)
        _ = try await auth.signInWithApple(idToken: "t", nonce: "n", fullName: nil)
        let session = AuthSession(service: auth)

        let outcome = await session.linkAppleToCurrentAccount(idToken: "t", nonce: "n", fullName: nil)

        XCTAssertEqual(outcome, .linked)
        XCTAssertEqual(session.user?.providers, [.apple])
    }

    /// 別の端末で Apple をつないだ後(手元の providers が古い): alreadyLinked で読み直し、つながった表示にする
    func testStaleProvidersAreReloadedOnAlreadyLinked() async {
        let auth = MockAuthService(signedIn: false)
        auth.signIn(providers: [.google], uid: "android-uid")
        let session = AuthSession(service: auth)
        auth.signIn(providers: [.google, .apple], uid: "android-uid") // 別の端末でつないだ
        XCTAssertEqual(session.user?.providers, [.google], "まだ古い")

        let outcome = await session.linkAppleToCurrentAccount(idToken: "t", nonce: "n", fullName: nil)

        XCTAssertEqual(outcome, .linked)
        XCTAssertEqual(session.user?.providers, [.apple, .google])
        XCTAssertFalse(LinkedAccountsSection.offersApple(to: try! XCTUnwrap(session.user)))
    }

    /// 設定を開いた時の読み直しで、別の端末でつないだ方法が出る
    func testRefreshUserPicksUpProvidersLinkedElsewhere() async {
        let auth = MockAuthService(signedIn: false)
        auth.signIn(providers: [.google], uid: "android-uid")
        let session = AuthSession(service: auth)
        auth.signIn(providers: [.apple, .google], uid: "android-uid")

        await session.refreshUser()

        XCTAssertEqual(session.user?.providers, [.apple, .google])
    }

    /// 訳していない失敗(Firebase の英語の文など)はそのまま出さず、訳した決まり文句にする
    func testUnmappedLinkErrorShowsLocalizedGenericMessage() async {
        let saved = LanguagePreference.shared.setting
        defer { LanguagePreference.shared.setting = saved }
        let auth = FailingLinkAuthService()
        auth.signIn(providers: [.google])
        let session = AuthSession(service: auth)

        LanguagePreference.shared.setting = .ja
        let ja = await session.linkAppleToCurrentAccount(idToken: "t", nonce: "n", fullName: nil)
        XCTAssertEqual(ja, .failed("Apple をつなげませんでした"))
        LanguagePreference.shared.setting = .en
        let en = await session.linkAppleToCurrentAccount(idToken: "t", nonce: "n", fullName: nil)
        XCTAssertEqual(en, .failed("Couldn't connect Apple"))
        XCTAssertEqual(session.user?.providers, [.google])
    }
    /// iOS で作ったアカウント(Apple)は Apple の行だけ・ボタンなし
    func testAppleAccountShowsOnlyAppleRowAndNoButton() async throws {
        let auth = MockAuthService(signedIn: false)
        let user = try await auth.signInWithApple(idToken: "t", nonce: "n", fullName: nil)
        XCTAssertEqual(LinkedAccountsSection.rows(for: user), [.apple])
        XCTAssertFalse(LinkedAccountsSection.offersApple(to: user))
    }

    /// Android の Google で作ったアカウント: Google の行と「Apple で続ける」。つないだ後はボタンが消える
    func testGoogleAccountShowsGoogleRowAndAppleButtonUntilLinked() async {
        let auth = MockAuthService(signedIn: false)
        auth.signIn(providers: [.google], uid: "android-uid")
        let session = AuthSession(service: auth)
        let before = try! XCTUnwrap(session.user)
        XCTAssertEqual(LinkedAccountsSection.rows(for: before), [.google])
        XCTAssertTrue(LinkedAccountsSection.offersApple(to: before))

        _ = await session.linkAppleToCurrentAccount(idToken: "t", nonce: "n", fullName: nil)

        let after = try! XCTUnwrap(session.user)
        XCTAssertEqual(LinkedAccountsSection.rows(for: after), [.apple, .google])
        XCTAssertFalse(LinkedAccountsSection.offersApple(to: after))
    }

    /// つながっている方法が無い(Apple も Google も無い)時は行を出さない
    func testNoKnownProviderShowsNoRows() {
        let user = AuthUser(uid: "u", isAnonymous: false)
        XCTAssertEqual(LinkedAccountsSection.rows(for: user), [])
        XCTAssertTrue(LinkedAccountsSection.offersApple(to: user))
    }

    /// Apple ID のメールが別のアカウントにある(Firebase 17007 / 17012): 訳した結果を返し、何も変えない
    func testAppleEmailInUseIsLocalizedOutcomeWithoutChange() async throws {
        let auth = MockAuthService(signedIn: false)
        auth.signIn(providers: [.google], uid: "android-uid")
        auth.appleEmailInUse = true
        let session = AuthSession(service: auth)

        let outcome = await session.linkAppleToCurrentAccount(idToken: "t", nonce: "n", fullName: nil)

        XCTAssertEqual(outcome, .emailInUseByOtherAccount)
        XCTAssertEqual(session.user?.uid, "android-uid")
        XCTAssertEqual(session.user?.providers, [.google])
        XCTAssertFalse(session.linkConflict)
        do {
            _ = try await auth.switchToPendingAppleAccount()
            XCTFail("切り替え先を残さない")
        } catch AuthError.noPendingAppleAccount {}
    }

    /// Firebase の英語の文ではなく、訳した文が出る
    func testAppleEmailInUseErrorCopyIsLocalized() {
        let saved = LanguagePreference.shared.setting
        defer { LanguagePreference.shared.setting = saved }
        LanguagePreference.shared.setting = .ja
        XCTAssertEqual(AuthError.appleEmailInUse.errorDescription, "この Apple ID のメールアドレスは、すでに別のアカウントで使われています")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(AuthError.appleEmailInUse.errorDescription, "The email address for this Apple ID is already used by another account")
    }
}

/// link で Firebase の訳していないエラー(英語の文)を返すモック
private final class FailingLinkAuthService: AuthService, @unchecked Sendable {
    private let mock = MockAuthService(signedIn: false)
    var currentUser: AuthUser? { mock.currentUser }
    func stateChanges() -> AsyncStream<AuthUser?> { mock.stateChanges() }
    func signInWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser {
        try await mock.signInWithApple(idToken: idToken, nonce: nonce, fullName: fullName)
    }
    func signInAnonymously() async throws -> AuthUser { try await mock.signInAnonymously() }
    func signOut() throws { try mock.signOut() }
    func idToken(forceRefresh: Bool) async throws -> String? { nil }
    func signIn(providers: [SignInProvider]) { mock.signIn(providers: providers) }
    func linkWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser {
        throw NSError(domain: "FIRAuthErrorDomain", code: 17999,
                      userInfo: [NSLocalizedDescriptionKey: "An internal error has occurred, print and inspect the error details for more information."])
    }
}
