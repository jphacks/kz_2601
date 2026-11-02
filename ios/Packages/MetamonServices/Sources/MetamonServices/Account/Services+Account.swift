import Foundation

/// アカウント削除(#119)。live では `HTTPAccountClient`、それ以外はモック(成功のみ)。
public enum AccountServiceKey: ServiceKey {
    public static let defaultValue: any AccountClient = MockAccountClient()
}

extension Services {
    public var account: any AccountClient {
        get { self[AccountServiceKey.self] }
        set { self[AccountServiceKey.self] = newValue }
    }
}
