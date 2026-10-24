import Foundation

/// appconfig の release-requests API(#76)。
public struct HTTPReleaseRequestClient: ReleaseRequestClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    private struct Body: Encodable {
        var purpose: String
        var contact: String?
    }
    private struct ListResponse: Decodable { var items: [ReleaseRequest] }

    public func request(appId: String, purpose: String, contact: String?) async throws -> ReleaseRequest {
        let data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/apps/\(appId)/release-requests",
                                       body: Body(purpose: purpose, contact: contact), token: token, timeout: 20)
        return try JSONDecoder().decode(ReleaseRequest.self, from: data)
    }

    public func myRequests() async throws -> [ReleaseRequest] {
        let data = try await HTTP.send("GET", baseURL: baseURL, path: "v1/release-requests", body: nil, token: token, timeout: 20)
        return try JSONDecoder().decode(ListResponse.self, from: data).items
    }
}
