// 審査用ビルド(APP_STORE_REVIEW・#245)ではシェアごと外す。
#if !APP_STORE_REVIEW
import Foundation

/// `GET /v1/shares/{token}` のプレビュー(import の前に見せる)。
/// `sharedData` = 共有の記録(scope: shared / public)を持つアプリ。参加の確認を出すかの判断に使う。
public struct ShareLinkPreview: Decodable, Sendable, Equatable {
    public var token: String
    public var name: String
    public var description: String
    public var mine: Bool
    public var sharedData: Bool

    enum CodingKeys: String, CodingKey { case token, name, description, mine, sharedData }

    public init(token: String, name: String, description: String = "", mine: Bool = false, sharedData: Bool = false) {
        self.token = token
        self.name = name
        self.description = description
        self.mine = mine
        self.sharedData = sharedData
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        token = try c.decodeIfPresent(String.self, forKey: .token) ?? ""
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        description = try c.decodeIfPresent(String.self, forKey: .description) ?? ""
        mine = try c.decodeIfPresent(Bool.self, forKey: .mine) ?? false
        sharedData = try c.decodeIfPresent(Bool.self, forKey: .sharedData) ?? false
    }
}

/// 参加者 1 人(`GET /v1/apps/{appId}/members` の items・#254)。
public struct SharedMember: Decodable, Sendable, Equatable, Identifiable, Hashable {
    public enum Role: String, Decodable, Sendable { case owner, member }

    public var uid: String
    /// import 時の表示名。無ければ空(作成者は常に空)
    public var displayName: String
    public var role: Role
    /// 呼び出したユーザー本人
    public var you: Bool

    public var id: String { uid }

    enum CodingKeys: String, CodingKey { case uid, displayName, role, you }

    public init(uid: String, displayName: String = "", role: Role = .member, you: Bool = false) {
        self.uid = uid
        self.displayName = displayName
        self.role = role
        self.you = you
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        uid = try c.decode(String.self, forKey: .uid)
        displayName = try c.decodeIfPresent(String.self, forKey: .displayName) ?? ""
        role = (try? c.decode(Role.self, forKey: .role)) ?? .member
        you = try c.decodeIfPresent(Bool.self, forKey: .you) ?? false
    }
}

/// 参加者一覧と、呼び出したユーザーの立場。
public struct SharedMemberList: Decodable, Sendable, Equatable {
    public var items: [SharedMember]
    /// 自分が作成者(各行に「外す」)か参加者(「抜ける」)か
    public var role: SharedMember.Role

    public init(items: [SharedMember], role: SharedMember.Role) {
        self.items = items
        self.role = role
    }
}

/// scope:shared の参加・退出・一覧(appconfig の members API・#254)。`SharingClient` の実装が併せて持つ。
public protocol MembershipClient: Sendable {
    func previewShare(token: String) async throws -> ShareLinkPreview
    /// 参加つきの import。`displayName` は参加者一覧に出す名前(空なら送らない)
    func importShare(token: String, displayName: String) async throws -> InstalledApp
    /// アプリに scope:shared の entity があるか(参加者一覧を出すか)
    func hasMembersOnlyData(appId: String) async throws -> Bool
    func members(appId: String) async throws -> SharedMemberList
    /// 作成者が他の人を外す
    func removeMember(appId: String, uid: String) async throws
    /// 自分が抜ける(冪等)
    func leave(appId: String) async throws
}
#endif
