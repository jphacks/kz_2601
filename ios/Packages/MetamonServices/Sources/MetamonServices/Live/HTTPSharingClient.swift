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
        _ = try await request("DELETE", "v1/shares/\(token)")
    }

    public func importShare(token: String) async throws -> InstalledApp {
        try decode(try await request("POST", "v1/shares/\(token)/import"))
    }

    public func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing {
        try decode(try await request("POST", "v1/apps/\(appId)/publish",
                                     body: PublishBody(description: description, tags: tags, authorName: authorName)))
    }

    public func unpublish(appId: String) async throws {
        _ = try await request("DELETE", "v1/apps/\(appId)/publish")
    }

    public func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing] {
        var c = URLComponents()
        c.queryItems = [URLQueryItem(name: "q", value: query), URLQueryItem(name: "sort", value: sort),
                        URLQueryItem(name: "pageSize", value: "50")]
        let path = "v1/public-apps?" + (c.percentEncodedQuery ?? "")
        return try JSONDecoder().decode(ListResponse.self, from: try await request("GET", path)).items
    }

    public func install(publicAppId: String) async throws -> InstalledApp {
        try decode(try await request("POST", "v1/public-apps/\(publicAppId)/install"))
    }

    private func decode<T: Decodable>(_ data: Data) throws -> T {
        try JSONDecoder().decode(T.self, from: data)
    }

    private func request(_ method: String, _ path: String, body: (any Encodable)? = nil) async throws -> Data {
        try await HTTP.send(method, baseURL: baseURL, path: path, body: body, token: token, timeout: 20)
    }
}
#endif
