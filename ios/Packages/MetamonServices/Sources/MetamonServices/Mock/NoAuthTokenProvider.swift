import Foundation

/// 認証無し(Firebase 導入まで / モック用)。
public struct NoAuthTokenProvider: TokenProvider {
    public init() {}
    public func idToken() async throws -> String? { nil }
}
