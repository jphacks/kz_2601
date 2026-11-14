import Foundation
import MetamonDSL

/// 取り消せる削除の 1 件(M11)。`deleteUndoably` が返し、`undoDelete` に渡す。
public struct DeletionTicket: Hashable, Sendable, Identifiable {
    public let id: UUID
    public let appId: String
    public let entity: String
    public let recordId: String
}

/// 一覧の「削除 → 元に戻す」(M11)。
///
/// 削除の送信を `grace` だけ遅らせ、その間は一覧から隠すだけにする(ローカル DB にも送信の列にも触れない)。
/// 元に戻したら隠すのをやめるだけなので、サーバーの記録の id・作成日時・作者は変わらない
/// (送った後に作り直すと id と作成日時が変わり、共有の記録では作者の印も付け直しになる)。
/// 待ちはメモリだけに持つ。背面に回る・サインアウトの送り切りでは待たずに送る。待ちの間にアプリが落ちたら削除されずに残る(消えすぎない側)。
/// サインアウト・アカウント削除の `wipe` は、積む途中の削除を待ってから列を空にし、wipe を跨いだ削除は積まない
/// (前の人の削除が次の人の列に残り、次の人の資格で送られることがないように)。
extension RecordSyncEngine {
    /// 一覧から隠し、`grace` 後に `delete` と同じ送信をする。動画・録音のファイルも送る時に消す(戻せるように)。
    public func deleteUndoably(appId: String, entity: String, recordId: String,
                               grace: Duration) async -> (records: [Record], ticket: DeletionTicket) {
        let ticket = DeletionTicket(id: UUID(), appId: appId, entity: entity, recordId: recordId)
        let timer = Task { [weak self] in
            try? await Task.sleep(for: grace)
            guard !Task.isCancelled else { return }
            await self?.commitDeletion(ticket.id)
        }
        undoable[ticket.id] = (ticket, timer)
        return (await shown(appId: appId, entity: entity), ticket)
    }

    /// 取り消す。送信前なら隠すのをやめて一覧を返す。もう送った(時間切れ・背面)なら nil。
    public func undoDelete(_ ticket: DeletionTicket) async -> [Record]? {
        guard let pending = undoable.removeValue(forKey: ticket.id) else { return nil }
        pending.timer.cancel()
        return await shown(appId: ticket.appId, entity: ticket.entity)
    }

    /// 取り消しを待っている削除。
    public var pendingDeletions: [DeletionTicket] { undoable.values.map(\.ticket) }

    /// 待たずに全部送る(背面に回った・サインアウト前の送り切り)。
    public func commitPendingDeletions() async {
        for id in Array(undoable.keys) { await commitDeletion(id) }
        await settleCommitting() // 時間切れで並行して積んでいる分も列に入りきるまで待つ
        await refreshStatus() // 直後の未送信件数(ログアウトの確認)に入るように
    }

    func commitDeletion(_ id: UUID) async {
        guard let pending = undoable.removeValue(forKey: id) else { return }
        pending.timer.cancel()
        let t = pending.ticket
        let generation = wipeGeneration
        let work = Task { [weak self] in
            guard let self else { return }
            let current = await self.currentRecordId(t)
            guard self.wipeGeneration == generation else { return } // 待つ間に wipe された: 次の人の列に積まない
            _ = await self.delete(appId: t.appId, entity: t.entity, recordId: current)
        }
        committing[id] = work
        await work.value
        committing[id] = nil
    }

    /// 列に積む途中の削除が終わるまで待つ。
    func settleCommitting() async {
        while let work = committing.values.first {
            await work.value
            if let id = committing.first(where: { $0.value == work })?.key { committing[id] = nil }
        }
    }

    /// 送らずに捨てる(アプリ削除・アカウント削除。ローカルもサーバーも別の経路で消す)。
    func dropPendingDeletions(where match: (DeletionTicket) -> Bool) {
        for (id, pending) in undoable where match(pending.ticket) {
            pending.timer.cancel()
            undoable[id] = nil
        }
    }

    /// 表示用の一覧: ローカル DB の内容から、取り消し待ちの削除を除く。
    func shown(appId: String, entity: String) async -> [Record] {
        opened(await sealedShown(appId: appId, entity: entity))
    }

    /// `shown` の復号する前(暗号文のまま)。
    func sealedShown(appId: String, entity: String) async -> [Record] {
        let hidden = undoable.values.map(\.ticket).filter { $0.appId == appId && $0.entity == entity }
        guard !hidden.isEmpty else { return await store.records(appId: appId, entity: entity) }
        return await store.rows(appId: appId, entity: entity)
            .filter { row in row.pendingOp != .delete && !hidden.contains { Self.matches($0, row) } }
            .map(\.record)
    }

    /// 待ちの間に未送信の作成が送られると、仮 id(`local-<clientId>`)がサーバー id に替わる。どちらでも同じ行とみなす。
    static func matches(_ ticket: DeletionTicket, _ row: LocalRecord) -> Bool {
        row.isKnown(as: ticket.recordId)
    }

    private func currentRecordId(_ t: DeletionTicket) async -> String {
        await store.current(appId: t.appId, entity: t.entity, recordId: t.recordId)?.recordId ?? t.recordId
    }
}
