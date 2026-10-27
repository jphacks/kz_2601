// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation

/// リリース申請(#76)の 1 件。`status` は管理者が Firestore コンソールで書き換える(received → reviewing → planned / declined)。
public struct ReleaseRequest: Codable, Sendable, Equatable, Identifiable {
    public var id: String
    public var appId: String
    public var name: String
    public var dslVersion: Int
    public var purpose: String
    public var contact: String
    public var status: String

    public init(id: String, appId: String, name: String = "", dslVersion: Int = 0,
                purpose: String = "", contact: String = "", status: String = ReleaseStatus.received.rawValue) {
        self.id = id
        self.appId = appId
        self.name = name
        self.dslVersion = dslVersion
        self.purpose = purpose
        self.contact = contact
        self.status = status
    }

    /// 未知の値(コンソールの打ち間違い等)は受付扱い。
    public var kind: ReleaseStatus { ReleaseStatus(rawValue: status) ?? .received }

    /// appId → いま見せる申請(申請中があればそれ、無ければ最新)。`items` は新しい順。
    public static func latestByApp(_ items: [ReleaseRequest]) -> [String: ReleaseRequest] {
        var out: [String: ReleaseRequest] = [:]
        for r in items {
            guard let cur = out[r.appId] else { out[r.appId] = r; continue }
            if cur.kind == .declined && r.kind != .declined { out[r.appId] = r }
        }
        return out
    }
}

public enum ReleaseStatus: String, Sendable, CaseIterable {
    case received, reviewing, planned, declined
}

/// appconfig の release-requests API(docs/backend/openapi-appconfig.yaml・#76)。
public protocol ReleaseRequestClient: Sendable {
    /// 申請する。同じアプリの申請中が既にあればそれが返る(冪等)。
    func request(appId: String, purpose: String, contact: String?) async throws -> ReleaseRequest
    /// 自分の申請(全アプリ・新しい順)
    func myRequests() async throws -> [ReleaseRequest]
}
#endif
