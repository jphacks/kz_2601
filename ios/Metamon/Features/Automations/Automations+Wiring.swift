import Foundation
import MetamonServices

extension LiveWiring {
    /// 自動の動きの見える化(#347)。審査用ビルドは外す(serverRules を作らない・#614)。
    static let automations = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.automations = appConfigURL.map { HTTPAutomationsClient(baseURL: $0, token: token) }
        #endif
    }
}
