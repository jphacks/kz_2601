import Foundation

/// Firebase App Check のトークン供給(#318)。`HTTP` が ai / appconfig への全リクエストに `X-Firebase-AppCheck` で付ける。
/// 取れなければ nil(ヘッダ無しで送る。サーバーが `APP_CHECK_MODE=off / log` なら通る)。throw しない=App Check の不調で通信を止めない。
public protocol AppCheckTokenProvider: Sendable {
    /// `forceRefresh: true` はサーバーに 401 `app_check_failed` で弾かれた後の取り直し
    func token(forceRefresh: Bool) async -> String?
}

/// App Check のヘッダとエラーコード(backend `internal/appcheck` と同じ値)。
public enum AppCheckHeader {
    public static let name = "X-Firebase-AppCheck"
    /// enforce で弾かれた時の 401 のコード。ID トークンの失効(サインインし直し)とは区別する
    public static let failedCode = "app_check_failed"
}
