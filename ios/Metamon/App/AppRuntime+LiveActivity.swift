import ActivityKit
import Foundation
import MetamonDSL
import MetamonRenderer

/// ライブアクティビティ(#423): ロック画面と Dynamic Island に「いま◯◯中」/ タイマーを出す。
/// - 更新は端末内だけ(push 無し)。時間は日付で渡すので OS が描き、アプリが止まっていても進む
/// - 数字(activity.stat)は records が変わるたびに取り直す(`records` の didSet)
/// 中身の組み立ては `LiveActivityContent`(純粋関数)。ここは ActivityKit への出し入れだけ。
extension AppRuntime {
    /// `timerDisplay.liveActivity` の開始・一時停止・リセット・終了。
    func timerActivity(_ update: TimerActivityUpdate) {
        let attributes = TailorActivityAttributes(appId: document.appId, appName: dsl.app.name, icon: LiveActivityContent.defaultIcon,
                                                  key: LiveActivityContent.timerKey(update.key), statId: nil)
        let plan = LiveActivityContent.plan(for: update)
        LiveActivities.enqueue { await LiveActivities.apply(plan, attributes: attributes) }
    }

    /// `startActivity`: このアプリの表示を出し直す(前のものは置き換える)。
    func startActivity(_ spec: ActivitySpec) async {
        guard LiveActivities.enabled else {
            toast = L10n.tr("ロック画面への表示が設定でオフになっています(設定 > Tailor It > ライブアクティビティ)")
            return
        }
        let reading = spec.stat.flatMap { LiveActivityContent.statReading(id: $0, dsl: dsl, records: records) }
        let (state, stale) = LiveActivityContent.state(for: spec, stat: reading, now: .now)
        let attributes = TailorActivityAttributes(appId: document.appId, appName: dsl.app.name,
                                                  icon: spec.icon.flatMap { $0.isEmpty ? nil : $0 } ?? LiveActivityContent.defaultIcon,
                                                  key: LiveActivityContent.appKey(document.appId), statId: spec.stat)
        LiveActivities.enqueue { await LiveActivities.apply(.show(state, staleDate: stale), attributes: attributes, replace: true) }
    }

    /// `endActivity`: startActivity で出したこのアプリの表示を消す(タイマーのものは残す)。
    func endActivity() async {
        let key = LiveActivityContent.appKey(document.appId)
        LiveActivities.enqueue { await LiveActivities.end(key: key) }
    }

    /// 記録が変わった: このアプリの表示のうち stat を持つものの数字を差し替える。
    func refreshLiveActivityStats() {
        let appId = document.appId
        let targets = LiveActivities.running.filter { $0.attributes.appId == appId && $0.attributes.statId != nil }
        guard !targets.isEmpty else { return }
        for activity in targets {
            guard let statId = activity.attributes.statId else { continue }
            let reading = LiveActivityContent.statReading(id: statId, dsl: dsl, records: records)
            guard let next = LiveActivityContent.refreshed(activity.content.state, with: reading) else { continue }
            let content = ActivityContent(state: next, staleDate: activity.content.staleDate)
            LiveActivities.enqueue { await activity.update(content) }
        }
    }
}

/// ActivityKit の薄い包み。1 つのキーにつき表示は 1 つ。
@MainActor
enum LiveActivities {
    typealias Item = Activity<TailorActivityAttributes>

    static var enabled: Bool { ActivityAuthorizationInfo().areActivitiesEnabled }

    /// 開始 → 一時停止 のように続けて来た操作を順番どおりに流す(ActivityKit の呼び出しは async)。
    private static var tail: Task<Void, Never>?

    static func enqueue(_ work: @escaping @MainActor () async -> Void) {
        let previous = tail
        tail = Task { @MainActor in
            await previous?.value
            await work()
        }
    }

    static var running: [Item] {
        Item.activities.filter { $0.activityState == .active || $0.activityState == .stale }
    }

    /// - replace: 中身(static な attributes)が違っても作り直す(startActivity の出し直し)
    static func apply(_ plan: LiveActivityContent.Plan, attributes: TailorActivityAttributes, replace: Bool = false) async {
        let current = running.first { $0.attributes.key == attributes.key }
        switch plan {
        case .show(let state, let stale):
            let content = ActivityContent(state: state, staleDate: stale)
            if let current, current.attributes == attributes, !replace {
                await current.update(content)
                return
            }
            await current?.end(nil, dismissalPolicy: .immediate)
            guard enabled else { return }
            do {
                _ = try Item.request(attributes: attributes, content: content, pushType: nil)
                print("[Metamon] liveActivity: started key=\(attributes.key)")
            } catch {
                print("[Metamon] liveActivity: request failed key=\(attributes.key): \(error)")
            }
        case .finish(let state):
            await current?.end(ActivityContent(state: state, staleDate: nil),
                               dismissalPolicy: .after(.now.addingTimeInterval(LiveActivityContent.finishedLinger)))
        case .end:
            await current?.end(nil, dismissalPolicy: .immediate)
        }
    }

    static func end(key: String) async {
        for activity in running where activity.attributes.key == key {
            await activity.end(nil, dismissalPolicy: .immediate)
        }
    }
}
