import Foundation

/// 新規作成(初期画面からの生成)の無料枠の判定(#77)。純粋関数のみ。
/// 改変(生成後の「ここを変えて」)は数えない。回数は端末側で数える(サーバー側の強制は後続・docs/ios/architecture.md §18)。
public struct CreationQuota: Equatable, Sendable {
    public enum Decision: Equatable, Sendable {
        /// Pro または制限の無効化(デモ端末)
        case unlimited
        /// 無料枠内。`remaining` は今回の分を含めた残り回数
        case allowed(remaining: Int)
        /// 今日の無料枠を使い切った
        case blocked
    }

    public var dailyFreeLimit: Int

    public init(dailyFreeLimit: Int = ProProducts.dailyFreeCreations) {
        self.dailyFreeLimit = max(0, dailyFreeLimit)
    }

    public func decide(used: Int, isPro: Bool, limitDisabled: Bool) -> Decision {
        if isPro || limitDisabled { return .unlimited }
        let remaining = dailyFreeLimit - max(0, used)
        return remaining > 0 ? .allowed(remaining: remaining) : .blocked
    }
}

/// 今日の新規作成回数。日付が変わったら 0 から数え直す(端末のローカル日付)。
public struct CreationLedger: Codable, Equatable, Sendable {
    /// `yyyy-MM-dd`(ローカル日付)
    public var day: String
    public var count: Int

    public init(day: String = "", count: Int = 0) {
        self.day = day
        self.count = count
    }

    public static func dayKey(_ date: Date, calendar: Calendar = .current) -> String {
        let c = calendar.dateComponents([.year, .month, .day], from: date)
        return String(format: "%04d-%02d-%02d", c.year ?? 0, c.month ?? 0, c.day ?? 0)
    }

    public func used(on date: Date, calendar: Calendar = .current) -> Int {
        day == Self.dayKey(date, calendar: calendar) ? count : 0
    }

    public func recording(on date: Date, calendar: Calendar = .current) -> CreationLedger {
        let key = Self.dayKey(date, calendar: calendar)
        return CreationLedger(day: key, count: (day == key ? count : 0) + 1)
    }
}

/// `CreationLedger` の保存先(UserDefaults)。
public final class CreationUsageStore: @unchecked Sendable {
    private let defaults: UserDefaults
    private let key: String

    public init(defaults: UserDefaults = .standard, key: String = "metamon.pro.creationLedger") {
        self.defaults = defaults
        self.key = key
    }

    public var ledger: CreationLedger {
        guard let data = defaults.data(forKey: key),
              let ledger = try? JSONDecoder().decode(CreationLedger.self, from: data) else { return CreationLedger() }
        return ledger
    }

    public func used(on date: Date = .now, calendar: Calendar = .current) -> Int {
        ledger.used(on: date, calendar: calendar)
    }

    public func record(on date: Date = .now, calendar: Calendar = .current) {
        let next = ledger.recording(on: date, calendar: calendar)
        if let data = try? JSONEncoder().encode(next) { defaults.set(data, forKey: key) }
    }

    public func reset() { defaults.removeObject(forKey: key) }
}
