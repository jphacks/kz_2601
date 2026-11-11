import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

// 人とつながる部品(#963)。iOS の `AppRuntime+Chat`・`+Marks`・`+Members`・`+Moderation`・`+Matching` と同じ流れで、
// 問い合わせ先は `MacSocial`(iOS と同じ HTTP のクライアント)。

// MARK: - 投稿(chatThread の送信)・印

extension MacAppRuntime: RecordPosting {
    /// chatThread の送信(#260)。作者(自分の uid)を付けて保存する(ローカル DB → 同期キュー)。
    /// Mac には投稿に出す名前の設定が無いので名前は送らない(backend が「匿名」を付ける)。
    func postRecord(entity: String, data: [String: JSONValue]) async -> Bool {
        guard dsl.entity(entity) != nil else { return false }
        return await createRecord(entity: entity, data: data)
    }

    /// shared / public の entity に作る記録の投稿者(#250)。private は nil
    func postAuthor(for entity: String) -> (uid: String?, name: String?)? {
        guard dsl.entity(entity)?.isShared == true else { return nil }
        return (currentUserId, nil)
    }
}

extension MacAppRuntime: MarksHost {
    /// いいね・保存(#255)。ローカル DB に楽観的な値を入れてから送り、サーバーの `{count, mine}` で確定する。
    /// 付けられなければ元に戻して理由を出し nil(iOS と同じ)。
    func toggleMark(entity: String, recordId: String, mark: String) async -> MarkState? {
        guard let ent = dsl.entity(entity), ent.isShared, ent.mark(mark) != nil else { return nil }
        do {
            let state = try await recordSync.toggleMark(appId: appId, entity: entity, recordId: recordId, mark: mark)
            await reloadLocal(entity)
            return state
        } catch {
            await reloadLocal(entity)
            toast = Self.markErrorText(error)
            return nil
        }
    }

    static func markErrorText(_ error: Error) -> String {
        switch error as? MarkError {
        case .offline: L10n.tr("オフラインのあいだは付けられません")
        case .notSynced: L10n.tr("送信が終わってから付けられます")
        default: L10n.tr("うまく付けられませんでした。もう一度お試しください")
        }
    }
}

#if !APP_STORE_REVIEW

// MARK: - 参加者(#272)

extension MacAppRuntime: MembersHost {
    var appMembers: [AppMember] { social.memberEntries[appId]?.members ?? [] }
    var isAppAdmin: Bool? { social.memberEntries[appId]?.isAdmin }

    /// 参加者を使う DSL か(type member・editableBy・deleteBy・visibleTo)。使わないアプリでは members API を呼ばない(iOS と同じ)
    nonisolated static func usesMembers(_ dsl: AppDSL) -> Bool {
        let fields = dsl.entities.contains { e in
            e.deleteBy != nil || e.fields.contains { $0.type == .member || $0.type == .members || $0.isAdminOnly }
        }
        return fields || dsl.screens.contains { s in s.components.contains(where: hasVisibleTo) }
    }

    private nonisolated static func hasVisibleTo(_ c: Component) -> Bool {
        c.visibleTo != nil || c.children.contains(where: hasVisibleTo)
    }

    /// 参加者と自分の立場を読み直す。参加していなければ空・管理者でない
    func reloadAppMembers() async {
        guard let client = social.members else { return }
        do {
            let list = try await client.members(appId: appId)
            social.memberEntries[appId] = (list.items.map { AppMember(uid: $0.uid, displayName: $0.displayName, isAdmin: $0.isAdmin) },
                                           list.isAdmin)
        } catch {
            if SharedAccessDenial(error) == .notMember { social.memberEntries[appId] = ([], false) }
        }
    }

    /// 作成者・管理者が参加者を外す
    @discardableResult
    func removeMember(uid: String) async -> Bool {
        await membersChange(L10n.tr("参加者を外せませんでした")) { try await $0.removeMember(appId: self.appId, uid: uid) }
    }

    /// 管理者にする / 外す
    @discardableResult
    func setMemberAdmin(uid: String, admin: Bool) async -> Bool {
        await membersChange(L10n.tr("管理者を変えられませんでした")) { try await $0.setAdmin(appId: self.appId, uid: uid, admin: admin) }
    }

    /// 自分が抜ける。抜けたら参加者は空・管理者でない
    @discardableResult
    func leaveApp() async -> Bool {
        guard let client = social.members else { return false }
        do {
            try await client.leave(appId: appId)
            social.memberEntries[appId] = ([], false)
            return true
        } catch {
            toast = L10n.tr("抜けられませんでした。もう一度お試しください")
            return false
        }
    }

    private func membersChange(_ failure: String, _ op: (any MembershipClient) async throws -> Void) async -> Bool {
        guard let client = social.members else { return false }
        do {
            try await op(client)
            await reloadAppMembers()
            return true
        } catch {
            toast = failure
            return false
        }
    }
}

// MARK: - 通報・ブロック(#321)

