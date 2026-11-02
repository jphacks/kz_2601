import Foundation
import MetamonServices

extension LiveWiring {
    /// アプリ内のフィードバック(#319)。審査用ビルドは外す。
    static let feedback = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        if let appConfigURL { services.feedback = HTTPFeedbackClient(baseURL: appConfigURL, token: token) }
        #endif
    }
}
