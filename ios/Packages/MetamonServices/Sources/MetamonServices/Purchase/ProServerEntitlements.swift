// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない。サーバーへの取引の送信も外す。
#if !APP_STORE_REVIEW
import CryptoKit
import Foundation

/// 購入時の `appAccountToken`(#314)。Firebase の uid は UUID ではないので、SHA-256 から UUIDv8 を作る。
/// backend `entitlement.AccountToken` と同じ計算(両方のテストに同じ値を置いている)。
public enum ProAccountToken {
    public static func uuid(for uid: String) -> UUID {
        var b = Array(SHA256.hash(data: Data("tailorit:appAccountToken:\(uid)".utf8)))
        b[6] = b[6] & 0x0f | 0x80  // version 8
        b[8] = b[8] & 0x3f | 0x80  // RFC 9562 variant
        return UUID(
            uuid: (
                b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7],
                b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15]
            ))
    }
}

/// サーバーが保存した Pro の権利(`GET/POST /v1/me/entitlements…`・openapi-appconfig.yaml `Entitlements`)。
public struct ServerEntitlements: Decodable, Equatable, Sendable {
    public struct Pro: Decodable, Equatable, Sendable {
        public var productId: String
        public var environment: String
        /// RFC 3339(小数秒つき)。表示に使う時だけ解釈する
        public var expiresAt: String
    }

    public var isPro: Bool
    public var pro: Pro?
}

/// StoreKit 2 の署名付き取引(JWS)をサーバーに送る(#314)。サーバーが検証して uid の Pro を保存し、ai の回数制限も Pro になる(#449)。
public protocol EntitlementClient: Sendable {
    func submitAppleTransaction(_ signedTransaction: String) async throws -> ServerEntitlements
}

/// `POST /v1/me/entitlements/apple`。422 = 検証に失敗・409 = 別のアカウントに結びついた購読。
public struct HTTPEntitlementClient: EntitlementClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    private struct Body: Encodable { var signedTransaction: String }

    public func submitAppleTransaction(_ signedTransaction: String) async throws -> ServerEntitlements {
        let data = try await HTTP.send(
            "POST", baseURL: baseURL, path: "v1/me/entitlements/apple",
            body: Body(signedTransaction: signedTransaction), token: token, timeout: 20)
        return try JSONDecoder().decode(ServerEntitlements.self, from: data)
    }
}
#endif
