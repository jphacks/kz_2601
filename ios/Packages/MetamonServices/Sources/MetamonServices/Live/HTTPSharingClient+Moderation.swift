// 審査用ビルド(APP_STORE_REVIEW・#245)ではシェアごと外す。
#if !APP_STORE_REVIEW
import Foundation

/// 通報・ブロックの API(docs/backend/openapi-appconfig.yaml `moderation` タグ・#321)。
extension HTTPSharingClient: ModerationClient {
    private struct ReportBody: Encodable {
        var kind: String
        var id: String?
        var appId: String?
        var entityId: String?
        var recordId: String?
        var reason: String
    }
    private struct BlockBody: Encodable {
        var uid: String?
        var name: String?
        var listingId: String?
    }
    private struct BlockList: Decodable { var items: [BlockedUser] }

    public func report(_ target: ReportTarget, reason: ReportReason) async throws {
        let body: ReportBody = switch target {
        case .listing(let id): ReportBody(kind: "listing", id: id, reason: reason.rawValue)
        case .record(let appId, let entityId, let recordId):
            ReportBody(kind: "record", appId: appId, entityId: entityId, recordId: recordId, reason: reason.rawValue)
        }
        _ = try await moderationSend("POST", "v1/reports", body: body)
    }

    public func blockedUsers() async throws -> [BlockedUser] {
        try JSONDecoder().decode(BlockList.self, from: try await moderationSend("GET", "v1/me/blocks")).items
    }

    public func block(uid: String, name: String?) async throws -> BlockedUser {
        try JSONDecoder().decode(BlockedUser.self, from: try await moderationSend("POST", "v1/me/blocks", body: BlockBody(uid: uid, name: name)))
    }

    public func block(listingId: String) async throws -> BlockedUser {
        try JSONDecoder().decode(BlockedUser.self, from: try await moderationSend("POST", "v1/me/blocks", body: BlockBody(listingId: listingId)))
    }

    public func unblock(uid: String) async throws {
        let escaped = uid.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-_."))) ?? uid
        _ = try await moderationSend("DELETE", "v1/me/blocks/\(escaped)")
    }

    private func moderationSend(_ method: String, _ path: String, body: (any Encodable)? = nil) async throws -> Data {
        try await HTTP.send(method, baseURL: baseURL, path: path, body: body, token: token, timeout: 20)
    }
}
#endif
