// 審査用ビルド(APP_STORE_REVIEW)ではバイナリから外す(#347)。
#if !APP_STORE_REVIEW
import Foundation

/// appconfig の automations API(#347)。
public struct HTTPAutomationsClient: AutomationsClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    public func automations(appId: String) async throws -> AppAutomations {
        let data = try await HTTP.send("GET", baseURL: baseURL, path: "v1/apps/\(appId)/automations", body: nil, token: token, timeout: 20)
        return try JSONDecoder().decode(AppAutomations.self, from: data)
    }
}
#endif
