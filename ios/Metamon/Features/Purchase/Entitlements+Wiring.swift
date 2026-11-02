import Foundation
import MetamonServices

extension LiveWiring {
    /// Pro の取引のサーバー検証(#314)。審査用ビルドは課金が無い(#450)。
    static let entitlements = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.entitlements = appConfigURL.map { HTTPEntitlementClient(baseURL: $0, token: token) }
        #endif
    }
}
