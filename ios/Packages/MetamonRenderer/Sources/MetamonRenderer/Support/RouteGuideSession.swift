import Foundation
import UIKit
import MapKit
import CoreLocation
import AVFoundation
import Observation
import MetamonDSL

/// 道案内(`routeGuide`・#411)の 1 回分。経路(MKDirections)・現在地(CLLocationUpdate)・読み上げ(AVSpeechSynthesizer)。
/// 進み方・読み上げる時は `RouteGuidance`(MetamonDSL・純粋ロジック)が決める。
///
/// - 位置は **使用中だけ**(When In Use・`CLBackgroundActivitySession` を使わない)。アプリを閉じると案内も止まる(審査用ビルドでも残せる)。
/// - 案内中は画面を消さない(`isIdleTimerDisabled`)。止める・画面を離れる・到着で戻す。
/// - 道を外れた位置が 3 回続いたら現在地から引き直す(最短 15 秒おき)。
/// - モック(`METAMON_MOCK=1`)は権限・通信を使わず、金沢駅から目的地への L 字の経路を 0.5 秒ごとに約 30 点で進む。
@MainActor
@Observable
final class RouteGuideSession {
    enum Phase: Equatable {
        case idle, locating, routing, guiding, arrived, denied
        case failed(String)
    }

    private(set) var phase: Phase = .idle
    private(set) var route: RouteGuidance.Route?
    private(set) var progress: RouteGuidance.Progress?
    private(set) var current: RouteGuidance.Coord?
    /// 直前に読み上げた(音声オフなら読み上げるはずだった)文
    private(set) var lastCue: String?
    private(set) var mode: RouteGuidance.Mode = .automobile
    var voiceOn = true {
        didSet { if !voiceOn { synthesizer.stopSpeaking(at: .immediate) } }
    }

    private var destination: RouteGuidance.Coord?
    private var stepIndex = 0
    private var spoken = Set<String>()
    private var offRouteFixes = 0
    private var lastReroute: Date?
    private var task: Task<Void, Never>?
    private var keptScreenOn = false
    private let synthesizer = AVSpeechSynthesizer()

    static var isMock: Bool { PlacePicking.isMock }

    var isActive: Bool { [.locating, .routing, .guiding].contains(phase) }

    // MARK: - start / stop

    func start(to destination: RouteGuidance.Coord, mode: RouteGuidance.Mode) {
        stop()
        self.destination = destination
        self.mode = mode
        phase = .locating
        task = Task { [weak self] in
            guard let self else { return }
            guard let here = await locate() else { return }
            guard await calculate(from: here) else { return }
            phase = .guiding
            keepScreenOn(true)
            handle(here, accuracy: 0)
            if Self.isMock { await runMock() } else { await runLive() }
        }
    }

    func stop() {
        task?.cancel()
        task = nil
        synthesizer.stopSpeaking(at: .immediate)
        keepScreenOn(false)
        if isActive { phase = .idle }
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
    }

    /// 車 / 徒歩の切り替え。案内中なら今の位置から引き直す。
    func change(mode: RouteGuidance.Mode) {
        guard mode != self.mode else { return }
        self.mode = mode
        guard phase == .guiding, let current else { return }
        Task { _ = await calculate(from: current) }
    }

    /// 終わった案内(到着・失敗)の表示を消して最初の状態に戻す。
    func reset() {
        stop()
        phase = .idle
        route = nil
        progress = nil
        current = nil
        lastCue = nil
    }

    // MARK: - steps

    private func locate() async -> RouteGuidance.Coord? {
        if Self.isMock {
            let c = PlacePicking.mockHere.coordinate
            return .init(lat: c.latitude, lng: c.longitude)
        }
        do {
            let loc = try await OneShotLocation().request()
            return .init(lat: loc.coordinate.latitude, lng: loc.coordinate.longitude)
        } catch OneShotLocation.LocationError.denied {
            phase = .denied
        } catch {
            phase = .failed(L("位置情報を取得できませんでした(\(error.localizedDescription))"))
        }
        return nil
    }

    /// 経路を(引き直しも)求める。進み具合・読み上げの印はここで最初からにする。
    private func calculate(from here: RouteGuidance.Coord) async -> Bool {
        guard let destination else { return false }
        if phase != .guiding { phase = .routing }
        let found: RouteGuidance.Route?
        if Self.isMock {
            found = RouteGuidance.mockRoute(from: here, to: destination, mode: mode)
        } else {
            found = await Self.directions(from: here, to: destination, mode: mode)
        }
        guard !Task.isCancelled else { return false }
        guard let found else {
            if phase != .guiding { phase = .failed(L("経路が見つかりませんでした")) }
            return false
        }
        route = found
        stepIndex = 0
        spoken = []
        offRouteFixes = 0
        return true
    }

