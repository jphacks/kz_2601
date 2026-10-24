import Foundation

/// `DELETE /v1/me`(docs/backend/openapi-appconfig.yaml)。成功は 204。
public struct HTTPAccountClient: AccountClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    public func deleteAccount() async throws {
        // アプリ・シェア・掲載の更新でサーバー側が数秒かかることがある
        _ = try await HTTP.send("DELETE", baseURL: baseURL, path: "v1/me", body: nil, token: token, timeout: 60)
    }
}
