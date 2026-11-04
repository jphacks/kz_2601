import Foundation
import MetamonServices

extension LiveWiring {
    /// アカウント削除(#119)。審査用ビルドにも入る(5.1.1(v))。
    static let account = LiveWiring { services, appConfigURL, token in
        if let appConfigURL { services.account = HTTPAccountClient(baseURL: appConfigURL, token: token) }
    }
}