    static func directions(from: RouteGuidance.Coord, to: RouteGuidance.Coord, mode: RouteGuidance.Mode) async -> RouteGuidance.Route? {
        let request = MKDirections.Request()
        request.source = MKMapItem(placemark: MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: from.lat, longitude: from.lng)))
        request.destination = MKMapItem(placemark: MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: to.lat, longitude: to.lng)))
        request.transportType = mode == .walking ? .walking : .automobile
        request.requestsAlternateRoutes = false
        guard let r = try? await MKDirections(request: request).calculate().routes.first else { return nil }
        let steps = r.steps.map { s in
            let pts = s.polyline.points()
            let coords = (0..<s.polyline.pointCount).map { i -> RouteGuidance.Coord in
                let c = pts[i].coordinate
                return .init(lat: c.latitude, lng: c.longitude)
            }
            return RouteGuidance.Step(instruction: s.instructions, points: coords)
        }
        return RouteGuidance.Route(steps: steps, expectedTravelTime: r.expectedTravelTime)
    }

    private func runLive() async {
        let config: CLLocationUpdate.LiveConfiguration = mode == .automobile ? .automotiveNavigation : .otherNavigation
        do {
            for try await update in CLLocationUpdate.liveUpdates(config) {
                if Task.isCancelled || phase != .guiding { return }
                guard let loc = update.location, loc.horizontalAccuracy >= 0 else { continue }
                handle(.init(lat: loc.coordinate.latitude, lng: loc.coordinate.longitude), accuracy: loc.horizontalAccuracy)
                if phase == .arrived { return }
                if progress?.isOffRoute == true,
                   RouteGuidance.shouldReroute(offRouteFixes: offRouteFixes, lastReroute: lastReroute, now: .now) {
                    lastReroute = .now
                    say(.rerouting)
                    _ = await calculate(from: .init(lat: loc.coordinate.latitude, lng: loc.coordinate.longitude))
                }
            }
        } catch {
            if !Task.isCancelled { phase = .failed(L("位置情報を取得できませんでした(\(error.localizedDescription))")) }
        }
    }

    private func runMock() async {
        guard let route else { return }
        for p in RouteGuidance.mockDrive(route, every: max(route.distance / 30, 10)).dropFirst() {
            try? await Task.sleep(for: .milliseconds(500))
            if Task.isCancelled || phase != .guiding { return }
            handle(p, accuracy: 5)
            if phase == .arrived { return }
        }
    }

    /// 位置 1 つぶん進める。
    private func handle(_ p: RouteGuidance.Coord, accuracy: Double) {
        guard let route else { return }
        let pr = RouteGuidance.progress(route, at: p, accuracy: accuracy, from: stepIndex, mode: mode)
        current = p
        progress = pr
        stepIndex = pr.stepIndex
        offRouteFixes = pr.isOffRoute ? offRouteFixes + 1 : 0
        if !pr.isOffRoute, let cue = RouteGuidance.cue(for: pr, spoken: &spoken, mode: mode) { say(cue) }
        if pr.arrived {
            phase = .arrived
            keepScreenOn(false)
        }
    }

    // MARK: - 読み上げ

    private func say(_ cue: RouteGuidance.Cue) {
        let text = Self.text(cue)
        lastCue = text
        guard voiceOn else { return }
        let session = AVAudioSession.sharedInstance()
        if session.category != .playback || session.mode != .voicePrompt {
            try? session.setCategory(.playback, mode: .voicePrompt, options: [.duckOthers, .interruptSpokenAudioAndMixWithOthers])
        }
        try? session.setActive(true)
        if synthesizer.isSpeaking { synthesizer.stopSpeaking(at: .word) }
        let u = AVSpeechUtterance(string: text)
        u.voice = AVSpeechSynthesisVoice(language: LanguagePreference.shared.code == "en" ? "en-US" : "ja-JP")
        synthesizer.speak(u)
    }

    /// 読み上げる文。距離は丸めて単位を言葉で(「300 メートル」)。
    static func text(_ cue: RouteGuidance.Cue) -> String {
        switch cue {
        case .next(let d, let instruction), .approach(let d, let instruction):
            let dist = spokenDistance(d)
            guard let instruction else { return L("\(dist)先、目的地です") }
            return L("\(dist)先、\(instruction)")
        case .now(let instruction):
            guard let instruction else { return L("まもなく目的地です") }
            return L("まもなく、\(instruction)")
        case .arrived:
            return L("目的地に到着しました。案内を終了します")
        case .rerouting:
            return L("ルートを外れました。経路を探し直します")
        }
    }

    static func spokenDistance(_ meters: Double) -> String {
        let f = MeasurementFormatter()
        f.locale = LanguagePreference.shared.locale
        f.unitStyle = .long
        f.unitOptions = .providedUnit
        f.numberFormatter.maximumFractionDigits = 1
        let r = RouteGuidance.rounded(meters)
        let m: Measurement<UnitLength> = r < 1000 ? Measurement(value: r, unit: .meters) : Measurement(value: r / 1000, unit: .kilometers)
        return f.string(from: m)
    }

    private func keepScreenOn(_ on: Bool) {
        guard on != keptScreenOn else { return }
        keptScreenOn = on
        UIApplication.shared.isIdleTimerDisabled = on
    }
}
