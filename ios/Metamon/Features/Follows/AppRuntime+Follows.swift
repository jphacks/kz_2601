import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// フォロー(#270)の一覧の置き場。AppRuntime(@Observable)には拡張で保存プロパティを足せないので別に持つ。
@MainActor
final class FollowsUI {
    static let shared = FollowsUI()
    /// "appId|uid" → そのアプリのフォロー一覧
    private var stores: [String: FollowStore] = [:]

    func store(appId: String, uid: String, client: any FollowsClient) -> FollowStore {
        let key = "\(appId)|\(uid)"
        if let s = stores[key] { return s }
        let s = FollowStore(appId: appId, client: client)
        stores[key] = s
        return s
    }

    /// テスト用: 全部忘れる。
    func reset() { stores = [:] }
}

extension AppRuntime: FollowsHost {
    /// このアプリのフォロー一覧。live で、フォローを使う DSL で、サインインしている時だけ。
    var followStore: FollowStore? {
        guard let client = services.follows, let me = currentUserId, !me.isEmpty, dsl.usesFollows else { return nil }
        return FollowsUI.shared.store(appId: document.appId, uid: me, client: client)
    }

    // MARK: FollowsHost

    func followedAuthors() -> Set<String>? {
        guard let store = followStore else { return nil }
        if store.uids == nil { Task { try? await store.load() } } // 読めなければ空のまま(次に開いた時に読み直す)
        return store.uids
    }

    func followState(entity: String, recordId: String) -> Bool? {
        guard let author = records(for: entity).first(where: { $0.id == recordId })?.author, !author.isEmpty, author != currentUserId,
              let uids = followedAuthors() else { return nil }
        return uids.contains(author)
    }

    // MARK: Action follow / unfollow

    /// 今開いている投稿(行から開いた画面・フォームのレコード)を書いた人をフォローする / 外す。楽観的に反映し、失敗したら戻して理由を出す。
    func runFollow(_ action: Action, _ ctx: ActionContext) async {
        guard let store = followStore, let entity = action.entity else {
            toast = L10n.tr("このアプリでは今はフォローを使えません")
            return
        }
        guard let id = ctx.currentRecordId, let author = records(for: entity).first(where: { $0.id == id })?.author, !author.isEmpty, author != currentUserId else {
            toast = L10n.tr("ほかの人の投稿を開いてからフォローしてください")
            return
        }
        let on = action.type == .follow
        do {
            try await store.set(author, on: on)
            toast = on ? L10n.tr("フォローしました") : L10n.tr("フォローを外しました")
        } catch {
            toast = Self.followErrorText(error)
        }
    }

    static func followErrorText(_ error: Error) -> String {
        if case .server(_, _, _, let details)? = error as? APIError, case .string(let r)? = details["reason"] {
            if r == FollowsReason.full { return L10n.tr("フォローできる人数(500 人)を超えています") }
            if r == FollowsReason.notInApp { return L10n.tr("この人はもうこのアプリに参加していません") }
        }
        if FailureKind.classify(error) == .offline { return L10n.tr("オフラインのあいだはフォローできません") }
        return L10n.tr("フォローを変えられませんでした。もう一度お試しください")
    }
}
