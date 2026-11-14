import Foundation
import MetamonServices

extension LiveWiring {
    /// コイン(#630)。appconfig の公開の設定がコイン課金を有効にした時だけ画面に出る。審査用ビルドは課金が無い(#450)。
    static let coins = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.coins = appConfigURL.map { HTTPCoinClient(baseURL: $0, token: token) }
        #endif
    }
}
