import Foundation

/// Firebase Auth の ID トークン供給。未ログインなら nil。
/// Firebase 導入時は `FirebaseTokenProvider`(Live/)を追加し、`BuildConfig.make(environment:)` で差し替える。
public protocol TokenProvider: Sendable {
    func idToken() async throws -> String?
}
