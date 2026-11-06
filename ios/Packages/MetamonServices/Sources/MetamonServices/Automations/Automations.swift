// 審査用ビルド(APP_STORE_REVIEW)では外す: 審査用の版は AI が serverRules を作らない(#614)ので出す先が無い。
#if !APP_STORE_REVIEW
import Foundation

/// アプリの「自動の動き」(serverRules)の一覧と直近の結果(`GET /v1/apps/{appId}/automations`・#347)。作成者だけが読める。
public struct AppAutomations: Decodable, Sendable, Equatable {
    public var appId: String
    /// 運営がアプリの自動の動きを全部止めている
    public var stopped: AutomationStop?
    /// 自動の動きは動くが、通知は止めている
    public var notifyPaused: AutomationStop?
    public var quota: AutomationQuota?
    public var rules: [AutomationRule]

    public init(appId: String, stopped: AutomationStop? = nil, notifyPaused: AutomationStop? = nil, quota: AutomationQuota? = nil, rules: [AutomationRule] = []) {
        self.appId = appId
        self.stopped = stopped
        self.notifyPaused = notifyPaused
        self.quota = quota
        self.rules = rules
    }

    enum CodingKeys: String, CodingKey { case appId, stopped, notifyPaused, quota, rules }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        appId = try c.decodeIfPresent(String.self, forKey: .appId) ?? ""
        stopped = try c.decodeIfPresent(AutomationStop.self, forKey: .stopped)
        notifyPaused = try c.decodeIfPresent(AutomationStop.self, forKey: .notifyPaused)
        quota = try? c.decodeIfPresent(AutomationQuota.self, forKey: .quota)
        rules = try c.decodeIfPresent([AutomationRule].self, forKey: .rules) ?? []
    }
}

/// 運営の停止が効いている印。理由はサーバーが返さない。
public struct AutomationStop: Decodable, Sendable, Equatable {
    /// 自動で解ける時刻(nil = 運営が解くまで)
    public var until: Date?

    public init(until: Date? = nil) { self.until = until }

    enum CodingKeys: String, CodingKey { case until }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        until = try c.decodeIfPresent(String.self, forKey: .until).flatMap(AppSummary.parseDate)
    }
}

/// アプリの今日の実行の数(日本時間の 0 時で戻る)。
public struct AutomationQuota: Decodable, Sendable, Equatable {
    public var used: Int
    public var limit: Int
    public var resetAt: Date?

    public init(used: Int, limit: Int, resetAt: Date? = nil) {
        self.used = used
        self.limit = limit
        self.resetAt = resetAt
    }

    enum CodingKeys: String, CodingKey { case used, limit, resetAt }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        used = try c.decode(Int.self, forKey: .used)
        limit = try c.decode(Int.self, forKey: .limit)
        resetAt = try c.decodeIfPresent(String.self, forKey: .resetAt).flatMap(AppSummary.parseDate)
    }
}

/// ルール 1 本。
public struct AutomationRule: Decodable, Sendable, Equatable, Identifiable {
    /// 直近の結果(openapi `AutomationResult`)。知らない語は `.other`。
    public enum Result: String, Sendable, Equatable, CaseIterable {
        case none, ok, notifyPaused = "notify_paused", limit, serviceLimit = "service_limit", servicePaused = "service_paused"
        case plan, stopped, changed, unavailable, late, inactive, failed, other

        /// 止まった・動かなかった(理由とお問い合わせへの導線を出す)
        public var isProblem: Bool { self != .none && self != .ok }
    }

    public enum Trigger: String, Decodable, Sendable, Equatable { case record, schedule }

    public var id: String
    /// サーバーが作る説明(日本語)
    public var description: String
    public var trigger: Trigger
    public var result: Result
    public var lastRunAt: Date?
    public var lastOkAt: Date?
    /// 運営がこのルールを止めている
    public var stopped: AutomationStop?

    public init(id: String, description: String, trigger: Trigger = .record, result: Result = .none,
                lastRunAt: Date? = nil, lastOkAt: Date? = nil, stopped: AutomationStop? = nil) {
        self.id = id
        self.description = description
        self.trigger = trigger
        self.result = result
        self.lastRunAt = lastRunAt
        self.lastOkAt = lastOkAt
        self.stopped = stopped
    }

    enum CodingKeys: String, CodingKey { case id, description, trigger, result, lastRunAt, lastOkAt, stopped }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        description = try c.decodeIfPresent(String.self, forKey: .description) ?? id
        trigger = (try? c.decode(Trigger.self, forKey: .trigger)) ?? .record
        result = (try c.decodeIfPresent(String.self, forKey: .result)).map { Result(rawValue: $0) ?? .other } ?? .none
        lastRunAt = try c.decodeIfPresent(String.self, forKey: .lastRunAt).flatMap(AppSummary.parseDate)
        lastOkAt = try c.decodeIfPresent(String.self, forKey: .lastOkAt).flatMap(AppSummary.parseDate)
        stopped = try c.decodeIfPresent(AutomationStop.self, forKey: .stopped)
    }
}

/// 自動の動きの見える化(#347)。
public protocol AutomationsClient: Sendable {
    /// 作成者以外は 403(`APIError.status == 403`)。
    func automations(appId: String) async throws -> AppAutomations
}

/// live の時だけ差し込む(モックとインメモリでは nil = 設定の行を出さない)。
public enum AutomationsServiceKey: ServiceKey {
    public static var defaultValue: (any AutomationsClient)? { nil }
}

extension Services {
    public var automations: (any AutomationsClient)? {
        get { self[AutomationsServiceKey.self] }
        set { self[AutomationsServiceKey.self] = newValue }
    }
}
#endif
