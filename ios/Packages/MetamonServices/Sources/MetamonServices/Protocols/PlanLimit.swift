import Foundation
import MetamonDSL

/// プランの上限に当たった 403 `plan_limit_reached`(#317・openapi-appconfig.yaml)。
/// `details.reason == "creator_plan_limit"` = 上限に達したのはアプリの作成者のプラン(呼び出した人は参加者・払っても直らない)。
public struct PlanLimit: Sendable, Equatable {
    public static let code = "plan_limit_reached"
    public static let creatorPlanLimitReason = "creator_plan_limit"

    /// 何の上限か(`details.resource`)
    public enum Resource: String, Sendable {
        case apps
        case recordsPerApp
        case photosPerApp
        case other
    }

    public var resource: Resource
    /// free | pro
    public var plan: String?
    public var limit: Int?
    /// Pro にすると上限が上がる(`details.upgrade`)
    public var upgrade: Bool
    /// 作成者のプランの上限(参加者には課金の導線を出さない・U3/U4)
    public var byCreator: Bool

    public init(resource: Resource, plan: String? = nil, limit: Int? = nil, upgrade: Bool = false, byCreator: Bool = false) {
        self.resource = resource
        self.plan = plan
        self.limit = limit
        self.upgrade = upgrade
        self.byCreator = byCreator
    }

    public init?(_ error: Error) {
        guard let api = error as? APIError, api.status == 403, api.code == Self.code,
              case .server(_, _, _, let d) = api else { return nil }
        var resource = Resource.other
        if case .string(let r)? = d["resource"] { resource = Resource(rawValue: r) ?? .other }
        var plan: String?
        if case .string(let p)? = d["plan"] { plan = p }
        var limit: Int?
        if case .number(let n)? = d["limit"] { limit = Int(n) }
        var upgrade = false
        if case .bool(let b)? = d["upgrade"] { upgrade = b }
        self.init(resource: resource, plan: plan, limit: limit, upgrade: upgrade,
                  byCreator: api.reason == Self.creatorPlanLimitReason)
    }
}

extension APIError {
    /// 無料プランの一時停止(#317・503 `free_plan_paused`・その日の費用の上限)。混雑・AI の停止とは別の案内を出す
    public static let freePlanPausedCode = "free_plan_paused"
    public var isFreePlanPaused: Bool { status == 503 && code == Self.freePlanPausedCode }
    /// `details.resumeAt`(再開の時刻)。無ければ `Retry-After` から
    public var resumeAt: Date? {
        guard case .server(_, _, _, let d) = self else { return nil }
        if case .string(let raw)? = d["resumeAt"], let date = ISO8601DateFormatter().date(from: raw) { return date }
        if case .number(let s)? = d["retryAfterSec"] { return Date(timeIntervalSinceNow: s) }
        return retryAfter.map { Date(timeIntervalSinceNow: $0) }
    }

    /// 運営がアプリの自動の動き(サーバーのルール)を止めている(#346・403 `app_stopped`)。理由は返らない
    public static let appStoppedCode = "app_stopped"
    public var isAppStopped: Bool { status == 403 && code == Self.appStoppedCode }

    /// 他の人に見える文字に表示できない言葉がある(#321・422 `text_not_allowed`)。`field` が直す項目
    public static let textNotAllowedCode = "text_not_allowed"
    public var isTextNotAllowed: Bool { status == 422 && code == Self.textNotAllowedCode }

    /// 仲間探しの 18 歳以上の申告が無い(#507・400 `details.reason: age_unconfirmed`)。プロフィールの投稿・仲間探しの口
    public static let ageUnconfirmedReason = "age_unconfirmed"
    public var isAgeUnconfirmed: Bool {
        guard status == 400, case .server(_, _, _, let d) = self, case .string(let r)? = d["reason"] else { return false }
        return r == Self.ageUnconfirmedReason
    }
}
