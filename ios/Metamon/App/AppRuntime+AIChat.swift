// 審査用ビルド(APP_STORE_REVIEW)は aiChat を持たない(AiChatLeaf と同じ)。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// `roleField` の無い aiChat(stg で作られた古い DSL)の 1 往復を対で保存する。役は並び順で読むので、
/// 返事の保存に失敗したら自分の発言も消し、後でどちらかがサーバーに断られたらもう片方も消す(交互を崩さない)。
/// 対の作り方・覚え方は Mac と共有(`AIChatPairLedger`)。
extension AppRuntime: AIChatPairSaving {
    /// 仮 id → 相方(`aiChatPairs` の控え)
    var aiChatPartners: [String: AIChatPairLedger.Partner] { aiChatPairs.partners }

    func saveAIChatPair(entity: String, user: [String: JSONValue], assistant: [String: JSONValue]) async -> Bool {
        await saveAIChatPairIds(entity: entity, user: user, assistant: assistant) != nil
    }

    /// 保存した 2 件の id(仮 id)。どちらかを保存できなければ何も残さず nil。
    func saveAIChatPairIds(entity: String, user: [String: JSONValue],
                           assistant: [String: JSONValue]) async -> (user: String, assistant: String)? {
        let appId = document.appId
        return await aiChatPairs.save(appId: appId, entity: entity, user: user, assistant: assistant,
                                      isCurrent: { appId == self.document.appId },
                                      create: { await self.createRecordId(entity: entity, data: $0) },
                                      remove: { await self.removeAIChatTurn(appId: appId, entity: entity, recordId: $0) })
    }

    /// 対の片方が断られた(ローカル行はキューが消した)なら、もう片方も消す。
    func revertAIChatPartner(for event: SyncEvent) async {
        guard let refused = event.refusedWrite, let id = refused.recordId,
              let partner = aiChatPairs.takePartner(appId: refused.appId, entity: refused.entity, recordId: id) else { return }
        await removeAIChatTurn(appId: refused.appId, entity: refused.entity, recordId: partner)
    }

    private func removeAIChatTurn(appId: String, entity: String, recordId: String) async {
        _ = await recordSync.delete(appId: appId, entity: entity, recordId: recordId)
        await reloadLocal(entity, appId: appId)
    }
}
#endif
