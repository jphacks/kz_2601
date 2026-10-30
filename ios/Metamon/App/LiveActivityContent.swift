import Foundation
import MetamonDSL
import MetamonRenderer

/// DSL → ライブアクティビティの中身(#423)。ActivityKit に触らない純粋関数なのでユニットテストできる。
@MainActor
enum LiveActivityContent {
    typealias State = TailorActivityAttributes.ContentState

    /// 表示をどうするか。
    enum Plan: Equatable {
        /// 出す / 更新する。`staleDate` は countdown の終了時刻(過ぎたらウィジェットが「終了」を出す)
        case show(State, staleDate: Date?)
        /// 終了表示のまま残して消す
        case finish(State)
        /// すぐ消す
        case end
    }

    /// stat / progressBar を読んだ結果。
    struct StatReading: Equatable {
        var label: String?
        var value: String?
        var progress: Double?
    }

    static let defaultIcon = "timer"
    /// 終わった表示をロック画面に残す時間
    static let finishedLinger: TimeInterval = 15 * 60

    static func timerKey(_ key: String) -> String { "timer:" + key }
    static func appKey(_ appId: String) -> String { "app:" + appId }

    // MARK: - timerDisplay.liveActivity

    static func plan(for update: TimerActivityUpdate) -> Plan {
        var state = State(title: update.label, subtitle: nil, clock: nil, value: nil, valueLabel: nil, progress: nil, finished: false)
        switch update.phase {
        case .running(let start, let end):
            if let end {
                state.clock = .countdown(start: start, end: end)
            } else {
                state.clock = .elapsed(since: start)
            }
            return .show(state, staleDate: end)
        case .paused(let shown):
            state.clock = .paused(seconds: shown)
            state.subtitle = L10n.tr("一時停止中")
            return .show(state, staleDate: nil)
        case .finished:
            state.clock = .paused(seconds: 0)
            state.subtitle = L10n.tr("終わりました")
            state.finished = true
            return .finish(state)
        case .ended:
            return .end
        }
    }

    // MARK: - startActivity

    static func state(for spec: ActivitySpec, stat: StatReading?, now: Date) -> (State, staleDate: Date?) {
        var clock: TailorActivityAttributes.Clock = .elapsed(since: now)
        var stale: Date?
        if let minutes = spec.minutes, minutes > 0 {
            let end = now.addingTimeInterval(TimeInterval(min(minutes, maxMinutes) * 60))
            clock = .countdown(start: now, end: end)
            stale = end
        }
        let state = State(title: spec.title, subtitle: nonEmpty(spec.subtitle), clock: clock,
                          value: stat?.value, valueLabel: stat?.label, progress: stat?.progress, finished: false)
        return (state, stale)
    }

    /// iOS はライブアクティビティを 8 時間で終える(Go の dsl.MaxActivityMinutes と同じ)
    static let maxMinutes = 480

    /// 記録が変わった時: 数字だけ差し替える。変わっていなければ nil(更新回数を節約)。
    static func refreshed(_ state: State, with stat: StatReading?) -> State? {
        var next = state
        next.value = stat?.value
        next.valueLabel = stat?.label
        next.progress = stat?.progress
        return next == state ? nil : next
    }

    // MARK: - stat / progressBar

    static func statReading(id: String, dsl: AppDSL, records: [String: [Record]]) -> StatReading? {
        guard let c = dsl.screens.lazy.flatMap(\.allComponents).first(where: { $0.id == id }) else { return nil }
        let label = nonEmpty(c.label)
        switch c.type {
        case .stat:
            let evaluator = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
            return StatReading(label: label, value: WidgetSnapshotWriter.statText(c, dsl: dsl, records: records, evaluator: evaluator))
        case .progressBar:
            guard let ratio = progress(c, dsl: dsl, records: records) else { return StatReading(label: label) }
            return StatReading(label: label, value: "\(Int((ratio * 100).rounded()))%", progress: ratio)
        default:
            return nil
        }
    }

    /// `ProgressBarLeaf` と同じ式(リテラル value か aggregate(field) / target)。レコード未取得なら nil。
    static func progress(_ c: Component, dsl: AppDSL, records: [String: [Record]]) -> Double? {
        if let v = c.value { return v }
        guard let entityId = c.entity, let target = c.target, target > 0, records[entityId] != nil else { return nil }
        let rows = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] }).viewRecords(for: entityId) // computed field も数える
        let filtered = Aggregator.apply(c.filter, entity: dsl.entity(entityId), to: rows)
        return Aggregator.aggregate(c.aggregate ?? "sum", field: c.field, over: filtered) / target
    }

    private static func nonEmpty(_ s: String?) -> String? {
        guard let s, !s.trimmingCharacters(in: .whitespaces).isEmpty else { return nil }
        return s
    }
}
