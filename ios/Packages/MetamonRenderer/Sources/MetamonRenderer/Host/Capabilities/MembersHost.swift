import MetamonDSL

/// 共有データの参加者 1 人(#272・type member の選択肢と表示名)。
public struct AppMember: Sendable, Equatable, Identifiable {
    public var uid: String
    /// 参加した時の表示名。空なら「参加者」と出す
    public var displayName: String
    public var isAdmin: Bool
    public var id: String { uid }

    public init(uid: String, displayName: String, isAdmin: Bool = false) {
        self.uid = uid
        self.displayName = displayName
        self.isAdmin = isAdmin
    }
}

/// 参加者と管理者(#272)。持たないホスト(プレビュー・テスト・審査用の版)では「分からない」扱い:
/// member の入力は選べず、visibleTo のボタンは出し、削除の可否は作った人で決める(どれもサーバーが最後に判定する)。
@MainActor
public protocol MembersHost: AnyObject {
    /// 表示中のアプリの参加者(作成者を含む)。読む前・参加していない時は空。
    var appMembers: [AppMember] { get }
    /// 自分が管理者か。分からない間は nil。
    var isAppAdmin: Bool? { get }
}

@MainActor
public extension DSLRuntime {
    var membersHost: (any MembersHost)? { self as? any MembersHost }

    /// 自分が管理者か(分からなければ nil)。
    var viewerIsAdmin: Bool? { membersHost?.isAppAdmin }

    /// type member の値(uid)の表示名。未選択は空、参加者の一覧に無い(抜けた・読む前)は「参加者」。
    func memberName(_ value: JSONValue) -> String {
        guard let uid = value.stringValue, !uid.isEmpty else { return "" }
        guard let m = membersHost?.appMembers.first(where: { $0.uid == uid }) else { return L("参加者") }
        return m.displayName.isEmpty ? L("参加者") : m.displayName
    }
}
