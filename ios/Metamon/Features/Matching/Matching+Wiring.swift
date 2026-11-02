import Foundation
import MetamonServices

extension LiveWiring {
    /// 仲間探し(#507)。審査用ビルドは共有と一緒に外す。
    static let matching = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.matching = appConfigURL.map { HTTPMatchingClient(baseURL: $0, token: token) }
        #endif
    }
}
