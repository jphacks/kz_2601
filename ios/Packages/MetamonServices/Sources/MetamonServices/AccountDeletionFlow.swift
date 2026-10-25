import Foundation

/// アカウント削除の順番(#119)。UI(Apple の再認証シート・端末の後始末)は呼び出し側がクロージャで渡す。
///
/// 1. `DELETE /v1/me`(サーバーで論理削除)。失敗したら throw して何もしない(やり直せる)
/// 2. Firebase Auth のユーザーを削除(同じ Apple ID で次にサインインすると新しい uid = 新規ユーザー)。
///    `requiresRecentLogin` なら `reauthenticate` してもう一度。それでも失敗したらログだけ残して続ける(サーバーは削除済み)
/// 3. 端末のデータを消す
/// 4. サインアウト(2 が成功していれば Firebase 側は既にサインアウト済み)
public enum AccountDeletionFlow {
    /// - Parameter reauthenticate: 再認証できたら true(匿名・キャンセルは false)
    @MainActor
    public static func run(
        account: any AccountClient,
        auth: any AuthService,
        reauthenticate: @MainActor () async throws -> Bool,
        wipeLocal: @MainActor () async -> Void
    ) async throws {
        try await account.deleteAccount()
        do {
            do {
                try await auth.deleteCurrentUser()
            } catch AuthError.requiresRecentLogin {
                guard try await reauthenticate() else { throw AuthError.requiresRecentLogin }
                try await auth.deleteCurrentUser()
            }
        } catch {
            print("[Metamon] Firebase user delete failed (server already deleted; signing out): \(error)")
        }
        await wipeLocal()
        do { try auth.signOut() } catch { print("[Metamon] signOut after delete failed: \(error)") }
    }
}

/// アカウント削除の前の Sign in with Apple の後始末(#119 / #246・App Store 5.1.1(v))。
/// 直前の Apple 再認証で得た値で Firebase を再認証し、`authorizationCode` で Apple のトークンを失効させる。
/// 失効は Firebase Auth の `revokeToken(withAuthorizationCode:)` が Apple の `auth/token` → `auth/revoke` を代行する
/// (client secret を作る .p8 鍵は Firebase コンソールの Apple プロバイダに登録 = リポジトリ・端末・backend に鍵を置かない)。
/// どちらが失敗しても削除は続ける(結果は戻り値で返し、ログだけ残す)。
public enum AppleTokenRevocation {
    public struct Outcome: Equatable, Sendable {
        public var reauthenticated: Bool
        public var revoked: Bool

        public init(reauthenticated: Bool, revoked: Bool) {
            self.reauthenticated = reauthenticated
            self.revoked = revoked
        }
    }

    @MainActor
    public static func run(auth: any AuthService, authorizationCode: String, idToken: String, nonce: String) async -> Outcome {
        var outcome = Outcome(reauthenticated: false, revoked: false)
        do {
            try await auth.reauthenticateWithApple(idToken: idToken, nonce: nonce)
            outcome.reauthenticated = true
        } catch {
            print("[Metamon] Firebase reauthenticate failed (continuing): \(error)")
        }
        do {
            try await auth.revokeAppleToken(authorizationCode: authorizationCode)
            outcome.revoked = true
        } catch {
            // Firebase の Apple プロバイダに OAuth コードフロー(Services ID・Team ID・Key ID・秘密鍵)が無いと失敗する
            print("[Metamon] Apple token revoke failed (continuing): \(error)")
        }
        return outcome
    }
}
