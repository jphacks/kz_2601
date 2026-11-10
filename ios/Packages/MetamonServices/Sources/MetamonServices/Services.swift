import Foundation
import MetamonDSL

/// `Services` に機能ごとのサービスを足すためのキー(#516)。1 機能 = 1 キー + `Services+<機能>.swift` の computed property。
public protocol ServiceKey {
    associatedtype Value
    static var defaultValue: Value { get }
}

/// 全サービスの束(DI コンテナ)。アプリ本体の `BuildConfig.make(environment:)` が起動時に1回だけ組み立て、
/// SwiftUI Environment(`\.services`)と `AppRuntime` に渡す。利用側は protocol にしか依存しない。
public struct Services {
    public var auth: any AuthService
    public var appConfig: any AppConfigClient
    public var ai: any AIClient
    public var sync: any DocumentSync
    public var token: any TokenProvider
    public var transcriber: any SpeechTranscriber
    public var notifications: any NotificationScheduler
    /// true なら起動時にサーバー(appconfig)から DSL を取得し、ホットリロードを開始する。
    public var isLive: Bool
    /// 機能ごとのサービス(`ServiceKey` で引く)。プロパティは `<機能>/Services+<機能>.swift` の extension に置く(#516)。
    private var storage: [ObjectIdentifier: Any] = [:]

    public init(
        auth: any AuthService,
        appConfig: any AppConfigClient,
        ai: any AIClient,
        sync: any DocumentSync,
        token: any TokenProvider,
        transcriber: any SpeechTranscriber,
        notifications: any NotificationScheduler,
        isLive: Bool
    ) {
        self.auth = auth
        self.appConfig = appConfig
        self.ai = ai
        self.sync = sync
        self.token = token
        self.transcriber = transcriber
        self.notifications = notifications
        self.isLive = isLive
    }

    /// `EnvironmentValues` と同じ形。未設定なら `K.defaultValue`。
    public subscript<K: ServiceKey>(_ key: K.Type) -> K.Value {
        get {
            if let value = storage[ObjectIdentifier(key)], let typed = value as? K.Value { return typed }
            return K.defaultValue
        }
        set { storage[ObjectIdentifier(key)] = newValue }
    }
}

public enum APIError: LocalizedError {
    /// backend 共通エラー封筒 `{error:{code,message,details?}}`(例: 422 `invalid_record` は `details.field` に Field.id)
    case server(status: Int, code: String, message: String, details: [String: JSONValue] = [:])
    case http(status: Int, body: String)
    case invalidURL

    public var errorDescription: String? {
        switch self {
        case .server(_, let code, let message, _): return message.isEmpty ? code : message
        case .http(let status, let body): return "HTTP \(status): \(body.prefix(200))"
        case .invalidURL: return L("URL が不正です")
        }
    }

    public var status: Int? {
        switch self {
        case .server(let s, _, _, _), .http(let s, _): return s
        case .invalidURL: return nil
        }
    }

    public var code: String? {
        if case .server(_, let c, _, _) = self { return c }
        return nil
    }

    /// App Check で弾かれた 401(#318)。ID トークンの失効(`sessionExpired`)と区別する
    public var isAppCheckFailure: Bool { status == 401 && code == AppCheckHeader.failedCode }

    /// 運営が AI の生成を一時停止している 503(#511・`AI_PAUSED`)。混雑・上限とは別の案内を出す
    public var isAIPaused: Bool { status == 503 && code == Self.aiPausedCode }
    public static let aiPausedCode = "ai_paused"

    /// `details.field`(422 `invalid_record` / 400 unknown field)。toast で「どの項目か」を出すために使う。
    public var field: String? {
        if case .server(_, _, _, let d) = self, case .string(let f)? = d["field"] { return f }
        return nil
    }

    /// `HTTP.error(from:)` が `Retry-After` ヘッダを秒で入れる details キー(429 / 503)。
    public static let retryAfterKey = "_retryAfter"

    /// サーバーが指示した再試行までの秒数(429 / 503 の `Retry-After`)。無ければ nil。
    public var retryAfter: TimeInterval? {
        if case .server(_, _, _, let d) = self, case .number(let s)? = d[Self.retryAfterKey] { return s }
        return nil
    }

    /// ai の 1 日の上限(#280・429 `rate_limited` + `details.scope`)。Claude 側の混雑の 429 は nil(= busy)。
    public var rateLimit: RateLimitInfo? {
        guard case .server(429, "rate_limited", _, let d) = self, case .string(let scope)? = d["scope"] else { return nil }
        func int(_ key: String) -> Int? {
            if case .number(let n)? = d[key] { return Int(n) }
            return nil
        }
        var resetAt: Date?
        if case .string(let raw)? = d["resetAt"] { resetAt = ISO8601DateFormatter().date(from: raw) }
        if resetAt == nil, let wait = int("retryAfterSec") ?? retryAfter.map(Int.init) {
            resetAt = Date(timeIntervalSinceNow: TimeInterval(wait))
        }
        var tier: String?
        if case .string(let t)? = d["tier"] { tier = t }
        return RateLimitInfo(scope: RateLimitInfo.Scope(rawValue: scope) ?? .user, tier: tier,
                             limit: int("limit"), remaining: int("remaining"), resetAt: resetAt)
    }
}

