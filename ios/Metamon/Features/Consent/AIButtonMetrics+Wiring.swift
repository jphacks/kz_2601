import Foundation
import MetamonServices

extension LiveWiring {
    /// aiButton の端末内 AI の集計の送り先(#446)。審査用ビルドには無い。
    static let aiButtonMetrics = LiveWiring { services, appConfigURL, token in
        #if !APP_STORE_REVIEW
        services.aiButtonMetrics = appConfigURL.map { HTTPAIButtonMetricsSender(baseURL: $0, token: token) }
        #endif
    }
}
