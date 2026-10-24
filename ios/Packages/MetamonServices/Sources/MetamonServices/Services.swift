import Foundation
import MetamonDSL

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
        case .invalidURL: return "URL が不正です"
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
    /// 混雑・一時障害(429 / 500 / 502 / 503)。`retryAfter` はサーバー指示の待ち秒
    case busy(retryAfter: TimeInterval?)
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
