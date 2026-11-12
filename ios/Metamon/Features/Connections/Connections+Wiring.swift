import Foundation
import MetamonServices

extension LiveWiring {
    /// 外の API(#1493)。設定の「つないだサービス」は審査用の版で出さないが、op を呼ぶ口は全ての版で同じ。
    static let connections = LiveWiring { services, appConfigURL, token in
        if let appConfigURL { services.external = HTTPExternalServicesClient(baseURL: appConfigURL, token: token) }
    }
}
