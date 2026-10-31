// 審査用ビルド(APP_STORE_REVIEW・#245)はシェアを外しているので、通報・ブロックも外す。
#if !APP_STORE_REVIEW
import Foundation

/// 通報の理由(`POST /v1/reports` の reason・#321)。
public enum ReportReason: String, CaseIterable, Sendable, Identifiable {
    case spam, sexual, violence, harassment, illegal, privacy, other
    public var id: String { rawValue }
}

/// 通報する対象(#321)。
public enum ReportTarget: Sendable, Equatable {
    /// 「探す」の掲載
    case listing(id: String)
    /// 共有アプリの他人の投稿
    case record(appId: String, entityId: String, recordId: String)
}

/// ブロック中のユーザー(`GET /v1/me/blocks` の items)。
public struct BlockedUser: Codable, Sendable, Equatable, Identifiable {
    public var uid: String
    public var name: String
    public var id: String { uid }

    public init(uid: String, name: String) {
        self.uid = uid
        self.name = name
    }
}

/// 通報・ブロック(appconfig の `moderation` タグ・#321)。`HTTPSharingClient` が持つ。
public protocol ModerationClient: Sendable {
    /// 通報する。同じ対象を通報し直しても成功(サーバーは 1 件にまとめる)。
    func report(_ target: ReportTarget, reason: ReportReason) async throws
    func blockedUsers() async throws -> [BlockedUser]
    /// 投稿の作者をブロックする。
    func block(uid: String, name: String?) async throws -> BlockedUser
    /// 掲載者をブロックする(uid はサーバーが引く)。
    func block(listingId: String) async throws -> BlockedUser
    func unblock(uid: String) async throws
}
#endif
