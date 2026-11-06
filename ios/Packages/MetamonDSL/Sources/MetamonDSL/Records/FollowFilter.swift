import Foundation

/// フォローした人の投稿だけ(#270)。filter `{"field":"$author","op":"eq","value":{"myFollows":true}}`(neq = フォローしていない人)。
/// フォローの一覧はアプリの外(appconfig)にあるので、描画の前にホストが uid の配列に置き換える(`{"me": true}` と同じ流れ)。
public extension Filter {
    /// `{"myFollows": true}` = サインイン中の人がこのアプリでフォローしている人(field `$author` 専用)。
    static let myFollowsValue: JSONValue = .object(["myFollows": .bool(true)])

    var isMyFollows: Bool { value == Self.myFollowsValue }

    /// `{"myFollows": true}` をフォローしている uid の配列に置き換える。uids が nil(まだ読めていない)の時はそのまま = どの行にも一致しない。
    func resolvingFollows(_ uids: Set<String>?) -> Filter {
        guard isMyFollows, let uids else { return self }
        return Filter(field: field, op: op, value: .array(uids.sorted().map(JSONValue.string)))
    }

    /// `resolvingFollows` 後の uid の集合(それ以外の filter は nil)。
    var followedAuthors: Set<String>? {
        guard field == RecordValue.author, case .array(let items)? = value else { return nil }
        return Set(items.compactMap(\.stringValue))
    }
}

public extension AppDSL {
    /// このアプリがフォローを読むか(filter の `{"myFollows": true}` か follow / unfollow のボタン)。読む時だけ一覧を取りに行く。
    var usesFollows: Bool {
        screens.contains { screen in
            screen.toolbar.contains { $0.action.type.isFollow }
                || screen.allComponents.contains { c in
                    c.filters.contains(where: \.isMyFollows) || c.action?.type.isFollow == true || c.onItemTap?.type.isFollow == true
                }
        }
    }
}

public extension ActionType {
    var isFollow: Bool { self == .follow || self == .unfollow }
}
