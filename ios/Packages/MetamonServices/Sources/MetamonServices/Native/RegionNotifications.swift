import Foundation
#if os(iOS)
import CoreLocation
#endif

/// 場所の通知(`near`)のうち「最初の 1 回だけ」(repeat once)を、届いた後の予約し直しで再び見張らないための記録。
/// 予約した(armed)のに iOS の予約に無い = 届いた(fired)とみなす。場所(緯度・経度・半径)が変われば別の通知として見張り直す。
/// UserDefaults に保存(アプリを閉じても 1 回きりを守る)。位置は端末の中だけに置く。
actor RegionNotificationStore {
    static let shared = RegionNotificationStore()
    private static let key = "metamon.notifications.region.v1"
    private struct State: Codable {
        var armed: Set<String> = []
        var fired: Set<String> = []
    }
    private let defaults: UserDefaults
    private var state: State

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        state = defaults.data(forKey: Self.key).flatMap { try? JSONDecoder().decode(State.self, from: $0) } ?? State()
    }

    /// "<識別子>@<緯度>,<経度>,<半径>"。
    static func key(_ p: PlannedNotification) -> String? {
        guard case .region(let lat, let lng, let r, _) = p.trigger else { return nil }
        return p.identifier + "@" + String(format: "%.5f,%.5f,%.0f", lat, lng, r)
    }

    /// 予約する場所の通知を選ぶ。1 回きりで届いた分を落とし、記録を今の場所の通知だけに絞る(`known` = 全アプリの場所の通知)。
    func toSchedule(_ regions: [PlannedNotification], known: [PlannedNotification], pending: Set<String>) -> [PlannedNotification] {
        let knownKeys = Set(known.compactMap(Self.key))
        state.fired.formIntersection(knownKeys)
        let out = regions.filter { p in
            guard case .region(_, _, _, let repeats) = p.trigger, !repeats, let k = Self.key(p) else { return true }
            if state.armed.contains(k) && !pending.contains(p.identifier) { state.fired.insert(k) }
            return !state.fired.contains(k)
        }
        save()
        return out
    }

    /// 今回 iOS に渡せた場所の通知。
    func setArmed(_ added: [PlannedNotification]) {
        state.armed = Set(added.compactMap(Self.key))
        save()
    }

    func removeApp(_ appId: String) {
        let prefix = NotificationPlanner.prefix + appId + "."
        state.armed = state.armed.filter { !$0.hasPrefix(prefix) }
        state.fired = state.fired.filter { !$0.hasPrefix(prefix) }
        save()
    }

    func removeAll() {
        state = State()
        save()
    }

    private func save() {
        if let data = try? JSONEncoder().encode(state) { defaults.set(data, forKey: Self.key) }
    }
}

#if os(iOS)
/// 場所の通知に要る位置情報の許可(使用中のみ)。場所を保存したアプリの予約の時だけ、未決定なら 1 回だけ聞く(拒否されたら聞き直さない)。
/// 現在地は追跡しない(`location` は OS が持つ最後の位置。近い順に選ぶのに使うだけで、端末の外に出さない)。
@MainActor
final class RegionLocationAccess: NSObject, CLLocationManagerDelegate {
    static let shared = RegionLocationAccess()
    private let manager = CLLocationManager()
    private var waiters: [CheckedContinuation<Bool, Never>] = []

    override init() {
        super.init()
        manager.delegate = self
    }

    var isAuthorized: Bool {
        let s = manager.authorizationStatus
        return Self.isGranted(s)
    }

    /// iOS: 使用中のみ / 常に。Mac は「使用中のみ」が無い。
    nonisolated static func isGranted(_ s: CLAuthorizationStatus) -> Bool {
        #if os(iOS)
        s == .authorizedWhenInUse || s == .authorizedAlways
        #else
        s == .authorizedAlways
        #endif
    }

    var isUndetermined: Bool { manager.authorizationStatus == .notDetermined }

    var lastKnown: (lat: Double, lng: Double)? {
        guard isAuthorized, let c = manager.location?.coordinate else { return nil }
        return (c.latitude, c.longitude)
    }

    /// 未決定なら許可を聞いて結果を待つ。決まっていればその結果。
    func authorize() async -> Bool {
        guard isUndetermined else { return isAuthorized }
        return await withCheckedContinuation { cont in
            waiters.append(cont)
            if waiters.count == 1 { manager.requestWhenInUseAuthorization() }
        }
    }

    nonisolated func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = manager.authorizationStatus
        Task { @MainActor in
            guard status != .notDetermined, !waiters.isEmpty else { return }
            let ok = Self.isGranted(status)
            let w = waiters
            waiters = []
            w.forEach { $0.resume(returning: ok) }
        }
    }
}
#endif
