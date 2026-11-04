#if !APP_STORE_REVIEW // 審査用ビルドはリリース申請をバイナリから外す(#245)
import Foundation

/// リリース申請(#76)。sharing と同じく live の時だけ。
public enum ReleaseRequestsServiceKey: ServiceKey {
    public static var defaultValue: (any ReleaseRequestClient)? { nil }
}

extension Services {
    public var releaseRequests: (any ReleaseRequestClient)? {
        get { self[ReleaseRequestsServiceKey.self] }
        set { self[ReleaseRequestsServiceKey.self] = newValue }
    }
}
#endif
