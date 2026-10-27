// 審査用ビルド(APP_STORE_REVIEW・#245)ではシェアごと外す。
#if !APP_STORE_REVIEW
import Foundation

/// members API(docs/backend/openapi-appconfig.yaml `members` タグ・#254)。
extension HTTPSharingClient: MembershipClient {
    private struct ImportBody: Encodable { var displayName: String }
    private struct DSLScopes: Decodable {
        struct Body: Decodable { var entities: [Entity]? }
        struct Entity: Decodable { var scope: String? }
        var dsl: Body
    }

    public func previewShare(token: String) async throws -> ShareLinkPreview {
        try JSONDecoder().decode(ShareLinkPreview.self, from: try await send("GET", "v1/shares/\(token)"))
    }

    public func importShare(token: String, displayName: String) async throws -> InstalledApp {
        let name = displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        let body: ImportBody? = name.isEmpty ? nil : ImportBody(displayName: String(name.prefix(64)))
        return try JSONDecoder().decode(InstalledApp.self, from: try await send("POST", "v1/shares/\(token)/import", body: body))
    }

    public func hasMembersOnlyData(appId: String) async throws -> Bool {
        let doc = try JSONDecoder().decode(DSLScopes.self, from: try await send("GET", "v1/apps/\(appId)/dsl"))
        return doc.dsl.entities?.contains { $0.scope == "shared" } ?? false
    }

    public func members(appId: String) async throws -> SharedMemberList {
        try JSONDecoder().decode(SharedMemberList.self, from: try await send("GET", "v1/apps/\(appId)/members"))
    }

    public func removeMember(appId: String, uid: String) async throws {
        let escaped = uid.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-_."))) ?? uid
        _ = try await send("DELETE", "v1/apps/\(appId)/members/\(escaped)")
    }

    public func leave(appId: String) async throws {
        _ = try await send("DELETE", "v1/apps/\(appId)/members/me")
    }

    private func send(_ method: String, _ path: String, body: (any Encodable)? = nil) async throws -> Data {
        try await HTTP.send(method, baseURL: baseURL, path: path, body: body, token: token, timeout: 20)
    }
}
#endif
