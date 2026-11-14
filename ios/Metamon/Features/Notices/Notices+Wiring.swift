import Foundation
import MetamonServices

extension LiveWiring {
    /// 「あなたの声で直りました」のお知らせと push の同意(#374)。審査用ビルドはフィードバックと一緒に外す。
    static let notices = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.loopNotices = appConfigURL.map { HTTPLoopNoticesClient(baseURL: $0, token: token) }
        #endif
    }
}
