import Foundation

/// 固定文字列を Bearer として送る(開発用)。`AUTH_MODE=dev` の backend は Bearer の値をそのまま uid として扱うので、
/// ローカル appconfig/ai に対して端末固定の uid で E2E できる(`METAMON_DEV_UID`)。本番 Firebase 検証には通らない。
public struct StaticTokenProvider: TokenProvider {
    public let value: String

    public init(_ value: String) { self.value = value }

    public func idToken() async throws -> String? { value }
}
