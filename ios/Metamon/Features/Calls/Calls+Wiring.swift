import Foundation
import MetamonServices

extension LiveWiring {
    /// 通話(#447)。審査用ビルドは外す(共有と一緒・マイクとカメラで他の人とつながる)。
    static let calls = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.calls = appConfigURL.map { HTTPCallClient(baseURL: $0, token: token) }
        #endif
    }
}
