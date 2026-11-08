// 審査用ビルド(APP_STORE_REVIEW)は aiChat を持たない(AiChatLeaf と同じ)。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// `roleField` の無い aiChat(stg で作られた古い DSL)の 1 往復を対で保存する。役は並び順で読むので、
/// 返事の保存に失敗したら自分の発言も消し、後でどちらかがサーバーに断られたらもう片方も消す(交互を崩さない)。
extension AppRuntime: AIChatPairSaving {
    struct AIChatPartner: Equatable {
        let appId: String
        let entity: String
        let partner: String
    }

    func saveAIChatPair(entity: String, user: [String: JSONValue], assistant: [String: JSONValue]) async -> Bool {
        await saveAIChatPairIds(entity: entity, user: user, assistant: assistant) != nil
    }

    /// 保存した 2 件の id(仮 id)。どちらかを保存できなければ何も残さず nil。
    func saveAIChatPairIds(entity: String, user: [String: JSONValue],
                           assistant: [String: JSONValue]) async -> (user: String, assistant: String)? {
        let appId = document.appId
        guard let userId = await createRecordId(entity: entity, data: user) else { return nil }
        try? await Task.sleep(for: .milliseconds(5)) // 作成時刻で並べるので、自分 → AI の順を同じ時刻にしない
        guard appId == document.appId, let aiId = await createRecordId(entity: entity, data: assistant) else {
            await removeAIChatTurn(appId: appId, entity: entity, recordId: userId)
            return nil
        }
        aiChatPartners[userId] = AIChatPartner(appId: appId, entity: entity, partner: aiId)
        aiChatPartners[aiId] = AIChatPartner(appId: appId, entity: entity, partner: userId)
        return (userId, aiId)
    }

    /// 対の片方が断られた(ローカル行はキューが消した)なら、もう片方も消す。
    func revertAIChatPartner(for event: SyncEvent) async {
        guard let refused = Self.refusedWrite(event), let id = refused.recordId,
              let pair = aiChatPartners[id], pair.appId == refused.appId, pair.entity == refused.entity else { return }
        aiChatPartners[id] = nil
        aiChatPartners[pair.partner] = nil
        await removeAIChatTurn(appId: pair.appId, entity: pair.entity, recordId: pair.partner)
    }

    private func removeAIChatTurn(appId: String, entity: String, recordId: String) async {
        _ = await recordSync.delete(appId: appId, entity: entity, recordId: recordId)
        await reloadLocal(entity, appId: appId)
    }
}
#endif
