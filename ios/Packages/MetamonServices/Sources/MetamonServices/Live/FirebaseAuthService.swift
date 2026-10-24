import Foundation
import FirebaseAuth
import FirebaseCore

/// Firebase 初期化。`GoogleService-Info.plist` がバンドルに無い環境(plist 未同期・CI)でもクラッシュさせず false を返す。
public enum FirebaseBootstrap {
    /// `FirebaseApp.app()` は未設定時にエラーログを吐くので、自前のフラグで管理する。
    public private(set) static var isConfigured = false

    /// plist があれば `FirebaseApp.configure()`。アプリ起動の最初(`BuildConfig.make` より前)に 1 回呼ぶ。
    @discardableResult
    public static func configureIfAvailable(bundle: Bundle = .main) -> Bool {
        if isConfigured { return true }
        guard let path = bundle.path(forResource: "GoogleService-Info", ofType: "plist"),
              let options = FirebaseOptions(contentsOfFile: path) else {
            print("[Metamon] GoogleService-Info.plist がバンドルにありません。`ios/scripts/sync-firebase-plist.sh` を実行して xcodegen し直してください。認証はモック(固定 uid)で動きます")
            return false
        }
        FirebaseApp.configure(options: options)
        isConfigured = true
        return true
    }
}

/// Firebase Auth 実装。Sign in with Apple(credential 交換)と匿名認証、ID トークン供給。
public final class FirebaseAuthService: AuthService, @unchecked Sendable {
    private let auth: Auth

    /// Firebase 未初期化なら nil(呼び出し側が Mock に落とす)。
    public init?() {
        guard FirebaseBootstrap.isConfigured else { return nil }
        auth = Auth.auth()
    }

    public var currentUser: AuthUser? { auth.currentUser.map(AuthUser.init) }

    public func stateChanges() -> AsyncStream<AuthUser?> {
        AsyncStream { continuation in
            // addStateDidChangeListener は登録直後に現在値を 1 回呼ぶ
            let handle = auth.addStateDidChangeListener { _, user in
                continuation.yield(user.map(AuthUser.init))
            }
            continuation.onTermination = { [auth] _ in auth.removeStateDidChangeListener(handle) }
        }
    }

    public func signInWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser {
        let credential = OAuthProvider.appleCredential(withIDToken: idToken, rawNonce: nonce, fullName: fullName)
        let result = try await auth.signIn(with: credential)
        return AuthUser(result.user)
    }

    public func signInAnonymously() async throws -> AuthUser {
        AuthUser(try await auth.signInAnonymously().user)
    }

    public func signOut() throws { try auth.signOut() }

    public func idToken(forceRefresh: Bool) async throws -> String? {
        guard let user = auth.currentUser else { return nil }
        return try await user.getIDToken(forcingRefresh: forceRefresh)
    }

    /// Firebase 経由で Apple の `auth/revoke` を呼ぶ。Firebase コンソールの Apple プロバイダに
    /// Services ID・Team ID・Key ID・秘密鍵(OAuth コードフロー構成)が設定されていないと失敗する。
    public func revokeAppleToken(authorizationCode: String) async throws {
        try await auth.revokeToken(withAuthorizationCode: authorizationCode)
    }

    public func deleteCurrentUser() async throws {
        guard let user = auth.currentUser else { return }
        do {
            try await user.delete() // 成功すると Firebase がサインアウト状態にする
        } catch let error as NSError where error.code == AuthErrorCode.requiresRecentLogin.rawValue {
            throw AuthError.requiresRecentLogin
        }
    }

    public func reauthenticateWithApple(idToken: String, nonce: String) async throws {
        guard let user = auth.currentUser else { return }
        let credential = OAuthProvider.appleCredential(withIDToken: idToken, rawNonce: nonce, fullName: nil)
        _ = try await user.reauthenticate(with: credential)
    }
}

private extension AuthUser {
    init(_ user: User) {
        self.init(uid: user.uid, isAnonymous: user.isAnonymous, displayName: user.displayName)
    }
}
