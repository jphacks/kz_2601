import Foundation
import CoreLocation
import Observation
import MetamonDSL

/// 位置の記録(`locationTracker`・#168)のセッション。アプリ全体で 1 つ(画面を移っても・背面でも続く)。
///
/// - **使う人が開始を押した間だけ**動く。既定はオフ。最長 `Track.autoStopSeconds`(既定 1 時間・最大 4 時間)で自動停止。
/// - 権限は **When In Use だけ**(Always は要求しない)。背面での継続は `CLBackgroundActivitySession`(iOS 17+)
///   + Info.plist の `UIBackgroundModes: location`。セッション中は OS がステータスバーに青い位置インジケータを出す。
/// - 位置は `CLLocationUpdate.liveUpdates(.fitness)`。保存は `Track.shouldKeep` で間引く(精度 50 m 超は捨て、25 m 動くか 5 分経った時だけ)。
/// - モック(`METAMON_MOCK=1`)は権限を聞かず `Track.mockRoute` を 2 秒ごとに流す(シミュレータ・UI 確認用)。
@MainActor
@Observable
final class LocationTracking: NSObject {
    static let shared = LocationTracking()

    enum Phase: Equatable {
        case idle, requesting, recording, denied
        case failed(String)
    }

    /// 記録先。同じアプリ内の同じ entity + location field の部品なら「この部品の記録」とみなす。
    struct Target: Equatable {
        var appName: String
        var entity: String
        var locationField: String
        var dateField: String?
    }

    private(set) var phase: Phase = .idle
    private(set) var target: Target?
    private(set) var startedAt: Date?
    private(set) var endsAt: Date?
    private(set) var savedCount = 0
    private(set) var distance: Double = 0
    /// 直前の記録が止まった理由(自動停止など)。次の開始で消える。
    private(set) var lastStopMessage: String?

    private var lastKept: Track.Point?
    private var updatesTask: Task<Void, Never>?
    private var stopTask: Task<Void, Never>?
    private var backgroundSession: CLBackgroundActivitySession?
    private var save: (@MainActor ([String: JSONValue]) async -> Bool)?
    private let manager = CLLocationManager()
    private var authContinuation: CheckedContinuation<CLAuthorizationStatus, Never>?

    static var isMock: Bool { ProcessInfo.processInfo.environment["METAMON_MOCK"] == "1" }

    var isActive: Bool { phase == .recording || phase == .requesting }

    func isRecording(_ t: Target) -> Bool { isActive && target == t }

    /// 記録を始める。`save` は 1 点ぶんの data(location + 日時)を受け取ってレコードを 1 件足す。
    func start(_ t: Target, autoStopSeconds: Int, save: @escaping @MainActor ([String: JSONValue]) async -> Bool) {
        guard !isActive else { return }
        target = t
        self.save = save
        savedCount = 0
        distance = 0
        lastKept = nil
        lastStopMessage = nil
        phase = .requesting
        updatesTask = Task { [weak self] in
            guard let self else { return }
            if Self.isMock {
                beginRecording(autoStopSeconds)
                await runMock()
                return
            }
            let status = await authorize()
            guard status == .authorizedWhenInUse || status == .authorizedAlways else {
                finish(.denied, message: nil)
                return
            }
            beginRecording(autoStopSeconds)
            await runLive()
        }
    }

    func stop(message: String? = nil) {
        finish(.idle, message: message)
    }

    // MARK: - private

    private func beginRecording(_ seconds: Int) {
        let now = Date()
        startedAt = now
        endsAt = now.addingTimeInterval(TimeInterval(seconds))
        phase = .recording
        // 前面にいる今のうちに作る(背面からは作れない)。生きている間だけ背面でも位置が届き、青いインジケータが出る
        if !Self.isMock { backgroundSession = CLBackgroundActivitySession() }
        stopTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(seconds))
            guard !Task.isCancelled else { return }
            self?.stop(message: L("\(seconds / 60)分たったので記録を止めました"))
        }
    }

    private func finish(_ phase: Phase, message: String?) {
        updatesTask?.cancel()
        updatesTask = nil
        stopTask?.cancel()
        stopTask = nil
        backgroundSession?.invalidate()
        backgroundSession = nil
        save = nil
        if phase != .idle || message != nil { lastStopMessage = message }
        self.phase = phase
        if phase == .idle { endsAt = nil }
    }

    private func runLive() async {
        do {
            for try await update in CLLocationUpdate.liveUpdates(.fitness) {
                if Task.isCancelled { return }
                if #available(iOS 18, *), update.authorizationDenied || update.authorizationDeniedGlobally {
                    finish(.denied, message: nil)
                    return
                }
                guard let loc = update.location else { continue }
                let p = Track.Point(lat: loc.coordinate.latitude, lng: loc.coordinate.longitude, date: loc.timestamp)
                await keep(p, accuracy: loc.horizontalAccuracy)
                if phase != .recording { return }
            }
        } catch {
            if !Task.isCancelled { finish(.failed(error.localizedDescription), message: nil) }
        }
    }

    private func runMock() async {
        for c in Track.mockRoute {
            if Task.isCancelled || phase != .recording { return }
            await keep(Track.Point(lat: c.lat, lng: c.lng, date: Date()), accuracy: 5)
            try? await Task.sleep(for: .seconds(2))
        }
    }

    private func keep(_ p: Track.Point, accuracy: Double) async {
        guard phase == .recording, let target, let save,
              Track.shouldKeep(p, accuracy: accuracy, last: lastKept, minDistance: Self.isMock ? 0 : 25) else { return }
        var data: [String: JSONValue] = [target.locationField: .string(Track.encode(lat: p.lat, lng: p.lng))]
        if let df = target.dateField { data[df] = .string(FieldCoding.datetimeFormatter.string(from: p.date)) }
        guard await save(data) else {
            finish(.failed(L("記録を保存できませんでした")), message: nil)
            return
        }
        if let last = lastKept { distance += Track.distance(last, p) }
        lastKept = p
        savedCount += 1
    }

    /// When In Use を聞く(Always は聞かない)。決まっていればすぐ返す。
    private func authorize() async -> CLAuthorizationStatus {
        manager.delegate = self
        let status = manager.authorizationStatus
        guard status == .notDetermined else { return status }
        return await withCheckedContinuation { cont in
            authContinuation = cont
            manager.requestWhenInUseAuthorization()
        }
    }
}

extension LocationTracking: CLLocationManagerDelegate {
    nonisolated func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = manager.authorizationStatus
        Task { @MainActor in
            guard status != .notDetermined, let cont = authContinuation else { return }
            authContinuation = nil
            cont.resume(returning: status)
        }
    }
}
