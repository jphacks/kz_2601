import Foundation
import MetamonDSL

/// インメモリの appconfig。サーバー無しで DSL レンダラーを動かすためのもの。
actor InMemoryAppConfigClient: AppConfigClient {
    private var tables: [String: [Record]]

    init(seed: [String: [Record]] = [:]) { tables = seed }

    func fetchDocument(appId: String) async throws -> AppDocument {
        try AppDocument.bundled("initial")
    }

    func list(appId: String, entity: String) async throws -> [Record] {
        tables[entity] ?? []
    }

    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        let now = ISO8601DateFormatter().string(from: .now)
        let record = Record(id: UUID().uuidString, data: data, createdAt: now, updatedAt: now)
        tables[entity, default: []].append(record)
        return record
    }

    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        guard let i = tables[entity]?.firstIndex(where: { $0.id == id }) else {
            throw APIError.http(status: 404, body: "\(entity)/\(id) not found")
        }
        var r = tables[entity]![i]
        for (k, v) in data { r.data[k] = v }
        r.updatedAt = ISO8601DateFormatter().string(from: .now)
        tables[entity]![i] = r
        return r
    }

    func delete(appId: String, entity: String, id: String) async throws {
        tables[entity]?.removeAll { $0.id == id }
    }
}

/// 発話のキーワードで同梱サンプル DSL(docs/dsl/examples)を返す ai モック。
struct MockAIClient: AIClient {
    var delay: Duration = .seconds(1.2)

    static func sampleName(for transcript: String) -> String {
        if transcript.contains("トレ") || transcript.contains("筋") || transcript.contains("運動") { return "training" }
        if transcript.contains("散歩") || transcript.contains("写真") || transcript.contains("場所") { return "walk" }
        if transcript.contains("最初") || transcript.contains("戻") { return "initial" }
        return "kakeibo"
    }

    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> AppDocument {
        try await Task.sleep(for: delay)
        return try AppDocument.bundled(Self.sampleName(for: transcript), version: currentVersion + 1)
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
