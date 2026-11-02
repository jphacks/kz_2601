import Foundation
import MetamonDSL

/// 条件で知らせる通知(#265・`Notification.when`)。レコードの保存・削除・同期の後に評価し、偽 → 真に変わった時に 1 回だけ出す。
/// 他の人の操作だけで真になった時は、この端末が同期するまで評価されない(サーバー側の評価は M4)。
public enum ConditionNotifications {
    public static let prefix = "when."

    /// "when.<appId>.<specId>"。予約の "dsl." とは別(予約し直しで消されない)。
    public static func identifier(appId: String, specId: String) -> String {
        prefix + appId + "." + specId
    }

    /// 各 `when` の今の真偽。評価できない(nil)は偽。
    public static func evaluate(
        _ specs: [NotificationSpec], dsl: AppDSL, records: [String: [Record]], now: Date = DSLClock.now()
    ) -> [String: Bool] {
        let ev = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] }, now: now)
        var out: [String: Bool] = [:]
        for spec in specs {
            guard let when = spec.when else { continue }
            out[spec.id] = ev.evaluate(when)?.truthy ?? false
        }
        return out
    }

    /// 前回 → 今回。出すのは前回が偽 → 今回が真の id だけ。
    /// 前回が無い(通知を足した直後・初めての評価)は今の状態を覚えるだけで出さない(足した時点で満たしていても鳴らさない)。
    /// 今回に無い id(DSL から消えた通知)は忘れる。
    public static func transition(previous: [String: Bool], current: [String: Bool]) -> (fire: [String], next: [String: Bool]) {
        let fire = current.filter { id, now in now && previous[id] == false }.keys.sorted()
        return (fire, current)
    }
}

/// 条件の前回の真偽をアプリ × 通知 id ごとに UserDefaults に持つ(アプリを閉じても 1 回きりを守る)。
public actor ConditionNotificationStore {
    public static let shared = ConditionNotificationStore()
    private static let key = "metamon.notifications.when.v1"
    private let defaults: UserDefaults
    /// appId → (通知 id → 前回の真偽)
    private var table: [String: [String: Bool]]

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        if let data = defaults.data(forKey: Self.key), let t = try? JSONDecoder().decode([String: [String: Bool]].self, from: data) {
            table = t
        } else {
            table = [:]
        }
    }

    /// 今回の真偽で前回を置き換え、出す通知 id を返す。
    public func update(appId: String, current: [String: Bool]) -> [String] {
        let (fire, next) = ConditionNotifications.transition(previous: table[appId] ?? [:], current: current)
        if table[appId] != next {
            table[appId] = next.isEmpty ? nil : next
            save()
        }
        return fire
    }

    public func state(appId: String) -> [String: Bool] { table[appId] ?? [:] }

    public func removeApp(_ appId: String) {
        guard table.removeValue(forKey: appId) != nil else { return }
        save()
    }

    public func removeAll() {
        table = [:]
        save()
    }

    private func save() {
        if let data = try? JSONEncoder().encode(table) { defaults.set(data, forKey: Self.key) }
    }
}
