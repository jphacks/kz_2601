import Foundation
import MetamonServices

extension LiveWiring {
    /// シェア / 探す(#49 #50)。審査用ビルドは外す(#245)。
    static let sharing = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.sharing = appConfigURL.map { HTTPSharingClient(baseURL: $0, token: token) }
        #endif
    }
}