extension MacAppRuntime: ModerationHost {
    /// 他人の共有レコードだけ(自分の投稿・private の entity・作者の分からない行は出さない)
    func canReport(entity: String, record: Record) -> Bool {
        guard social.moderation != nil, dsl.entity(entity)?.isShared == true,
              let author = record.author, !author.isEmpty, let me = currentUserId else { return false }
        return author != me
    }

    func reportRecord(entity: String, record: Record) {
        social.reporting = moderationRequest(entity: entity, record: record)
    }

    func blockAuthor(entity: String, record: Record) {
        social.blocking = moderationRequest(entity: entity, record: record)
    }

    private func moderationRequest(entity: String, record: Record) -> MacModerationRequest {
        MacModerationRequest(target: .record(appId: appId, entityId: entity, recordId: record.id),
                             authorName: record.authorName ?? L10n.tr("匿名"), authorUID: record.author)
    }

    func submitReport(_ req: MacModerationRequest, reason: ReportReason) async {
        guard let client = social.moderation else { return }
        social.reporting = nil
        do {
            try await client.report(req.target, reason: reason)
            social.notice = MacModerationNotice(
                title: L10n.tr("通報しました"),
                message: L10n.tr("運営が内容を確認し、利用規約に反するものは非公開にします。この人を表示したくない時はブロックもできます。"))
        } catch {
            let message = (error as? APIError)?.status == 429
                ? L10n.tr("今日はこれ以上通報できません。明日またお試しください")
                : L10n.tr("通信できる状態でもう一度お試しください")
            social.notice = MacModerationNotice(title: L10n.tr("通報できませんでした"), message: message)
        }
    }

    /// 作者を先に端末で隠してから送る(失敗したら戻す)
    func confirmBlock(_ req: MacModerationRequest) async {
        guard let client = social.moderation, let uid = req.authorUID, !uid.isEmpty else { return }
        social.blocking = nil
        social.blocked[uid] = req.authorName
        do {
            let user = try await client.block(uid: uid, name: req.authorName)
            social.blocked[user.uid] = user.name
            social.notice = MacModerationNotice(
                title: L10n.tr("ブロックしました"),
                message: L10n.tr("「\(user.name)」さんの投稿と公開したアプリは表示されません。設定から解除できます。"))
        } catch {
            social.blocked[uid] = nil
            social.notice = MacModerationNotice(title: L10n.tr("ブロックできませんでした"),
                                                message: L10n.tr("通信できる状態でもう一度お試しください"))
        }
    }

    func unblock(_ user: BlockedUser) async {
        guard let client = social.moderation else { return }
        social.blocked[user.uid] = nil
        do {
            try await client.unblock(uid: user.uid)
            await load(entities: Set(dsl.entities.filter(\.isShared).map(\.id)), force: true) // サーバーが除いていた投稿を取り直す
        } catch {
            social.blocked[user.uid] = user.name
            toast = L10n.tr("解除できませんでした")
        }
    }

    /// サインイン中の uid のブロック一覧を 1 回読む
    func loadBlockedUsers() async {
        guard let client = social.moderation, let me = currentUserId, social.blockedLoadedFor != me else { return }
        guard let users = try? await client.blockedUsers() else { return }
        social.blocked = Dictionary(users.map { ($0.uid, $0.name) }, uniquingKeysWith: { a, _ in a })
        social.blockedLoadedFor = me
    }

    /// 共有の entity から、ブロック中の人の投稿を除く(`records(for:)`)
    func hidingBlocked(_ rows: [Record], entity: String) -> [Record] {
        let blocked = social.blocked
        guard !blocked.isEmpty, dsl.entity(entity)?.isShared == true else { return rows }
        return rows.filter { $0.author.map { blocked[$0] == nil } ?? true }
    }
}

// MARK: - 仲間探し(#507)

extension MacAppRuntime: MatchingHost {
    /// このアプリの一覧。サーバーにつながっていて、プロフィールの entity があり、サインインしている時だけ
    var matchingStore: MatchingStore? {
        guard let me = currentUserId, !me.isEmpty, dsl.entities.contains(where: \.isMatchProfile) else { return nil }
        return social.matchingStore(appId: appId, uid: me)
    }

    func interestState(entity: String, record: Record) -> InterestState? {
        guard let store = matchingStore, dsl.entity(entity)?.isMatchProfile == true,
              let author = record.author, !author.isEmpty, author != currentUserId,
              social.blocked[author] == nil else { return nil }
        return store.state(of: author)
    }

    func interestBusy(record: Record) -> Bool {
        guard let author = record.author else { return false }
        return matchingStore?.isBusy(author) ?? false
    }

    func sendInterest(entity: String, record: Record) async {
        guard let store = matchingStore, let author = record.author, !author.isEmpty, author != currentUserId else { return }
        guard let mine = records(for: entity).first(where: { $0.author == currentUserId }) else {
            toast = L10n.tr("先に自分のプロフィールを書いてください")
            return
        }
        guard social.ageGate.isConfirmed else {
            social.askingAge = .send(entity: entity, record: record)
            return
        }
        do {
            let result = try await store.send(to: author, recordId: record.id, fromRecordId: mine.id)
            ageRecorded()
            if result.isMatched { await connected() }
        } catch {
            matchingFailed(error, retry: .send(entity: entity, record: record))
        }
    }

