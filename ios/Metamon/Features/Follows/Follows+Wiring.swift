import Foundation
import MetamonServices

extension LiveWiring {
    /// フォロー(#270)。appconfig が live の時だけ。
    static let follows = LiveWiring { services, appConfigURL, token in
        services.follows = appConfigURL.map { HTTPFollowsClient(baseURL: $0, token: token) }
    }
}
