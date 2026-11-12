import Foundation
import FirebaseAuth
import FirebaseCore

/// Firebase 初期化。`GoogleService-Info.plist` がバンドルに無い環境(plist 未同期・CI)でもクラッシュさせず false を返す。
public enum FirebaseBootstrap {
    /// `FirebaseApp.app()` は未設定時にエラーログを吐くので、自前のフラグで管理する。
    public private(set) static var isConfigured = false

    /// plist があれば `FirebaseApp.configure()`。アプリ起動の最初(`BuildConfig.make` より前)に 1 回呼ぶ。
    /// `appCheck`(#318): App Check のプロバイダ。`.off` 以外なら ai / appconfig へのリクエストに `X-Firebase-AppCheck` が付く。
    @discardableResult
    public static func configureIfAvailable(bundle: Bundle = .main, appCheck: AppCheckMode = .off) -> Bool {
        if isConfigured { return true }
        guard let path = bundle.path(forResource: "GoogleService-Info", ofType: "plist"),
              let options = FirebaseOptions(contentsOfFile: path) else {
            print("[Metamon] GoogleService-Info.plist がバンドルにありません。`ios/scripts/sync-firebase-plist.sh` を実行して xcodegen し直してください。認証はモック(固定 uid)で動きます")
            return false
        }
        FirebaseAppCheckSetup.installFactory(appCheck) // configure より前(Firebase の決まり)
        FirebaseApp.configure(options: options)
        if appCheck != .off { HTTP.appCheck = FirebaseAppCheckTokenProvider() }
        isConfigured = true
        return true
    }

    /// 認証の入口(ここだけで `Auth` を作る)。`appName` が nil なら既定のアプリ(`Auth.auth()`・今までどおり)。
    /// 名前があれば、既定のアプリと同じ options で名前付きのアプリを 1 回だけ作り、その `Auth` を返す
    /// (サインインの状態はアプリの名前ごとに Firebase がキーチェーンに持つ・#2084 の試験用の枠)。
    static func auth(appName: String?) -> Auth? {
        guard isConfigured else { return nil }
        guard let appName else { return Auth.auth() }
        if let app = FirebaseApp.app(name: appName) { return Auth.auth(app: app) }
        guard let options = FirebaseApp.app()?.options.copy() as? FirebaseOptions else { return nil }
        FirebaseApp.configure(name: appName, options: options)
        return FirebaseApp.app(name: appName).map { Auth.auth(app: $0) }
    }

    /// 名前付きのアプリ(nil は既定)でサインインしている人。切り替えずに見るだけ。Firebase 未初期化なら nil
    public static func currentUser(appName: String?) -> AuthUser? {
        auth(appName: appName)?.currentUser.map(AuthUser.init)
    }
}

/// Firebase Auth 実装。Sign in with Apple(credential 交換)と匿名認証、ID トークン供給。
public final class FirebaseAuthService: AuthService, @unchecked Sendable {
    private let auth: Auth
    /// 引き継ぎ(#276)で credentialAlreadyInUse になった時の Apple の資格情報(Apple の nonce は 1 回限りなので、Firebase が返す更新済みのものを使う)。
    private let lock = NSLock()
    private var pendingAppleCredential: AuthCredential?

    /// Firebase 未初期化なら nil(呼び出し側が Mock に落とす)。
    /// `appName`: nil = 既定のアプリ(今までどおり)。名前を渡すとその名前付きアプリの別のサインイン状態を使う(#2084)
    public init?(appName: String? = nil) {
        guard let auth = FirebaseBootstrap.auth(appName: appName) else { return nil }
        self.auth = auth
    }

    /// 使っている Firebase のアプリの名前(既定は `__FIRAPP_DEFAULT`)
    public var firebaseAppName: String? { auth.app?.name }

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

    public func linkWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser {
        guard let user = auth.currentUser else {
            return try await signInWithApple(idToken: idToken, nonce: nonce, fullName: fullName)
        }
        // 匿名 → Apple(#276)と、Apple 以外の既存アカウントに Apple を足す(#361)の両方。統合はしない
        guard !user.providerData.contains(where: { $0.providerID == SignInProvider.apple.rawValue }) else { throw AuthError.alreadyLinked }
        let credential = OAuthProvider.appleCredential(withIDToken: idToken, rawNonce: nonce, fullName: fullName)
        do {
            let result = try await user.link(with: credential)
            try? await result.user.reload() // isAnonymous / providerData を最新に
            return AuthUser(auth.currentUser ?? result.user)
        } catch let error as NSError where error.code == AuthErrorCode.credentialAlreadyInUse.rawValue {
            let updated = error.userInfo[AuthErrorUserInfoUpdatedCredentialKey] as? AuthCredential
            // 切り替えを出すのは匿名の時だけ(#276)。既存アカウントは切り替えもまとめもしない(#361)
            lock.withLock { pendingAppleCredential = user.isAnonymous ? updated : nil }
            throw AuthError.credentialAlreadyInUse
        } catch let error as NSError where error.code == AuthErrorCode.providerAlreadyLinked.rawValue {
            throw AuthError.alreadyLinked
        } catch let error as NSError where error.code == AuthErrorCode.emailAlreadyInUse.rawValue
                    || error.code == AuthErrorCode.accountExistsWithDifferentCredential.rawValue {
            throw AuthError.appleEmailInUse // Firebase の英語の文をそのまま出さない
        }
    }

    public func switchToPendingAppleAccount() async throws -> AuthUser {
        let credential = lock.withLock { () -> AuthCredential? in
            defer { pendingAppleCredential = nil }
            return pendingAppleCredential
        }
        guard let credential else { throw AuthError.noPendingAppleAccount }
        return AuthUser(try await auth.signIn(with: credential).user)
    }

    /// `providerData` は手元のキャッシュなので、別の端末でつないだ方法はサーバーから読み直さないと見えない(#361)。
    public func reloadCurrentUser() async throws -> AuthUser? {
        guard let user = auth.currentUser else { return nil }
        try await user.reload()
        return AuthUser(auth.currentUser ?? user)
    }
}

private extension AuthUser {
    init(_ user: User) {
        self.init(uid: user.uid, isAnonymous: user.isAnonymous, displayName: user.displayName,
                  providers: SignInProvider.from(providerIDs: user.providerData.map(\.providerID)))
    }
}
