import Foundation

/// アカウント削除(#119・App Store 審査ガイドライン 5.1.1(v))。
/// 実装: `HTTPAccountClient`(`DELETE /v1/me`)/ `MockAccountClient`(何もしない)。
public protocol AccountClient: Sendable {
    /// サーバーでアカウントを論理削除する(アプリは archived・シェア取り消し・掲載取り下げ。以後のリクエストは 410)。冪等。
    func deleteAccount() async throws
}

/// モック・インメモリ構成用。サーバーに何も無いので成功だけ返す。
public struct MockAccountClient: AccountClient {
    public init() {}
    public func deleteAccount() async throws {}
}
