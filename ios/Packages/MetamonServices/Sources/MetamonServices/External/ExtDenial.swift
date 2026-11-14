import Foundation

/// 外の API(#1493)の op が断った理由(#2001)。画面はこれを見て「次にすること」(つなぐ・Pro・コイン)を出す。
/// 402 の判定は `ProRequired` / `CoinDenial` をそのまま使う(同じ判定を 2 度書かない)。
public enum ExtDenial: Equatable, Sendable {
    /// 409 `ext_not_connected`。`details.provider` が無い古いサーバーは nil
    case notConnected(provider: String?)
    /// 402 `pro_required`(外の API は Pro だけ)
    case proRequired(ProRequired)
    #if !APP_STORE_REVIEW // 審査用の版はコインを持たない(#450)。コインの断りは `.unavailable` 扱い
    /// 402 `coins_insufficient` / `allowance_exhausted` / `overflow_cap_reached`
    case coins(CoinDenial)
    #endif
    /// 503 `ext_unavailable`
    case unavailable
    /// 503 `ext_busy`
    case busy

    /// 外の API の断りでなければ nil(呼び出し側のふつうのエラーの扱いに任せる)
    public init?(_ error: Error) {
        guard let api = error as? APIError else { return nil }
        if api.isExtNotConnected {
            self = .notConnected(provider: api.extProvider)
        } else if let pro = ProRequired(api) {
            self = .proRequired(pro)
        } else if let coins = Self.coinDenial(api) {
            self = coins
        } else if api.isExtUnavailable {
            self = .unavailable
        } else if api.isExtBusy {
            self = .busy
        } else {
            return nil
        }
    }

    private static func coinDenial(_ api: APIError) -> ExtDenial? {
        #if APP_STORE_REVIEW
        guard api.status == 402, let code = api.code, coinCodes.contains(code) else { return nil }
        return .unavailable
        #else
        return CoinDenial(api).map(ExtDenial.coins)
        #endif
    }

    #if APP_STORE_REVIEW
    private static let coinCodes: Set<String> = ["coins_insufficient", "allowance_exhausted", "overflow_cap_reached"]
    #endif
}
