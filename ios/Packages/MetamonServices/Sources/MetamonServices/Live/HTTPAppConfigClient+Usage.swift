import Foundation

/// 使用量(#317・#630)。`GET /v1/me/usage`・`GET /v1/apps/{appId}/usage`。
/// 古いサーバー(まだ無い)・このアプリの写しが無い時は 404 → nil(出さない)。
extension HTTPAppConfigClient: PlanUsageSource {
    public func myUsage() async throws -> MyUsage? {
        guard let data = try await getOrNil("v1/me/usage") else { return nil }
        return try MyUsage.decode(data)
    }

    public func appUsage(appId: String) async throws -> AppUsage? {
        guard let data = try await getOrNil("v1/apps/\(appId)/usage") else { return nil }
        return try JSONDecoder().decode(AppUsage.self, from: data)
    }

    private func getOrNil(_ path: String) async throws -> Data? {
        do {
            return try await HTTP.send("GET", baseURL: baseURL, path: path, body: nil, token: token)
        } catch let error as APIError where error.status == 404 {
            return nil
        }
    }
}
