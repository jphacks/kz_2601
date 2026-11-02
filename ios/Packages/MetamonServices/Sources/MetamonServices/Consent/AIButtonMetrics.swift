// 審査用ビルド(APP_STORE_REVIEW)には入れない: 栄養ラベルに「使用状況データ」が増えるため(#446)。
#if !APP_STORE_REVIEW
import Foundation
import Observation
import MetamonDSL

/// `POST /v1/metrics/ai-button` の本文(#446)。決まったラベルと回数だけ。
public struct AIButtonMetricsReport: Codable, Equatable, Sendable {
    public struct TaskCount: Codable, Equatable, Sendable {
        public var task: AIButtonMetric.Task
        public var attempts: Int
        public var failures: [String: Int]
    }

    public struct Day: Codable, Equatable, Sendable {
        /// 日ごとの乱数(再送の重複を消すため。端末・人とは結ばない)
        public var reportId: String
        /// 端末のローカル日(YYYY-MM-DD)
        public var day: String
        public var availability: AIButtonMetric.Availability
        public var tasks: [TaskCount]
    }

    public var days: [Day]
}

public protocol AIButtonMetricsSender: Sendable {
    func send(_ report: AIButtonMetricsReport) async throws
}

/// appconfig の `POST /v1/metrics/ai-button`。
public struct HTTPAIButtonMetricsSender: AIButtonMetricsSender {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    public func send(_ report: AIButtonMetricsReport) async throws {
        _ = try await HTTP.send("POST", baseURL: baseURL, path: "v1/metrics/ai-button", body: report, token: token, timeout: 20)
    }
}

/// aiButton の端末内 AI の集計(#446)。端末のローカル日ごとに数を UserDefaults に貯め、
/// 前の日までの分を 1 日 1 回まとめて送る。設定の「AI ボタンの利用状況を送る」でオフにすると、貯めた分も消して数えない。
/// uid・利用者の文・記録は持たない。
@MainActor
@Observable
public final class AIButtonMetricsRecorder {
    nonisolated static let enabledKey = "metamon.aiButtonMetrics.enabled"
    nonisolated static let bucketsKey = "metamon.aiButtonMetrics.buckets"
    /// 送らずに貯めておく日数の上限(圏外が続いても増え続けない)
    nonisolated static let keepDays = 7

    public static let shared = AIButtonMetricsRecorder(defaults: .standard)

    public private(set) var isEnabled: Bool

    struct Bucket: Codable, Equatable {
        var reportId = UUID().uuidString
        var availability: AIButtonMetric.Availability?
        var attempts: [String: Int] = [:]
        var failures: [String: [String: Int]] = [:]
    }

    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private let calendar: Calendar
    @ObservationIgnored private var buckets: [String: Bucket]
    @ObservationIgnored private var flushing = false

    public init(defaults: UserDefaults, calendar: Calendar = .current) {
        self.defaults = defaults
        self.calendar = calendar
        isEnabled = defaults.object(forKey: Self.enabledKey) as? Bool ?? true
        buckets = (defaults.data(forKey: Self.bucketsKey)).flatMap { try? JSONDecoder().decode([String: Bucket].self, from: $0) } ?? [:]
    }

    public func setEnabled(_ enabled: Bool) {
        defaults.set(enabled, forKey: Self.enabledKey)
        isEnabled = enabled
        if !enabled {
            buckets = [:]
            save()
        }
    }

    public func record(_ event: AIButtonMetric.Event, now: Date = Date()) {
        guard isEnabled else { return }
        let key = day(now)
        var b = buckets[key] ?? Bucket()
        switch event {
        case .shown(let a):
            guard b.availability != a else { return }  // 表示のたびに書かない
            b.availability = a
        case .ran(let task, let failure):
            if b.availability == nil { b.availability = .available }  // 端末内で動いた = その時は使えた
            b.attempts[task.rawValue, default: 0] += 1
            if let failure { b.failures[task.rawValue, default: [:]][failure.rawValue, default: 0] += 1 }
        }
        buckets[key] = b
        save()
    }

    /// 今日より前の日の分(新しい順に最大 keepDays 日)。無ければ nil。
    public func pendingReport(now: Date = Date()) -> AIButtonMetricsReport? {
        let today = day(now)
        let days = buckets.keys.filter { $0 < today }.sorted().suffix(Self.keepDays).compactMap { key -> AIButtonMetricsReport.Day? in
            guard let b = buckets[key], let availability = b.availability else { return nil }
            let tasks = b.attempts.keys.sorted().map { t in
                AIButtonMetricsReport.TaskCount(task: AIButtonMetric.Task(dslTask: t), attempts: b.attempts[t] ?? 0, failures: b.failures[t] ?? [:])
            }
            return .init(reportId: b.reportId, day: key, availability: availability, tasks: tasks)
        }
        return days.isEmpty ? nil : AIButtonMetricsReport(days: Array(days))
    }

    /// 前の日までの分を送る(起動・前面に戻った時)。送れたら消す。400(形の誤り)も消す。通信の失敗は次回に回す。
    public func flush(sender: any AIButtonMetricsSender, now: Date = Date()) async {
        prune(now: now)
        guard isEnabled, !flushing, let report = pendingReport(now: now) else { return }
        flushing = true
        defer { flushing = false }
        do {
            try await sender.send(report)
            drop(report)
        } catch let e as APIError where e.status == 400 {
            drop(report)
        } catch {}
    }

    private func drop(_ report: AIButtonMetricsReport) {
        for d in report.days { buckets[d.day] = nil }
        save()
    }

    /// keepDays より古い日を捨てる。
    private func prune(now: Date) {
        guard let oldest = calendar.date(byAdding: .day, value: -Self.keepDays, to: now) else { return }
        let cutoff = day(oldest)
        let before = buckets.count
        buckets = buckets.filter { $0.key >= cutoff }
        if buckets.count != before { save() }
    }

    private func save() {
        if buckets.isEmpty {
            defaults.removeObject(forKey: Self.bucketsKey)
        } else if let data = try? JSONEncoder().encode(buckets) {
            defaults.set(data, forKey: Self.bucketsKey)
        }
    }

    func day(_ date: Date) -> String {
        let c = calendar.dateComponents([.year, .month, .day], from: date)
        return String(format: "%04d-%02d-%02d", c.year ?? 0, c.month ?? 0, c.day ?? 0)
    }
}
#endif
