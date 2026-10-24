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
