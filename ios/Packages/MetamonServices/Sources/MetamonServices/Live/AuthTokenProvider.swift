import Foundation

/// `AuthService` の ID トークンを Bearer として供給する。`HTTP.send` が毎リクエスト呼ぶ(Firebase 側がキャッシュ/更新を管理)。
public struct AuthTokenProvider: TokenProvider {
    public let auth: any AuthService

    public init(auth: any AuthService) { self.auth = auth }

    public func idToken() async throws -> String? {
        try await auth.idToken(forceRefresh: false)
    }
}
