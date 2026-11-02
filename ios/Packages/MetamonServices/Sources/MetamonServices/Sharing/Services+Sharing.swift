#if !APP_STORE_REVIEW // 審査用ビルドはシェア / 探すをバイナリから外す(#245)
import Foundation

/// シェア / 「探す」(#49 #50)。appconfig が live の時だけ(モックでは nil = 導線を出さない)。
public enum SharingServiceKey: ServiceKey {
    public static var defaultValue: (any SharingClient)? { nil }
}

extension Services {
    public var sharing: (any SharingClient)? {
        get { self[SharingServiceKey.self] }
        set { self[SharingServiceKey.self] = newValue }
    }
}
#endif