    func matchItems(_ box: MatchBox) -> [MatchInterest]? {
        guard let items = matchingStore?.items(box) else { return nil }
        let blocked = social.blocked
        return blocked.isEmpty ? items : items.filter { blocked[$0.uid] == nil }
    }

    func matchLoadFailed(_ box: MatchBox) -> Bool { matchingStore?.failed.contains(box) ?? false }

    func refreshMatches(_ boxes: Set<MatchBox>) async {
        guard let store = matchingStore else { return }
        for box in MatchBox.allCases where boxes.contains(box) {
            do {
                try await store.load(box)
            } catch {
                if (error as? APIError)?.isAgeUnconfirmed == true {
                    askAge(.refresh(boxes))
                    return
                }
            }
        }
    }

    func acceptInterest(_ item: MatchInterest) async {
        guard let store = matchingStore else { return }
        guard social.ageGate.isConfirmed else {
            social.askingAge = .accept(item)
            return
        }
        do {
            try await store.accept(item.uid)
            ageRecorded()
            await connected()
        } catch {
            matchingFailed(error, retry: .accept(item))
        }
    }

    func declineInterest(_ item: MatchInterest, box: MatchBox) async {
        guard let store = matchingStore else { return }
        do {
            if box == .outgoing { try await store.withdraw(item.uid) } else { try await store.decline(item.uid) }
        } catch {
            matchingFailed(error, retry: nil)
        }
    }

    func unmatch(_ item: MatchInterest) async {
        guard let store = matchingStore else { return }
        do {
            try await store.unmatch(item.uid)
            toast = L10n.tr("つながりを解除しました")
            await reloadMatchedRecords()
        } catch {
            matchingFailed(error, retry: nil)
        }
    }

    // MARK: 18 歳以上の確認

    /// 「18 歳以上です」: 端末に覚えてサーバーに送り(年齢そのものは保存しない)、止めていた操作をやり直す
    func confirmAdult(_ pending: MacPendingInterest) async {
        social.askingAge = nil
        social.declinedAge = false
        social.ageGate.confirm()
        if let client = social.matching, let me = currentUserId, !me.isEmpty {
            do {
                try await social.ageGate.declare(uid: me, client: client)
            } catch {
                toast = Self.matchingErrorText(error)
                return
            }
        }
        switch pending {
        case .send(let entity, let record): await sendInterest(entity: entity, record: record)
        case .accept(let item): await acceptInterest(item)
        case .refresh(let boxes): if !boxes.isEmpty { await refreshMatches(boxes) }
        }
    }

    /// 「18 歳未満です」: 何も覚えず、送らない
    func declineAdult() {
        social.askingAge = nil
        social.declinedAge = true
        toast = L10n.tr("18 歳未満の方は「つながりたい」を使えません")
    }

    private func connected() async {
        toast = L10n.tr("つながりました。連絡先が見られます")
        await reloadMatchedRecords()
    }

    private func reloadMatchedRecords() async {
        let ids = Set(dsl.entities.filter(\.isMatchMatched).map(\.id))
        if !ids.isEmpty { await load(entities: ids, force: true) }
    }

    private func matchingFailed(_ error: Error, retry: MacPendingInterest?) {
        if (error as? APIError)?.reason == MatchingReason.ageUnconfirmed {
            if let retry { askAge(retry) } else { social.ageGate.reset() }
            return
        }
        toast = Self.matchingErrorText(error)
    }

    private func askAge(_ pending: MacPendingInterest) {
        social.ageGate.reset()
        if case .refresh(let boxes) = pending {
            if social.declinedAge { return }
            switch social.askingAge {
            case .refresh(let asked)?: social.askingAge = .refresh(asked.union(boxes))
            case nil: social.askingAge = pending
            default: break
            }
            return
        }
        social.askingAge = pending
    }

    private func ageRecorded() {
        guard let me = currentUserId, !me.isEmpty else { return }
        social.ageGate.markSent(uid: me)
    }

    static func matchingErrorText(_ error: Error) -> String {
        let api = error as? APIError
        if api?.reason == MatchingReason.blocked { return L10n.tr("ブロックを解除してから送れます") }
        if api?.field == "fromRecordId" { return L10n.tr("プロフィールの送信が終わってから送れます") }
        if (error as? URLError)?.code == .notConnectedToInternet { return L10n.tr("オフラインのあいだは送れません") }
        switch api?.status {
        case 429: return L10n.tr("今日はこれ以上送れません。明日またお試しください")
        case 403: return L10n.tr("このアプリでは今は仲間探しを使えません")
        case 404: return L10n.tr("相手が見つかりませんでした。一覧を更新してください")
        default: return L10n.tr("うまくいきませんでした。もう一度お試しください")
        }
    }
}
#endif
