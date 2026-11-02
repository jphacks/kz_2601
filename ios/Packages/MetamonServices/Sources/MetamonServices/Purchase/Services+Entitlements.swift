#if !APP_STORE_REVIEW // 審査用ビルドは課金が無い(#450)
import Foundation

/// Pro の取引をサーバーで検証・保存する(#314)。appconfig が live の時だけ。
public enum EntitlementsServiceKey: ServiceKey {
    public static var defaultValue: (any EntitlementClient)? { nil }
}

extension Services {
    public var entitlements: (any EntitlementClient)? {
        get { self[EntitlementsServiceKey.self] }
        set { self[EntitlementsServiceKey.self] = newValue }
    }
}
#endif
