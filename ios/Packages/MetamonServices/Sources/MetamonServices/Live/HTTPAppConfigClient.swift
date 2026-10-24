import Foundation
import MetamonDSL

/// appconfig の REST 実装(docs/backend/openapi-appconfig.yaml)。
public struct HTTPAppConfigClient: AppConfigClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    private struct ListResponse: Decodable { var items: [Record] }
    private struct RecordBody: Encodable { var data: [String: JSONValue] }

    public func fetchDocument(appId: String) async throws -> AppDocument {
        try AppDocument.decode(try await request("GET", "v1/apps/\(appId)/dsl"))
    }

    public func list(appId: String, entity: String) async throws -> [Record] {
        try JSONDecoder().decode(ListResponse.self, from: try await request("GET", "v1/apps/\(appId)/entities/\(entity)/records?pageSize=200")).items
    }

    public func create(appId: String, entity: String, data: [String: JSONValue]) async throws -> Record {
        try JSONDecoder().decode(Record.self,
                                 from: try await request("POST", "v1/apps/\(appId)/entities/\(entity)/records", body: RecordBody(data: data)))
    }

    public func update(appId: String, entity: String, id: String, data: [String: JSONValue]) async throws -> Record {
        try JSONDecoder().decode(Record.self,
                                 from: try await request("PATCH", "v1/apps/\(appId)/entities/\(entity)/records/\(id)", body: RecordBody(data: data)))
    }

    public func delete(appId: String, entity: String, id: String) async throws {
        _ = try await request("DELETE", "v1/apps/\(appId)/entities/\(entity)/records/\(id)")
    }

    private func request(_ method: String, _ path: String, body: (any Encodable)? = nil) async throws -> Data {
        try await HTTP.send(method, baseURL: baseURL, path: path, body: body, token: token)
    }
}
