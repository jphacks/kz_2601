// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation

/// appconfig の share / public-apps API(docs/backend/openapi-appconfig.yaml・#49 #50)。
public struct HTTPSharingClient: SharingClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    private struct PublishBody: Encodable {
        var description: String
        var tags: [String]
        var authorName: String?
    }
    private struct ListResponse: Decodable { var items: [PublicAppListing] }

    public func share(appId: String) async throws -> ShareInfo {
        try decode(try await request("POST", "v1/apps/\(appId)/share"))
    }

    public func revokeShare(token: String) async throws {
        _ = try await request("DELETE", try Self.sharePath(token))
    }

    public func importShare(token: String) async throws -> InstalledApp {
        try decode(try await request("POST", try Self.sharePath(token) + "/import"))
    }

    public func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing {
        try decode(try await request("POST", "v1/apps/\(appId)/publish",
                                     body: PublishBody(description: description, tags: tags, authorName: authorName)))
    }

    public func unpublish(appId: String) async throws {
        _ = try await request("DELETE", "v1/apps/\(appId)/publish")
    }

    public func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] {
        // POST: 入力された検索語を URL に載せない
        let body = SearchBody(q: query, sort: sort, pageSize: 50)
        return try JSONDecoder().decode(ListResponse.self, from: try await request("POST", "v1/public-apps/search", body: body)).items
    }

    private struct SearchBody: Encodable {
        let q: String
        let sort: String
        let pageSize: Int
    }

    public func install(publicAppId: String) async throws -> InstalledApp {
        try decode(try await request("POST", "v1/public-apps/\(publicAppId)/install"))
    }

    /// `v1/shares/<token>`。形の合わないトークンは送らない(点だけの値は URL の解決で別の道になる)。
    static func sharePath(_ token: String) throws -> String {
        guard ShareURL.isValidToken(token) else { throw APIError.invalidURL }
        return "v1/shares/\(token)"
    }

    private func decode<T: Decodable>(_ data: Data) throws -> T {
        try JSONDecoder().decode(T.self, from: data)
    }

    private func request(_ method: String, _ path: String, body: (any Encodable)? = nil) async throws -> Data {
        try await HTTP.send(method, baseURL: baseURL, path: path, body: body, token: token, timeout: 20)
    }
}
#endif
