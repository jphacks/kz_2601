import Foundation
import MetamonServices

extension LiveWiring {
    /// 外部の公開データ(#348)。審査用ビルドは外す(外部の Web の内容を出すため)。
    static let feeds = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.feeds = appConfigURL.map { HTTPFeedClient(baseURL: $0, token: token) }
        #endif
    }
}
