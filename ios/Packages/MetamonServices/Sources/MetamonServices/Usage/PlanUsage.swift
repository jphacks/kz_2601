import Foundation
import Observation

/// 使っている量と上限(#317・#630・openapi-appconfig.yaml の `UsageMeter`)。`limit` / `remaining` が nil = 数えない。
public struct UsageMeter: Codable, Equatable, Sendable {
    public var used: Int
    public var limit: Int?
    public var remaining: Int?

    public init(used: Int, limit: Int?, remaining: Int? = nil) {
        self.used = used
        self.limit = limit
        self.remaining = remaining ?? limit.map { max(0, $0 - used) }
    }

    /// 上限に対する割合(数えない・上限 0 は nil)
    public var fraction: Double? {
        guard let limit, limit > 0 else { return nil }
        return Double(used) / Double(limit)
    }

    public var isAtLimit: Bool { limit.map { used >= $0 } ?? false }
}

/// `GET /v1/me/usage`。本人の今日の AI の残り・アプリの数・プランのアプリごとの上限。
public struct MyUsage: Decodable, Equatable, Sendable {
    public struct AI: Decodable, Equatable, Sendable {
        public var used: Int
        public var limit: Int?
        public var remaining: Int?
    }
    public struct PerApp: Decodable, Equatable, Sendable {
        public var recordsPerApp: Int?
        public var photosPerApp: Int?
    }

    /// free | pro
    public var plan: String
    public var tier: String?
    public var resetAt: Date?
    public var ai: AI?
    public var apps: UsageMeter?
    public var perApp: PerApp?

    public var isPro: Bool { plan == "pro" }

    public init(plan: String, tier: String? = nil, resetAt: Date? = nil, ai: AI? = nil, apps: UsageMeter?, perApp: PerApp? = nil) {
        self.plan = plan
        self.tier = tier
        self.resetAt = resetAt
        self.ai = ai
        self.apps = apps
        self.perApp = perApp
    }

    public static func decode(_ data: Data) throws -> MyUsage {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { d in
            let raw = try d.singleValueContainer().decode(String.self)
            if let date = ISO8601DateFormatter().date(from: raw) { return date }
            let frac = ISO8601DateFormatter()
            frac.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = frac.date(from: raw) { return date }
            throw DecodingError.dataCorrupted(.init(codingPath: d.codingPath, debugDescription: "resetAt: \(raw)"))
        }
        return try decoder.decode(MyUsage.self, from: data)
    }

    /// 「今日の残り」(X3: 単位の数は「今日の残り N」としてだけ出す)。数えない・読めない時は nil
    public func remainingToday(now: Date) -> RemainingToday? {
        guard let ai, let remaining = ai.remaining else { return nil }
        return RemainingToday(remaining: remaining, limit: ai.limit, resetAt: resetAt, observedAt: now)
    }
}

/// `GET /v1/apps/{appId}/usage`。払う人のプランで数えた記録・写真。
public struct AppUsage: Decodable, Equatable, Sendable {
    public var appId: String
    /// 払う人のプラン(free | pro)
    public var plan: String
    /// self = 自分のアプリ・creator = 他の人が作ったアプリ(作成者のプランで数える)
    public var payer: String
    /// Pro にすると記録か写真の上限が上がる
    public var upgrade: Bool
    public var records: UsageMeter?
    public var photos: UsageMeter?

    public init(appId: String, plan: String, payer: String, upgrade: Bool, records: UsageMeter?, photos: UsageMeter?) {
        self.appId = appId
        self.plan = plan
        self.payer = payer
        self.upgrade = upgrade
        self.records = records
        self.photos = photos
    }

    /// 自分のアプリ(作成者)。参加者には量も Pro も出さない
    public var isMine: Bool { payer == "self" }
}

/// 使用量を読む口(`HTTPAppConfigClient` が実装)。nil = サーバーが古くて無い(404)・このアプリの写しが無い。
public protocol PlanUsageSource: Sendable {
    func myUsage() async throws -> MyUsage?
    func appUsage(appId: String) async throws -> AppUsage?
}

/// 上限に近い時の 1 行のお知らせ(#317)。断られる前に知らせる。
public struct UsageNotice: Equatable, Sendable, Identifiable {
    public var resource: PlanLimit.Resource
    /// 記録・写真はアプリごと
    public var appId: String?
    public var meter: UsageMeter
    /// Pro にすると上がる(作成者だけ。参加者のお知らせはそもそも作らない)
    public var upgrade: Bool

    public init(resource: PlanLimit.Resource, appId: String? = nil, meter: UsageMeter, upgrade: Bool) {
        self.resource = resource
        self.appId = appId
        self.meter = meter
        self.upgrade = upgrade
    }

    /// 1 日 1 回の記録に使う(リソース + アプリ)
    public var id: String { appId.map { "\(resource.rawValue):\($0)" } ?? resource.rawValue }
}

