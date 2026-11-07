#if !APP_STORE_REVIEW // 審査用ビルドはプッシュの通信ごと外す(#269)
import Foundation

/// APNs の環境。トークンはこの環境でだけ有効(`PUT /v1/me/devices` の `environment`)。
public enum PushEnvironment: String, Sendable {
    case sandbox
    case production

    /// 署名の `aps-environment` を読む。開発署名だけ development で、配布の版に profile は無いか production。
    public static func from(provisioningProfile: Data?) -> PushEnvironment {
        guard let profile = provisioningProfile, let text = String(data: profile, encoding: .isoLatin1) else { return .production }
        let development = #"<key>aps-environment</key>\s*<string>development</string>"#
        return text.range(of: development, options: .regularExpression) == nil ? .production : .sandbox
    }

    public static var current: PushEnvironment {
        #if targetEnvironment(simulator)
        .sandbox
        #else
        from(provisioningProfile: Bundle.main.url(forResource: "embedded", withExtension: "mobileprovision").flatMap { try? Data(contentsOf: $0) })
        #endif
    }
}

/// `PUT /v1/me/devices/{installationId}` の本文。
public struct PushDeviceRegistration: Encodable, Equatable, Sendable {
    public var token: String
    public var environment: String
    public var appVersion: String
    public var os: String

    public init(token: String, environment: PushEnvironment, appVersion: String, os: String) {
        self.token = token
        self.environment = environment.rawValue
        self.appVersion = appVersion
        self.os = os
    }
}

/// 端末の登録と、他の人の行動で届く通知の同意(#269・#339)。
public protocol PushDeviceClient: Sendable {
    func putDevice(installationId: String, registration: PushDeviceRegistration) async throws
    func deleteDevice(installationId: String) async throws
    func setNotifyConsent(appId: String, allowed: Bool) async throws
}

/// appconfig が live の時だけ(モックでは nil = 登録しない・同意の切り替えを出さない)。
public enum PushDeviceServiceKey: ServiceKey {
    public static var defaultValue: (any PushDeviceClient)? { nil }
}

extension Services {
    public var pushDevices: (any PushDeviceClient)? {
        get { self[PushDeviceServiceKey.self] }
        set { self[PushDeviceServiceKey.self] = newValue }
    }
}

/// runner の APNs の本文(`backend/internal/runner/notify/apns`)の読み方。
public enum PushPayload {
    /// タップで開くアプリ。形が backend の appId でなければ nil。ローカル通知には無いので nil。
    public static func appId(from userInfo: [AnyHashable: Any]) -> String? {
        guard let id = userInfo["appId"] as? String,
              id.range(of: #"^[A-Za-z0-9_.:-]{1,128}$"#, options: .regularExpression) != nil else { return nil }
        return id
    }

    /// `deviceToken` を backend の形(小文字の 16 進)に。
    public static func tokenHex(_ token: Data) -> String {
        token.map { String(format: "%02x", $0) }.joined()
    }
}

/// 同意の控え(uid + appId ごと)。読む API が無いので、この端末で変えた値を覚える。ログアウトの `LocalWipe` で消える。
public struct NotifyConsentStore: @unchecked Sendable {
    private let defaults: UserDefaults

    public init(defaults: UserDefaults = .standard) { self.defaults = defaults }

    public func allowed(uid: String, appId: String) -> Bool { defaults.bool(forKey: key(uid, appId)) }

    public func set(_ allowed: Bool, uid: String, appId: String) { defaults.set(allowed, forKey: key(uid, appId)) }

    private func key(_ uid: String, _ appId: String) -> String { "metamon.push.notifyConsent.\(uid).\(appId)" }
}
#endif
