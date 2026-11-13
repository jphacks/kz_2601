import XCTest
import AuthenticationServices
import MetamonServices
@testable import Metamon

/// 設定の「サインインの方法」(#361・`MacLinkedAccountsSection`)の出す / 出さないの判定と、Apple をつなぐ時の結果。
/// 匿名の人には出さない・Apple がつながっていれば「Apple で続ける」は出さない・別のアカウントの Apple ID はまとめない
@MainActor
final class MacLinkedAccountsTests: XCTestCase {
    private func user(_ providers: [SignInProvider], anonymous: Bool = false) -> AuthUser {
        AuthUser(uid: "u", isAnonymous: anonymous, providers: providers)
    }

    func testSectionHiddenForAnonymousAndSignedOut() {
        XCTAssertFalse(LinkedAccountsRules.showsSection(for: nil), "未ログインには出さない")
        XCTAssertFalse(LinkedAccountsRules.showsSection(for: user([], anonymous: true)), "匿名には出さない")
        XCTAssertTrue(LinkedAccountsRules.showsSection(for: user([.google])))
        XCTAssertTrue(LinkedAccountsRules.showsSection(for: user([.apple])))
    }

    func testOffersAppleOnlyWhenNotLinked() {
        XCTAssertTrue(LinkedAccountsRules.offersApple(to: user([.google])), "Google で作ったアカウントには Apple を足せる")
        XCTAssertFalse(LinkedAccountsRules.offersApple(to: user([.apple])))
        XCTAssertFalse(LinkedAccountsRules.offersApple(to: user([.apple, .google])))
    }

    func testRowsListOnlyLinkedProviders() {
        XCTAssertEqual(LinkedAccountsRules.rows(for: user([.google])), [.google], "つながっていない Apple の行は出さない")
        XCTAssertEqual(LinkedAccountsRules.rows(for: user([.apple, .google])), [.apple, .google])
        XCTAssertEqual(LinkedAccountsRules.rows(for: user([])), [])
    }

    private func account(providers: [SignInProvider], appleIDInUse: Bool = false) -> (MacAccount, MockAuthService) {
        let auth = MockAuthService(signedIn: false)
        auth.signIn(providers: providers, uid: "android-uid")
        auth.appleIDInUse = appleIDInUse
        let account = MacAccount(session: AuthSession(service: auth), account: MockAccountClient(), isLive: false)
        return (account, auth)
    }

    func testCancelledAndFailedAppleSheetDoNotChangeTheAccount() async {
        let (account, _) = account(providers: [.google])
        let cancelled = await account.linkApple(.failure(ASAuthorizationError(.canceled)), nonce: "n")
        XCTAssertEqual(cancelled, .cancelled)
        let failed = await account.linkApple(.failure(ASAuthorizationError(.failed)), nonce: "n")
        guard case .failed = failed else { return XCTFail("失敗は .failed: \(failed)") }
        XCTAssertEqual(account.session.user?.providers, [.google], "Apple はつながっていない")
        XCTAssertEqual(account.session.user?.uid, "android-uid")
    }

    /// 別のアカウントにある Apple ID: 伝えるだけで、切り替えもまとめもしない(iOS と同じ)
    func testAppleIDInUseIsReportedWithoutSwitch() async {
        let (account, _) = account(providers: [.google], appleIDInUse: true)
        let outcome = await account.session.linkAppleToCurrentAccount(idToken: "t", nonce: "n", fullName: nil)
        XCTAssertEqual(outcome, .inUseByOtherAccount)
        XCTAssertEqual(account.session.user?.uid, "android-uid", "いまのアカウントのまま")
        XCTAssertFalse(account.session.linkConflict)
        let user = account.session.user
        XCTAssertEqual(user.map { LinkedAccountsRules.offersApple(to: $0) }, true, "まだ Apple を足せる")
    }
}
