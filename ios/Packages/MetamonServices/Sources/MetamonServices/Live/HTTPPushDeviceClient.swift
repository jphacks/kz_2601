#if !APP_STORE_REVIEW // 審査用ビルドはプッシュの通信ごと外す(#269)
import Foundation

/// `PUT / DELETE /v1/me/devices/{installationId}`・`PUT /v1/me/notify-consent/{appId}`(docs/backend/openapi-appconfig.yaml)。
public struct HTTPPushDeviceClient: PushDeviceClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    public func putDevice(installationId: String, registration: PushDeviceRegistration) async throws {
        _ = try await HTTP.send("PUT", baseURL: baseURL, path: "v1/me/devices/\(installationId)", body: registration, token: token)
    }

    public func deleteDevice(installationId: String) async throws {
        _ = try await HTTP.send("DELETE", baseURL: baseURL, path: "v1/me/devices/\(installationId)", body: nil, token: token)
    }

    public func setNotifyConsent(appId: String, allowed: Bool) async throws {
        _ = try await HTTP.send("PUT", baseURL: baseURL, path: "v1/me/notify-consent/\(appId)", body: ["allowed": allowed], token: token)
    }
}
#endif
