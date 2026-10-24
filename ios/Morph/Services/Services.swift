import Foundation

// MARK: - Protocols (本物/モックを差し替える境界)

/// appconfig サービス: DSL ドキュメント取得 + 汎用エンティティ/レコード API。
/// 生成アプリのユーザーデータはすべてここ(Firestore)に保存する。iOS ローカルはキャッシュのみ。
protocol AppConfigClient: Sendable {
    func fetchDocument(appId: String) async throws -> AppDocument
    func list(appId: String, entity: String, query: JSONValue?) async throws -> [JSONValue]
    func create(appId: String, entity: String, data: JSONValue) async throws -> JSONValue
    func update(appId: String, entity: String, id: String, data: JSONValue) async throws -> JSONValue
    func delete(appId: String, entity: String, id: String) async throws
}

/// ai サービス: 発話テキスト → (Claude が JSON Patch 生成 → appconfig に適用) → 最新 DSL を返す。
protocol AIClient: Sendable {
    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> AppDocument
}

/// ホットリロード: 新しい version の DSL を流す。
protocol DocumentSync: Sendable {
    func updates(appId: String, after version: Int) -> AsyncStream<AppDocument>
}

/// Firebase Auth の ID トークン供給。未ログインなら nil。
protocol TokenProvider: Sendable {
    func idToken() async throws -> String?
}

/// ネイティブ機能ブリッジ。`native` アクションの feature 名で分岐する。
@MainActor
protocol NativeBridge {
    func perform(_ feature: String, args: JSONValue) async throws -> JSONValue
}

/// 音声入力。WhisperKit 実装に差し替える前提のインターフェース。
@MainActor
protocol SpeechTranscriber: AnyObject {
    var isReady: Bool { get }
    /// モデル DL / ロード。初回は時間がかかるのでアプリ起動直後に非同期で呼ぶ。
    func prepare() async throws
    func startRecording() async throws
    func stopAndTranscribe() async throws -> String
}

// MARK: - Container

struct Services {
    var appConfig: any AppConfigClient
    var ai: any AIClient
    var sync: any DocumentSync
    var token: any TokenProvider
    var native: any NativeBridge
    var transcriber: any SpeechTranscriber
    /// true なら起動時にサーバーから DSL を取得し、ホットリロードを開始する。
    var isLive: Bool
}

enum APIError: LocalizedError {
    case http(status: Int, body: String)
    case invalidURL

    var errorDescription: String? {
        switch self {
        case .http(let status, let body): return "HTTP \(status): \(body.prefix(200))"
        case .invalidURL: return "URL が不正です"
        }
    }
}
