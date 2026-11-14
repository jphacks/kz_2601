import Foundation

/// フォローの API(docs/backend/openapi-appconfig.yaml `/apps/{appId}/follows`・#270)。
public struct HTTPFollowsClient: FollowsClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    private struct ListResponse: Decodable { var uids: [String]? }
    private struct SetResponse: Decodable { var following: Bool }

    public func list(appId: String) async throws -> [String] {
        let data = try await HTTP.send("GET", baseURL: baseURL, path: "v1/apps/\(Self.escape(appId))/follows", body: nil, token: token, timeout: 20)
        return try JSONDecoder().decode(ListResponse.self, from: data).uids ?? []
    }

    public func setFollow(appId: String, uid: String, on: Bool) async throws -> Bool {
        let data = try await HTTP.send(on ? "PUT" : "DELETE", baseURL: baseURL,
                                       path: "v1/apps/\(Self.escape(appId))/follows/\(Self.escape(uid))", body: nil, token: token, timeout: 20)
        return try JSONDecoder().decode(SetResponse.self, from: data).following
    }

    static func escape(_ s: String) -> String {
        s.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-_."))) ?? s
    }
}
