import MetamonDSL

/// フォロー(#270)。持たないホスト(モック・プレビュー)では、フォロー中の一覧は空・フォローのボタンは出さない。
@MainActor
public protocol FollowsHost: AnyObject {
    /// このアプリでフォローしている人の uid。nil = まだ読めていない(読み込みはホストが始める)。
    func followedAuthors() -> Set<String>?
    /// 投稿を書いた人をフォローしているか。nil = ボタンを出さない(自分の投稿・作者の無い投稿・読めていない・サインインしていない)。
    func followState(entity: String, recordId: String) -> Bool?
}

@MainActor
public extension DSLRuntime {
    var follows: (any FollowsHost)? { self as? any FollowsHost }

    /// `{"myFollows": true}` をフォローしている uid に置き換える(使っていなければ一覧を読みに行かない)。
    func resolvingFollows(_ filters: [Filter]) -> [Filter] {
        guard filters.contains(where: \.isMyFollows) else { return filters }
        let uids = follows?.followedAuthors()
        return filters.map { $0.resolvingFollows(uids) }
    }

    /// follow / unfollow のボタンを出すか。今の状態に合う方だけ(フォロー済みなら unfollow、まだなら follow)。
    func showsFollowButton(_ action: Action?, recordId: String?) -> Bool {
        guard let action, action.type.isFollow else { return true }
        guard let recordId, let entity = action.entity, let following = follows?.followState(entity: entity, recordId: recordId) else { return false }
        return following == (action.type == .unfollow)
    }
}
