import Foundation
import MetamonServices

extension LiveWiring {
    /// プッシュの端末と同意(#269)。審査用ビルドは外す(#245)。
    static let push = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.pushDevices = appConfigURL.map { HTTPPushDeviceClient(baseURL: $0, token: token) }
        #endif
    }
}
