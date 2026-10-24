import Foundation

/// Firebase Auth の ID トークン供給。未ログインなら nil。
/// `HTTP` は毎リクエスト `idToken()` を呼び、401 を受けたら `idToken(forceRefresh: true)` で取り直して 1 回だけ再送する(#60)。
public protocol TokenProvider: Sendable {
    func idToken() async throws -> String?
    /// 失効した可能性のあるトークンを強制更新して返す。更新手段が無い実装(固定トークン / 無認証)は `idToken()` と同じ値でよい
    /// (HTTP 層は「前と同じ値」なら再送しない)。
    func idToken(forceRefresh: Bool) async throws -> String?
}

public extension TokenProvider {
    func idToken(forceRefresh: Bool) async throws -> String? { try await idToken() }
}
