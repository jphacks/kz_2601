import Foundation

// MARK: - Record (appconfig `Record`: {id, entity, data, createdAt, updatedAt})

/// 生成アプリのユーザーデータ1件。`data` のキーは Field.id。
struct Record: Decodable, Identifiable, Hashable, Sendable {
    var id: String
    var data: [String: JSONValue]
    var createdAt: String?
    var updatedAt: String?

    init(id: String, data: [String: JSONValue], createdAt: String? = nil, updatedAt: String? = nil) {
        self.id = id
        self.data = data
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }

    private enum Keys: String, CodingKey { case id, data, createdAt, updatedAt }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        id = try c.decode(String.self, forKey: .id)
        data = try c.decodeIfPresent([String: JSONValue].self, forKey: .data) ?? [:]
        createdAt = try? c.decodeIfPresent(String.self, forKey: .createdAt)
        updatedAt = try? c.decodeIfPresent(String.self, forKey: .updatedAt)
    }

    subscript(field: String) -> JSONValue { data[field] ?? .null }
}

// MARK: - Protocols (本物/モックを差し替える境界)

/// appconfig サービス: DSL 取得 + entity レコード API。
/// 生成アプリのユーザーデータはすべてここ(Firestore)に保存する。iOS ローカルはキャッシュのみ。
protocol AppConfigClient: Sendable {
    func fetchDocument(appId: String) async throws -> AppDocument
    func list(appId: String, entity: String) async throws -> [Record]
    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record
    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record
    func delete(appId: String, entity: String, id: String) async throws
}

/// ai サービス: 発話テキスト → (Claude が DSL / JSON Patch 生成 → appconfig に適用) → 最新 DSL 封筒を返す。
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

/// 音声認識モデルの状態(UI 表示用)。
enum TranscriberStatus: Equatable, Sendable {
    /// モデル未ダウンロード(prepare 前)
    case notDownloaded
    /// ダウンロード中(downloadProgress 0...1)
    case downloading
    /// ダウンロード済み・CoreML ロード/特化中
    case loading
    /// 録音・文字起こし可能
    case ready
    /// 文字起こし実行中
    case recognizing
    /// 準備失敗(prepare を再試行できる)
    case failed(String)
}

/// 音声入力。実機は `WhisperKitRecognizer`、シミュレータ/テストは `MockTranscriber`。
@MainActor
protocol SpeechTranscriber: AnyObject {
    var isReady: Bool { get }
    var status: TranscriberStatus { get }
    /// モデル DL 進捗 0...1(downloading 以外は意味を持たない)
    var downloadProgress: Double { get }
    /// モデル DL / ロード。初回は時間がかかるのでアプリ起動直後に非同期で呼ぶ。
    func prepare() async throws
    func startRecording() async throws
    func stopAndTranscribe() async throws -> String
}

extension SpeechTranscriber {
    var status: TranscriberStatus { isReady ? .ready : .loading }
    var downloadProgress: Double { 0 }
}

// MARK: - Container

struct Services {
    var appConfig: any AppConfigClient
    var ai: any AIClient
    var sync: any DocumentSync
    var token: any TokenProvider
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
