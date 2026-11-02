#if !APP_STORE_REVIEW // 審査用ビルドには無い(#446)
import Foundation

/// aiButton の端末内 AI の集計の送り先(#446)。appconfig が live の時だけ。
public enum AIButtonMetricsServiceKey: ServiceKey {
    public static var defaultValue: (any AIButtonMetricsSender)? { nil }
}

extension Services {
    public var aiButtonMetrics: (any AIButtonMetricsSender)? {
        get { self[AIButtonMetricsServiceKey.self] }
        set { self[AIButtonMetricsServiceKey.self] = newValue }
    }
}
#endif