/// サーバーの 1 日の上限に当たった時の中身(#280・openapi-ai.yaml `RateLimitDetails`)。
public struct RateLimitInfo: Equatable, Sendable {
    public enum Scope: String, Sendable {
        /// 利用者の 1 日の上限
        case user
        /// 同じネットワークからの匿名利用の上限(サインインで外れる)
        case ip
        /// 環境全体の費用ブレーカー(全員が止まる)
        case global
    }

    public var scope: Scope
    /// anonymous | signed_in | pro
    public var tier: String?
    /// 1 日の上限(単位)
    public var limit: Int?
    public var remaining: Int?
    /// 次に使える時刻(翌 0:00 JST)
    public var resetAt: Date?

    public init(scope: Scope, tier: String? = nil, limit: Int? = nil, remaining: Int? = nil, resetAt: Date? = nil) {
        self.scope = scope
        self.tier = tier
        self.limit = limit
        self.remaining = remaining
        self.resetAt = resetAt
    }

    public var isAnonymous: Bool { tier == "anonymous" }
}

/// 失敗の種類(#60)。UI の状態カード(`ErrorPresentation`)はこの分類だけを見て文言と主ボタンを決める。
/// 「次に何をすればいいか」が分類ごとに 1 つ決まる: offline → 復帰待ち / busy・timeout → もう一度 / sessionExpired → サインイン / conflict → 最新を取り直す。
public enum FailureKind: Equatable, Sendable {
    /// 圏外・接続断(`URLError` notConnectedToInternet / networkConnectionLost / cannotConnectToHost / dnsLookupFailed …)
    case offline
    /// 応答が返らなかった(`URLError.timedOut` / 504 `upstream_timeout`)
    case timeout
    /// 401(トークン再取得後も 401)。サインインし直すしかない
    case sessionExpired
    /// 401 `app_check_failed`(#318・App Check のトークンを取り直しても弾かれた)。サインインでは直らないので
    /// サインイン画面へ戻さない。正規の版を入れ直す / 少し待って再試行を案内する
    case appCheckFailed
    /// 運営が AI の生成を一時停止中(#511・503 `ai_paused`)。混雑(busy)・今日の上限(rateLimited)とは別の文言
    case aiPaused
    /// 無料プランの一時停止(#317・503 `free_plan_paused`)。`resumeAt` = 再開の時刻。もう一度では解けない
    case freePlanPaused(resumeAt: Date?)
    /// プランの上限(#317・403 `plan_limit_reached`)。作成者なら Pro への導線、参加者なら作った人に知らせる案内
    case planLimit(PlanLimit)
    /// 持てるアプリの数の上限(#630・402 `app_limit_reached`)。どれかと置き換えるか枠を買えば、同じ操作をやり直せる
    case appLimit(AppLimit)
    /// 運営がアプリの自動の動きを止めている(#346・403 `app_stopped`)
    case appStopped
    /// 混雑・一時障害(429 / 500 / 502 / 503)。`retryAfter` はサーバー指示の待ち秒
    case busy(retryAfter: TimeInterval?)
    /// 今日の上限に達した(#280・429 `rate_limited`)。「もう一度」では解けないので再試行させない
    case rateLimited(RateLimitInfo)
    /// 409 version 競合。最新を取り直して再適用する
    case conflict
    /// Task キャンセル(ユーザー操作)
    case cancelled
    /// その他の 4xx(入力不備など)。message は人向け
    case rejected(String)
    /// 分類不能(デコード失敗など)
    case other(String)

    public static func classify(_ error: Error) -> FailureKind {
        if error is CancellationError { return .cancelled }
        if let url = error as? URLError {
            switch url.code {
            case .timedOut: return .timeout
            case .cancelled: return .cancelled
            case .notConnectedToInternet, .networkConnectionLost, .cannotConnectToHost, .cannotFindHost,
                 .dnsLookupFailed, .internationalRoamingOff, .dataNotAllowed, .callIsActive:
                return .offline
            default: return .other(url.localizedDescription)
            }
        }
        if let api = error as? APIError {
            if let info = api.rateLimit { return .rateLimited(info) }
            if api.isAppCheckFailure { return .appCheckFailed }
            if api.isAIPaused { return .aiPaused }
            if api.isFreePlanPaused { return .freePlanPaused(resumeAt: api.resumeAt) }
            if let limit = PlanLimit(api) { return .planLimit(limit) }
            if let limit = AppLimit(api) { return .appLimit(limit) }
            if api.isAppStopped { return .appStopped }
            switch api.status {
            case 401: return .sessionExpired
            case 409: return .conflict
            case 429, 500, 502, 503: return .busy(retryAfter: api.retryAfter)
            case 504: return .timeout
            case .some(400..<500): return .rejected(api.errorDescription ?? "\(api)")
            case .some(500...): return .busy(retryAfter: api.retryAfter)
            default: return .other(api.errorDescription ?? "\(api)")
            }
        }
        return .other(error.localizedDescription)
    }

    /// 「もう一度」で同じ操作をやり直す意味がある失敗か(圏外・混雑・タイムアウト)。
    public var isRetryable: Bool {
        switch self {
        case .offline, .timeout, .busy: return true
        default: return false
        }
    }
}
