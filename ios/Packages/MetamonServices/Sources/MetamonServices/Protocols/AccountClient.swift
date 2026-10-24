import Foundation

/// アカウント削除(#119・App Store 審査ガイドライン 5.1.1(v))。
/// 実装: `HTTPAccountClient`(`DELETE /v1/me`)/ `MockAccountClient`(何もしない)。
public protocol AccountClient: Sendable {
    /// サーバー上の全データと認証ユーザーを物理削除する。冪等(失敗したらそのまま再実行してよい)。
    func deleteAccount() async throws
}

/// モック・インメモリ構成用。サーバーに何も無いので成功だけ返す。
public struct MockAccountClient: AccountClient {
    public init() {}
    public func deleteAccount() async throws {}
}
