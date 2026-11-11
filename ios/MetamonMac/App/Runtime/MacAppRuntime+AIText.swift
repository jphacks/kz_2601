import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// aiButton・aiGenerate・aiChat(#962)。部品は Renderer の iOS と同じもの: 端末内 AI(Foundation Models・macOS 26+)を先に使い、
/// 使えない Mac・長い文・写真・aiChat はここのオンライン版に回す(iOS の `AppRuntime+AIText` と同じ道)。
/// 送る前に同意(#246)を確かめ、無ければ同意の説明を出して送らない(押し直しで送る・iOS と同じ)。
/// AI のクライアントは会話と同じ `MacComposeSession.gatedAI()`(Firebase が無い間はモック・#930)。
/// 課金(Pro)の画面は Mac にまだ無いので出さない。サーバーが Pro を求めたら部品の下に理由を出す。
/// 審査用ビルド(APP_STORE_REVIEW)は iOS と同じくオンライン版を持たない(aiChat は部品ごと外れる)。
extension MacAppRuntime: AITextHost {
    #if APP_STORE_REVIEW
    var supportsOnlineAIText: Bool { false }

    func generateAIText(_ request: AITextRequest) async throws -> AITextResult {
        throw AITextError.unsupported
    }

    func recordAIButton(_ event: AIButtonMetric.Event) {}
    #else
    var supportsOnlineAIText: Bool { true }

    func generateAIText(_ request: AITextRequest) async throws -> AITextResult {
        guard isAIConsentGranted() else {
            needsAIConsent = true
            throw AITextError.needsConsent
        }
        return try await ai.text(request.inApp(appId))
    }

    /// 端末内 AI の集計(#446)。設定の「AI ボタンの利用状況を送る」で止められる。送るのは appconfig につながってから(#930)
    func recordAIButton(_ event: AIButtonMetric.Event) {
        AIButtonMetricsRecorder.shared.record(event)
    }
    #endif
}

#if !APP_STORE_REVIEW
/// `roleField` の無い aiChat(stg で作られた古い DSL)の 1 往復を対で保存する(#1326・iOS の `AppRuntime+AIChat.swift` と同じ)。
/// 役は並び順で読むので、返事を保存できなければ自分の発言も消し、後でどちらかが断られたらもう片方も消す。
extension MacAppRuntime: AIChatPairSaving {
    func saveAIChatPair(entity: String, user: [String: JSONValue], assistant: [String: JSONValue]) async -> Bool {
        await saveAIChatPairIds(entity: entity, user: user, assistant: assistant) != nil
    }

    /// 保存した 2 件の id(仮 id)。どちらかを保存できなければ何も残さず nil。
    func saveAIChatPairIds(entity: String, user: [String: JSONValue],
                           assistant: [String: JSONValue]) async -> (user: String, assistant: String)? {
        await aiChatPairs.save(appId: appId, entity: entity, user: user, assistant: assistant,
                               create: { await self.createRecordId(entity: entity, data: $0) },
                               remove: { await self.removeAIChatTurn(entity: entity, recordId: $0) })
    }

    /// 対の片方が断られた(ローカル行はキューが消した)なら、もう片方も消す。
    func revertAIChatPartner(for event: SyncEvent) async {
        guard let refused = event.refusedWrite, refused.appId == appId, let id = refused.recordId,
              let partner = aiChatPairs.takePartner(appId: appId, entity: refused.entity, recordId: id) else { return }
        await removeAIChatTurn(entity: refused.entity, recordId: partner)
    }

    private func removeAIChatTurn(entity: String, recordId: String) async {
        _ = await recordSync.delete(appId: appId, entity: entity, recordId: recordId)
        await reloadLocal(entity)
    }
}
#endif
