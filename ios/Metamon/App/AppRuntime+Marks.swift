import Foundation
import MetamonDSL
import MetamonServices

/// 共有レコードの印(いいね・保存・既読 #255)。1 人 1 回のトグルで、件数と自分の状態はサーバーが持つ。
extension AppRuntime {
    /// ローカル DB に楽観的な値を入れてから送り、サーバーの `{count, mine}` で確定する。付けられなければ元に戻して理由を出し nil。
    func toggleMark(entity: String, recordId: String, mark: String) async -> MarkState? {
        guard let ent = dsl.entity(entity), ent.isShared, ent.mark(mark) != nil else { return nil }
        let appId = document.appId
        // 押した一覧は ListLeaf が先に塗りを変える。ほかの画面(件数の表・「保存した」タブ)は確定後に差し替える
        do {
            let state = try await recordSync.toggleMark(appId: appId, entity: entity, recordId: recordId, mark: mark)
            if document.appId == appId { await reloadCached(entity) }
            return state
        } catch {
            if document.appId == appId { await reloadCached(entity) }
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
