#if DEBUG && !APP_STORE_REVIEW
import Foundation
import MetamonServices

/// UI テスト用の参加者(#254)。`METAMON_MOCK_SHARING=1` と併用する。
/// - `METAMON_MOCK_MEMBERS=owner|member` 自分の立場。未設定なら共有の記録が無いアプリ扱い(参加者の欄を出さない)
/// - `METAMON_MOCK_MEMBERS_FAIL=1` 外す・抜けるを失敗させる(楽観的 UI の巻き戻しを見る)
/// - import のトークン `uitest-inviter-left` / `uitest-members-full` は 403 を返す
enum DebugMockMembers {
    static var env: [String: String] { ProcessInfo.processInfo.environment }
    static var role: SharedMember.Role? { env["METAMON_MOCK_MEMBERS"].flatMap(SharedMember.Role.init(rawValue:)) }
    static var failsWrites: Bool { env["METAMON_MOCK_MEMBERS_FAIL"] == "1" }

    /// 端末内の参加者表(外す・抜けるで減る)
    final class Store: @unchecked Sendable {
        private let lock = NSLock()
        private var items: [SharedMember]?

        func list(role: SharedMember.Role) -> [SharedMember] {
            lock.withLock {
                if let items { return items }
                let initial = [
                    SharedMember(uid: "uid-owner", role: .owner, you: role == .owner),
                    SharedMember(uid: "uid-dad", displayName: "父", you: role == .member),
                    SharedMember(uid: "uid-kid", displayName: "たろう"),
                ]
                items = initial
                return initial
            }
        }

        func remove(_ uid: String) { lock.withLock { items?.removeAll { $0.uid == uid } } }
    }
    static let store = Store()

    static func denied(_ reason: String) -> APIError {
        .server(status: 403, code: "forbidden", message: reason, details: ["reason": .string(reason)])
    }
}

extension DebugSharingClient: MembershipClient {
    func previewShare(token: String) async throws -> ShareLinkPreview {
        ShareLinkPreview(token: token, name: "家族の家計簿", description: "家族みんなで支出を付ける家計簿",
                     sharedData: DebugMockMembers.role != nil)
    }

    func importShare(token: String, displayName: String) async throws -> InstalledApp {
        switch token {
        case "uitest-inviter-left": throw DebugMockMembers.denied("inviter_left")
        case "uitest-members-full": throw DebugMockMembers.denied("members_full")
        default: return InstalledApp(appId: "uitest-imported", name: "家族の家計簿", joined: true, member: true)
        }
    }

    func hasMembersOnlyData(appId: String) async throws -> Bool { DebugMockMembers.role != nil }

    func members(appId: String) async throws -> SharedMemberList {
        guard let role = DebugMockMembers.role else { throw DebugMockMembers.denied("not_member") }
        return SharedMemberList(items: DebugMockMembers.store.list(role: role), role: role)
    }

    func removeMember(appId: String, uid: String) async throws {
        try await Task.sleep(for: .milliseconds(300))
        if DebugMockMembers.failsWrites { throw APIError.server(status: 503, code: "unavailable", message: "mock failure") }
        DebugMockMembers.store.remove(uid)
    }

    func leave(appId: String) async throws {
        try await Task.sleep(for: .milliseconds(300))
        if DebugMockMembers.failsWrites { throw APIError.server(status: 503, code: "unavailable", message: "mock failure") }
        DebugMockMembers.store.remove("uid-dad")
    }
}
#endif
