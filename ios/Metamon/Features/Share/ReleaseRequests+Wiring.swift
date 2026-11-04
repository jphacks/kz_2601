import Foundation
import MetamonServices

extension LiveWiring {
    /// リリース申請(#76)。審査用ビルドは外す(#245)。
    static let releaseRequests = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.releaseRequests = appConfigURL.map { HTTPReleaseRequestClient(baseURL: $0, token: token) }
        #endif
    }
}
