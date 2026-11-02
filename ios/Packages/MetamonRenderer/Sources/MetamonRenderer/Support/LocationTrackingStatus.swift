// 審査用ビルド(APP_STORE_REVIEW・#245)では LocationTracking ごと外す。
#if !APP_STORE_REVIEW
import Foundation

/// 位置の記録(`LocationTracking`)の今の状態。ホストのアプリ全体の「記録中」バー(#559)が読む。
public struct LocationTrackingStatus: Equatable, Sendable {
    /// 記録中(許可を待っている間も含む)
    public var isTracking: Bool
    /// 位置情報の許可を待っている(まだ点を取り始めていない)
    public var isRequesting: Bool
    /// 記録を始めた時刻。許可待ちの間は nil
    public var startedAt: Date?
    /// 記録しているアプリ名と entity
    public var appName: String?
    public var entity: String?

    public init(isTracking: Bool, isRequesting: Bool = false, startedAt: Date? = nil, appName: String? = nil, entity: String? = nil) {
        self.isTracking = isTracking
        self.isRequesting = isRequesting
        self.startedAt = startedAt
        self.appName = appName
        self.entity = entity
    }

    public static let idle = LocationTrackingStatus(isTracking: false)
}

extension LocationTracking {
    var status: LocationTrackingStatus { Self.status(phase: phase, target: target, startedAt: startedAt) }

    /// startedAt は止めても残るので、記録中の時だけ渡す。
    static func status(phase: Phase, target: Target?, startedAt: Date?) -> LocationTrackingStatus {
        switch phase {
        case .recording:
            .init(isTracking: true, startedAt: startedAt, appName: target?.appName, entity: target?.entity)
        case .requesting:
            .init(isTracking: true, isRequesting: true, appName: target?.appName, entity: target?.entity)
        case .idle, .denied, .failed:
            .idle
        }
    }
}

/// ホストから記録の状態を読む・止める入口。`status` は Observation で追跡される(View の body で読めば変化で描き直る)。
@MainActor
public enum LocationTrackingControl {
    public static var status: LocationTrackingStatus { LocationTracking.shared.status }
    public static func stop() { LocationTracking.shared.stop() }
}
#endif