public enum UsageNoticeRules {
    /// 80% 以上で知らせる
    public static let threshold = 0.8

    public static func isNear(_ meter: UsageMeter?) -> Bool {
        guard let fraction = meter?.fraction else { return false }
        return fraction >= threshold
    }

    /// アプリの数(アプリ一覧に出す)
    public static func appsNotice(_ me: MyUsage?) -> UsageNotice? {
        guard let me, let apps = me.apps, isNear(apps) else { return nil }
        return UsageNotice(resource: .apps, meter: apps, upgrade: !me.isPro)
    }

    /// 記録・写真(そのアプリの中に出す)。参加者(他の人のアプリ)には出さない
    public static func appNotices(_ usage: AppUsage?) -> [UsageNotice] {
        guard let usage, usage.isMine else { return [] }
        var out: [UsageNotice] = []
        if let r = usage.records, isNear(r) {
            out.append(UsageNotice(resource: .recordsPerApp, appId: usage.appId, meter: r, upgrade: usage.upgrade))
        }
        if let p = usage.photos, isNear(p) {
            out.append(UsageNotice(resource: .photosPerApp, appId: usage.appId, meter: p, upgrade: usage.upgrade))
        }
        return out
    }
}

/// お知らせは 1 つのリソースにつき 1 日 1 回(出した時点で数える・閉じても同じ日はもう出さない)。日付は JST(上限の戻る時刻と同じ)
public struct UsageNoticeLog: @unchecked Sendable {
    static let key = "metamon.usageNotice.shownDay"
    let defaults: UserDefaults

    public init(defaults: UserDefaults = .standard) { self.defaults = defaults }

    static func day(_ date: Date) -> String {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = RemainingToday.jst
        let c = cal.dateComponents([.year, .month, .day], from: date)
        return "\(c.year ?? 0)-\(c.month ?? 0)-\(c.day ?? 0)"
    }

    private var shown: [String: String] { defaults.dictionary(forKey: Self.key) as? [String: String] ?? [:] }

    public func shouldShow(_ notice: UsageNotice, now: Date = .now) -> Bool {
        shown[notice.id] != Self.day(now)
    }

    public func markShown(_ notice: UsageNotice, now: Date = .now) {
        let today = Self.day(now)
        var map = shown.filter { $0.value == today } // 前の日の分は捨てる(増え続けない)
        map[notice.id] = today
        defaults.set(map, forKey: Self.key)
    }

    public func clear() { defaults.removeObject(forKey: Self.key) }
}

/// 使用量の短いキャッシュ(5 分)。作る・消す・記録を送ったら `invalidate…` で古くし、次に見る時に読み直す。
/// 古いサーバー(404)・読めない時は出さない(前の値も消さずに使う・404 は消す)。
@MainActor
@Observable
public final class PlanUsageStore {
    public static let shared = PlanUsageStore()
    public static let ttl: TimeInterval = 5 * 60

    public private(set) var me: MyUsage?
    public private(set) var apps: [String: AppUsage] = [:]
    /// 古くした回数。お知らせのホストが `task(id:)` で読み直す
    public private(set) var revision = 0
    @ObservationIgnored private var meFetchedAt: Date?
    @ObservationIgnored private var appFetchedAt: [String: Date] = [:]

    public init() {}

    public func app(_ appId: String) -> AppUsage? { apps[appId] }

    /// 自分の分。新しければ読まない
    @discardableResult
    public func refreshMe(source: any PlanUsageSource, force: Bool = false, now: Date = .now) async -> MyUsage? {
        if !force, let at = meFetchedAt, now.timeIntervalSince(at) < Self.ttl { return me }
        meFetchedAt = now
        do {
            me = try await source.myUsage()
        } catch {
            meFetchedAt = nil // 次に見る時にまた読む
            print("[Metamon] usage: me failed: \(error)")
        }
        return me
    }

    @discardableResult
    public func refreshApp(appId: String, source: any PlanUsageSource, force: Bool = false, now: Date = .now) async -> AppUsage? {
        if !force, let at = appFetchedAt[appId], now.timeIntervalSince(at) < Self.ttl { return apps[appId] }
        appFetchedAt[appId] = now
        do {
            apps[appId] = try await source.appUsage(appId: appId)
        } catch {
            appFetchedAt[appId] = nil
            print("[Metamon] usage: app=\(appId) failed: \(error)")
        }
        return apps[appId]
    }

    /// アプリを作った・消した
    public func invalidateMe() {
        meFetchedAt = nil
        revision += 1
    }

    /// 記録・写真を送った・消した
    public func invalidateApp(_ appId: String) {
        appFetchedAt[appId] = nil
        revision += 1
    }

    /// サインアウト・アカウント削除(別のアカウントの量を出さない)
    public func clear() {
        me = nil
        apps = [:]
        meFetchedAt = nil
        appFetchedAt = [:]
        revision += 1
    }
}
