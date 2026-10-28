// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import Observation
import MetamonDSL

/// つながっている相手(距離・方向は UWB 対応端末で測れた時だけ)。
public struct PassbyPeer: Identifiable, Equatable, Sendable {
    public var id: String
    public var name: String
    public var distance: Float?
    /// 水平方向の角度(ラジアン・右が正)
    public var azimuth: Float?

    public init(id: String, name: String, distance: Float? = nil, azimuth: Float? = nil) {
        self.id = id
        self.name = name
        self.distance = distance
        self.azimuth = azimuth
    }
}

public enum PassbyDistanceSupport: Equatable, Sendable {
    case supported
    case unsupported(String)
}

/// すれ違い通信(#425)の通信側。Renderer は MultipeerConnectivity / NearbyInteraction を知らず、アプリ本体が実装を差し込む
/// (`NearbyPassbyHost.controller`・HealthData.source と同じ形)。
@MainActor
public protocol NearbyPassbyController: AnyObject, Observable {
    /// 同意した同意文の版(nil = 未同意)
    var consentedVersion: Int? { get }
    /// いま探している(advertise / browse 中)
    var isAdvertising: Bool { get }
    /// 会った人(今のアプリ・新しい順)
    var encounters: [NearbyPassby.Encounter] { get }
    var peers: [PassbyPeer] { get }
    var distanceSupport: PassbyDistanceSupport { get }
    /// 直近の失敗・案内(ローカルネットワークの許可が無い等)
    var notice: String? { get }

    func isOptedIn(_ config: NearbyPassby.Config) -> Bool
    func setOptIn(_ on: Bool, config: NearbyPassby.Config)
    func agree()
    /// 画面に出て前面にいる間: オンなら探し始める
    func attach(_ config: NearbyPassby.Config)
    /// 画面を離れた・裏に回った: 止める
    func detach(_ config: NearbyPassby.Config)
}

public enum NearbyPassbyHost {
    @MainActor public static var controller: any NearbyPassbyController = StaticNearbyPassbyController()
}

/// 何も通信しない実装(プレビュー・テスト・未設定時)。値は外から入れる。
@MainActor
@Observable
public final class StaticNearbyPassbyController: NearbyPassbyController {
    public var consentedVersion: Int?
    public var isAdvertising = false
    public var encounters: [NearbyPassby.Encounter] = []
    public var peers: [PassbyPeer] = []
    public var distanceSupport: PassbyDistanceSupport = .unsupported("この端末は距離・方向の測定に対応していません")
    public var notice: String?
    public private(set) var optedIn: Set<String> = []
    public private(set) var attached: String?

    public init() {}

    public func isOptedIn(_ config: NearbyPassby.Config) -> Bool { optedIn.contains(config.componentId) }
    public func setOptIn(_ on: Bool, config: NearbyPassby.Config) {
        if on { optedIn.insert(config.componentId) } else { optedIn.remove(config.componentId) }
        isAdvertising = on && attached == config.componentId
    }
    public func agree() { consentedVersion = NearbyPassby.consentVersion }
    public func attach(_ config: NearbyPassby.Config) {
        attached = config.componentId
        isAdvertising = isOptedIn(config)
    }
    public func detach(_ config: NearbyPassby.Config) {
        guard attached == config.componentId else { return }
        attached = nil
        isAdvertising = false
    }
}
#endif
