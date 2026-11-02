#if !APP_STORE_REVIEW // 審査用ビルドは共有と一緒に仲間探しを外す(#507)
import Foundation

/// 仲間探し(#507)。appconfig が live の時だけ(モックでは nil = 「つながりたい」を出さない)。
public enum MatchingServiceKey: ServiceKey {
    public static var defaultValue: (any MatchingClient)? { nil }
}

extension Services {
    public var matching: (any MatchingClient)? {
        get { self[MatchingServiceKey.self] }
        set { self[MatchingServiceKey.self] = newValue }
    }
}
#endif
