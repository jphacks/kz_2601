import XCTest
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
        XCTAssertEqual(session.user?.providers, [.google, .apple])
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

    func testAppleAccountCannotLinkAppleAgain() async throws {
        let auth = MockAuthService(signedIn: false)
        _ = try await auth.signInWithApple(idToken: "t", nonce: "n", fullName: nil)
        let session = AuthSession(service: auth)

        let outcome = await session.linkAppleToCurrentAccount(idToken: "t", nonce: "n", fullName: nil)

        XCTAssertEqual(outcome, .failed(AuthError.alreadyLinked.localizedDescription))
        XCTAssertEqual(session.user?.providers, [.apple])
    }
}
