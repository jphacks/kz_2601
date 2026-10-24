import Foundation
import MetamonDSL

/// appconfig の REST 実装(docs/backend/openapi-appconfig.yaml)。
struct HTTPAppConfigClient: AppConfigClient {
    let baseURL: URL
    let token: any TokenProvider

    private struct ListResponse: Decodable { var items: [Record] }
    private struct RecordBody: Encodable { var data: [String: JSONValue] }

    func fetchDocument(appId: String) async throws -> AppDocument {
        try AppDocument.decode(try await request("GET", "v1/apps/\(appId)/dsl"))
    }

    func list(appId: String, entity: String) async throws -> [Record] {
        try JSONDecoder().decode(ListResponse.self, from: try await request("GET", "v1/apps/\(appId)/entities/\(entity)/records?pageSize=200")).items
    }

    func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        try JSONDecoder().decode(Record.self,
                                 from: try await request("POST", "v1/apps/\(appId)/entities/\(entity)/records", body: RecordBody(data: data)))
    }

    func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        try JSONDecoder().decode(Record.self,
                                 from: try await request("PATCH", "v1/apps/\(appId)/entities/\(entity)/records/\(id)", body: RecordBody(data: data)))
    }

    func delete(appId: String, entity: String, id: String) async throws {
        _ = try await request("DELETE", "v1/apps/\(appId)/entities/\(entity)/records/\(id)")
    }

    private func request(_ method: String, _ path: String, body: (any Encodable)? = nil) async throws -> Data {
        try await HTTP.send(method, baseURL: baseURL, path: path, body: body, token: token)
    }
}

/// ai サービス: POST /v1/generate → 適用後の DSL 封筒({appId, version, dsl, ...})
struct HTTPAIClient: AIClient {
    let baseURL: URL
    let token: any TokenProvider

    private struct GenerateBody: Encodable {
        var appId: String
        var baseVersion: Int
        var transcript: String
    }

    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> AppDocument {
        let body = GenerateBody(appId: appId, baseVersion: currentVersion, transcript: transcript)
        return try AppDocument.decode(try await HTTP.send("POST", baseURL: baseURL, path: "v1/generate", body: body, token: token))
    }
}

/// ポーリング方式のホットリロード(v1)。version が進んだ時だけ流す。
struct PollingDocumentSync: DocumentSync {
    let client: any AppConfigClient
    var interval: Duration = .seconds(2)

    func updates(appId: String, after version: Int) -> AsyncStream<AppDocument> {
        AsyncStream { continuation in
            let task = Task {
                var last = version
                while !Task.isCancelled {
                    try? await Task.sleep(for: interval)
                    if let doc = try? await client.fetchDocument(appId: appId), doc.version > last {
                        last = doc.version
                        continuation.yield(doc)
                    }
                }
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }
}

enum HTTP {
    static func send(_ method: String, baseURL: URL, path: String, body: (any Encodable)?, token: any TokenProvider) async throws -> Data {
        guard let url = URL(string: path, relativeTo: baseURL) else { throw APIError.invalidURL }
        var req = URLRequest(url: url)
        req.httpMethod = method
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let t = try await token.idToken() {
            req.setValue("Bearer \(t)", forHTTPHeaderField: "Authorization")
        }
        if let body {
            req.httpBody = try JSONEncoder().encode(body)
        }
        let (data, resp) = try await URLSession.shared.data(for: req)
        let status = (resp as? HTTPURLResponse)?.statusCode ?? -1
        guard (200..<300).contains(status) else {
            throw APIError.http(status: status, body: String(data: data, encoding: .utf8) ?? "")
        }
        return data
    }
}
