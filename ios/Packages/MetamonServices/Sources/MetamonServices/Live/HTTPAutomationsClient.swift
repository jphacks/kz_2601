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

    /// 今の DSL を読んで id から位置を引き、その version を If-Match にして PATCH する(間に変わっていれば 412)。
    public func setRule(_ ruleId: String, enabled: Bool, appId: String) async throws {
        let path = "v1/apps/\(appId)/dsl"
        let current = try AutomationToggle.parse(envelope: try await HTTP.send("GET", baseURL: baseURL, path: path, body: nil, token: token, timeout: 20))
        let ops = try AutomationToggle.ops(ruleId: ruleId, enabled: enabled, serverRules: current.serverRules)
        guard !ops.isEmpty else { return }
        _ = try await HTTP.send("PATCH", baseURL: baseURL, path: path, body: AutomationToggle.Body(patch: ops), token: token, timeout: 20,
                                headers: ["If-Match": HTTPAppConfigClient.etag(current.version)])
    }
}
#endif
