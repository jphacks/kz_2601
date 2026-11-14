#if !APP_STORE_REVIEW // 審査用ビルドは通話(#447)を外す
import Foundation

/// 通話の API(docs/backend/openapi-appconfig.yaml `/apps/{appId}/calls/token`・#447)。
public struct HTTPCallClient: CallClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    public func join(appId: String, request: CallJoinRequest) async throws -> CallJoin {
        let data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/apps/\(HTTPFollowsClient.escape(appId))/calls/token",
                                       body: request, token: token, timeout: 20)
        return try Self.decoder.decode(CallJoin.self, from: data)
    }

    static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .iso8601 // backend は秒までの RFC 3339
        return d
    }()
}
#endif
