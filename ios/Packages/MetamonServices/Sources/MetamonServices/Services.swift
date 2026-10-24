import Foundation

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
    /// backend 共通エラー封筒 `{error:{code,message}}`
    case server(status: Int, code: String, message: String)
    case http(status: Int, body: String)
    case invalidURL

    public var errorDescription: String? {
        switch self {
        case .server(_, let code, let message): return message.isEmpty ? code : message
        case .http(let status, let body): return "HTTP \(status): \(body.prefix(200))"
        case .invalidURL: return "URL が不正です"
        }
    }
}
