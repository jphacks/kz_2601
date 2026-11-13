import Foundation
import Observation
import MetamonDSL
import MetamonRenderer

/// ライブアクティビティ(#423)の代わりの試作(#985・#2254): `startActivity` / `endActivity` と `timerDisplay.liveActivity` を、
/// フラグが on の時だけメニューバーのアイコンに経過時間(countdown は残り)で出す。窓には「止める」「本体を開く」。同時に 1 つ(後から来た方)。
/// フラグは既定 off(出すかどうかはオーナーが #985 で決める)。off の時は今と同じ(`startActivity` は一言だけ・ほかは何もしない)。
/// 申告の `liveActivity` は変えない。
@MainActor
@Observable
final class MacMenuBarActivity {
    static let shared = MacMenuBarActivity()

    /// 試作のフラグ(UserDefaults)。`defaults write <bundle id> metamon.mac.menuBar.activity -bool YES` で on
    nonisolated static let flagKey = "metamon.mac.menuBar.activity"
    /// 試作のフラグ(環境変数)。`METAMON_MAC_MENUBAR_ACTIVITY=1` でも on
    nonisolated static let flagEnv = "METAMON_MAC_MENUBAR_ACTIVITY"

    /// フラグ(既定 off)
    nonisolated static func isFlagOn(defaults: UserDefaults = .standard,
                                     environment: [String: String] = ProcessInfo.processInfo.environment) -> Bool {
        environment[flagEnv] == "1" || defaults.bool(forKey: flagKey)
    }

    /// メニューバーに出している表示
    struct Item: Equatable {
        /// startActivity は `app:<appId>`、タイマーは部品のキー
        var key: String
        var appId: String
        var appName: String
        var title: String
        var icon: String
        /// 数え始め(一時停止分を差し引いた見かけの開始)。一時停止中は nil
        var start: Date?
        /// countdown の終わり。経過時間なら nil
        var end: Date?
        /// 一時停止中に見せる秒
        var pausedSeconds: Int?
    }

    private(set) var current: Item?
    /// 表示が変わった(`MacStatusItem` がアイコンの文字を出し直す)
    @ObservationIgnored var onChange: (() -> Void)?
    @ObservationIgnored private let isEnabled: () -> Bool

    init(isEnabled: @escaping () -> Bool = { MacMenuBarActivity.isFlagOn() }) {
        self.isEnabled = isEnabled
    }

    static func appKey(_ appId: String) -> String { "app:\(appId)" }

    /// `startActivity`。フラグが off なら何もせず false(呼ぶ側は今まで通り一言を出す)
    @discardableResult
    func start(appId: String, appName: String, spec: ActivitySpec, now: Date = .now) -> Bool {
        guard isEnabled() else { return false }
        let icon = spec.icon.flatMap { $0.isEmpty ? nil : $0 } ?? "timer"
        set(Item(key: Self.appKey(appId), appId: appId, appName: appName,
                 title: spec.title.isEmpty ? appName : spec.title, icon: icon,
                 start: now, end: spec.minutes.map { now.addingTimeInterval(TimeInterval(max($0, 0) * 60)) }))
        return true
    }

    /// `endActivity`: このアプリの startActivity の表示だけ消す(タイマーのものは残す・iOS と同じ)
    func end(appId: String) {
        guard current?.key == Self.appKey(appId) else { return }
        set(nil)
    }

    /// `timerDisplay.liveActivity` の開始・一時停止・終了。フラグが off なら何もしない
    func timer(appId: String, appName: String, update: TimerActivityUpdate) {
        guard isEnabled() else { return }
        switch update.phase {
        case .running(let start, let end):
            set(Item(key: update.key, appId: appId, appName: appName, title: update.label, icon: "timer", start: start, end: end))
        case .paused(let shown):
            set(Item(key: update.key, appId: appId, appName: appName, title: update.label, icon: "timer", pausedSeconds: shown))
        case .finished, .ended:
            if current?.key == update.key { set(nil) }
        }
    }

    /// 窓の「止める」: メニューバーの表示を消す
    func stop() { set(nil) }

    /// アプリを消した時: そのアプリの表示を消す
    func remove(appId: String) {
        guard current?.appId == appId else { return }
        set(nil)
    }

    private func set(_ item: Item?) {
        guard item != current else { return }
        current = item
        onChange?()
    }

    // MARK: - 表示

    /// 見せる秒: 経過時間は経過、countdown は残り(0 未満にしない)、一時停止中はその秒
    static func seconds(_ item: Item, now: Date) -> Int {
        if let paused = item.pausedSeconds { return max(paused, 0) }
        if let end = item.end { return max(Int(end.timeIntervalSince(now).rounded(.up)), 0) }
        guard let start = item.start else { return 0 }
        return max(Int(now.timeIntervalSince(start)), 0)
    }

    /// 「12:34」「1:02:03」
    static func clockText(_ seconds: Int) -> String {
        let s = max(seconds, 0)
        let h = s / 3600, m = (s % 3600) / 60, sec = s % 60
        return h > 0 ? String(format: "%d:%02d:%02d", h, m, sec) : String(format: "%d:%02d", m, sec)
    }

    /// メニューバーのアイコンの横の文字
    static func statusTitle(_ item: Item, now: Date) -> String {
        clockText(seconds(item, now: now))
    }
}

/// 中央のアプリの `timerDisplay.liveActivity`(#2254)。フラグが off なら何もしない(今と同じ)
extension MacAppRuntime: LiveActivityHost {
    func timerActivity(_ update: TimerActivityUpdate) {
        MacMenuBarActivity.shared.timer(appId: appId, appName: dsl.app.name, update: update)
    }
}
