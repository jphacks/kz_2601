import Foundation

/// インメモリの appconfig。サーバー無しで DSL レンダラーを動かすためのもの。
actor InMemoryAppConfigClient: AppConfigClient {
    private var tables: [String: [JSONValue]]

    init(seed: [String: [JSONValue]] = [:]) { tables = seed }

    func fetchDocument(appId: String) async throws -> AppDocument {
        try AppDocument.bundled("app_initial")
    }

    func list(appId: String, entity: String, query: JSONValue?) async throws -> [JSONValue] {
        tables[entity] ?? []
    }

    func create(appId: String, entity: String, data: JSONValue) async throws -> JSONValue {
        var row = data.objectValue ?? [:]
        row["id"] = .string(UUID().uuidString)
        row["createdAt"] = .string(ISO8601DateFormatter().string(from: .now))
        let value = JSONValue.object(row)
        tables[entity, default: []].append(value)
        return value
    }

    func update(appId: String, entity: String, id: String, data: JSONValue) async throws -> JSONValue {
        var rows = tables[entity] ?? []
        guard let i = rows.firstIndex(where: { $0[path: "id"]?.stringValue == id }) else {
            throw APIError.http(status: 404, body: "\(entity)/\(id) not found")
        }
        var row = rows[i].objectValue ?? [:]
        for (k, v) in data.objectValue ?? [:] { row[k] = v }
        rows[i] = .object(row)
        tables[entity] = rows
        return rows[i]
    }

    func delete(appId: String, entity: String, id: String) async throws {
        tables[entity]?.removeAll { $0[path: "id"]?.stringValue == id }
    }
}

/// 発話内容に関わらず、バンドルされたサンプル DSL を返す ai モック。
struct MockAIClient: AIClient {
    var sampleName = "app_kakeibo"
    var delay: Duration = .seconds(1.2)

    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> AppDocument {
        try await Task.sleep(for: delay)
        var doc = try AppDocument.bundled(sampleName)
        doc.version = currentVersion + 1
        return doc
    }
}

struct NoopDocumentSync: DocumentSync {
    func updates(appId: String, after version: Int) -> AsyncStream<AppDocument> {
        AsyncStream { $0.finish() }
    }
}

struct NoAuthTokenProvider: TokenProvider {
    func idToken() async throws -> String? { nil }
}

/// 固定文言を返す文字起こしモック。WhisperKit 導入までのプレースホルダ。
@MainActor
final class MockTranscriber: SpeechTranscriber {
    var isReady: Bool { true }
    var cannedText = "今月の支出を記録できる家計簿アプリが欲しい"

    func prepare() async throws {}
    func startRecording() async throws {}

    func stopAndTranscribe() async throws -> String {
        try await Task.sleep(for: .milliseconds(800))
        return cannedText
    }
}
