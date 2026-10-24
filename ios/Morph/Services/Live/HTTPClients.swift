import Foundation

/// appconfig の REST 実装。パスは docs/ios/dsl-proposal.md の提案値(backend とすり合わせ後に調整)。
struct HTTPAppConfigClient: AppConfigClient {
    let baseURL: URL
    let token: any TokenProvider

    private struct ListResponse: Decodable { var items: [JSONValue] }

    func fetchDocument(appId: String) async throws -> AppDocument {
        try AppDocument.decode(try await request("GET", "v1/apps/\(appId)"))
    }

    func list(appId: String, entity: String, query: JSONValue?) async throws -> [JSONValue] {
        var path = "v1/apps/\(appId)/entities/\(entity)/records"
        if let q = query?.objectValue, !q.isEmpty {
            let items = q.map { "\($0.key)=\($0.value.displayString)" }
            path += "?" + items.joined(separator: "&")
        }
        return try JSONDecoder().decode(ListResponse.self, from: try await request("GET", path)).items
    }

    func create(appId: String, entity: String, data: JSONValue) async throws -> JSONValue {
        try JSONDecoder().decode(JSONValue.self,
                                 from: try await request("POST", "v1/apps/\(appId)/entities/\(entity)/records", body: data))
    }

    func update(appId: String, entity: String, id: String, data: JSONValue) async throws -> JSONValue {
        try JSONDecoder().decode(JSONValue.self,
                                 from: try await request("PATCH", "v1/apps/\(appId)/entities/\(entity)/records/\(id)", body: data))
    }

    func delete(appId: String, entity: String, id: String) async throws {
        _ = try await request("DELETE", "v1/apps/\(appId)/entities/\(entity)/records/\(id)")
    }

    private func request(_ method: String, _ path: String, body: JSONValue? = nil) async throws -> Data {
        try await HTTP.send(method, baseURL: baseURL, path: path, body: body, token: token)
    }
}

/// ai サービス: POST /v1/morph
struct HTTPAIClient: AIClient {
    let baseURL: URL
    let token: any TokenProvider

    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> AppDocument {
        let body: JSONValue = [
            "appId": .string(appId),
            "currentVersion": .number(Double(currentVersion)),
            "transcript": .string(transcript),
        ]
        return try AppDocument.decode(try await HTTP.send("POST", baseURL: baseURL, path: "v1/morph", body: body, token: token))
    }
}

/// ポーリング方式のホットリロード(v0)。version が進んだ時だけ流す。
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
    static func send(_ method: String, baseURL: URL, path: String, body: JSONValue?, token: any TokenProvider) async throws -> Data {
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
