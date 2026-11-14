import Foundation
import Observation

/// ai の「今日の残り」(#630 M1)。応答の `X-RateLimit-Remaining` / `-Limit` / `-Reset`(docs/backend/rate-limit.md)から読む。
/// 単位はサーバーの重み(新規 3・修正 2・チャット 1・text 1)。残りは依頼を数えた直後の値で、返した分(#548)は次の応答から入る。
public struct RemainingToday: Codable, Equatable, Sendable {
    public var remaining: Int
    public var limit: Int?
    /// 0:00 JST(`X-RateLimit-Reset` の UNIX 秒)
    public var resetAt: Date?
    public var observedAt: Date

    public init(remaining: Int, limit: Int? = nil, resetAt: Date? = nil, observedAt: Date) {
        self.remaining = remaining
        self.limit = limit
        self.resetAt = resetAt
        self.observedAt = observedAt
    }

    /// サーバーの重みの既定(rate-limit.md・環境変数で変わりうる)。説明の文言だけに使う
    public enum Weight {
        public static let create = 3
        public static let edit = 2
        public static let chat = 1
    }

    static let jst = TimeZone(identifier: "Asia/Tokyo")!

    /// ヘッダーから読む。残りが無い・壊れている(整数でない・負・上限より多い)時は nil。
    /// 429 は利用者の上限(`details.scope == "user"`)の時だけ使う(IP・全体の上限の数は本人の残りではない)。
    public static func parse(header: (String) -> String?, status: Int, body: Data, now: Date) -> RemainingToday? {
        func int(_ name: String) -> Int? {
            guard let raw = header(name)?.trimmingCharacters(in: .whitespaces), let n = Int(raw), n >= 0 else { return nil }
            return n
        }
        guard let remaining = int("X-RateLimit-Remaining") else { return nil }
        let limit = int("X-RateLimit-Limit")
        if let limit, remaining > limit { return nil }
        if status == 429 {
            struct Envelope: Decodable {
                struct Inner: Decodable { var details: Details? }
                struct Details: Decodable { var scope: String? }
                var error: Inner
            }
            guard (try? JSONDecoder().decode(Envelope.self, from: body))?.error.details?.scope == "user" else { return nil }
        }
        let resetAt = int("X-RateLimit-Reset").flatMap { $0 > 0 ? Date(timeIntervalSince1970: TimeInterval($0)) : nil }
        return RemainingToday(remaining: remaining, limit: limit, resetAt: resetAt, observedAt: now)
    }

    /// 戻る時刻を過ぎた(無ければ JST の日付が変わった)ら古い。古い値は出さない(その日の数は次の依頼まで分からない)。
    public func isStale(now: Date) -> Bool {
        if let resetAt { return now >= resetAt }
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = Self.jst
        return !cal.isDate(observedAt, inSameDayAs: now)
    }
}

/// 最後に分かった「今日の残り」。HTTP の応答から入り(`HTTP.noteRemaining`)、音声のシートと設定が読む。
@MainActor
@Observable
public final class RemainingTodayStore {
    nonisolated static let key = "metamon.remainingToday"

    public static let shared = RemainingTodayStore(defaults: .standard)

    public private(set) var last: RemainingToday?
    @ObservationIgnored private let defaults: UserDefaults

    public init(defaults: UserDefaults) {
        self.defaults = defaults
        last = defaults.data(forKey: Self.key).flatMap { try? JSONDecoder().decode(RemainingToday.self, from: $0) }
    }

    /// 古い応答(後から届いた前の依頼の分)では上書きしない。
    public func record(_ value: RemainingToday) {
        if let last, last.observedAt > value.observedAt { return }
        last = value
        if let data = try? JSONEncoder().encode(value) { defaults.set(data, forKey: Self.key) }
    }

    /// 今使える値(古ければ nil)。
    public func current(now: Date = .now) -> RemainingToday? {
        guard let last, !last.isStale(now: now) else { return nil }
        return last
    }

    /// サインアウト・アカウント削除で消す(別のアカウントの残りを出さない)。
    public func clear() {
        last = nil
        defaults.removeObject(forKey: Self.key)
    }
}
