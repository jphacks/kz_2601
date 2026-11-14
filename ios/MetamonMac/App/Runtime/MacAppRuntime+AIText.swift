import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// aiButton・aiGenerate・aiChat(#962)。部品は Renderer の iOS と同じもの: 端末内 AI(Foundation Models・macOS 26+)を先に使い、
/// 使えない Mac・長い文・写真・aiChat はここのオンライン版に回す(iOS の `AppRuntime+AIText` と同じ道)。
/// 送る前に同意(#246)を確かめ、無ければ同意の説明を出して送らない(押し直しで送る・iOS と同じ)。
/// AI のクライアントは会話と同じ `MacComposeSession.gatedAI()`(Firebase が無い間はモック・#930)。
/// サーバーが Pro を求めたら Mac の課金画面(`MacPro`・iOS と同じ商品)を出し、部品の下にも理由を出す(押し直しで再実行)。
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
        do {
            return try await ai.text(request.inApp(appId))
        } catch AITextError.needsPro {
            requestProForAIText(chat: request.isChat)
            throw AITextError.needsPro
        }
    }

    /// iOS の `requestProForAIText` と同じ文言で課金画面を出す(閉じるのは待たない)
    func requestProForAIText(chat: Bool) {
        let feature = chat
            ? PaidFeature(id: "aiChat", title: L10n.tr("AI と話す"),
                          reason: L10n.tr("AI と話すには Tailor It Pro が必要です。会話はサーバーの AI で返します。"))
            : PaidFeature(id: "aiText", title: L10n.tr("オンラインの AI で書く"),
                          reason: L10n.tr("この Mac では AI を端末内で動かせないため、サーバーの AI を使います。"))
        let note = chat ? L10n.tr("閉じると、AI と話す部品は使えません。") : L10n.tr("閉じると、この Mac では AI で書くボタンは使えません。")
        MacPro.shared.requestPro(.paidFeature([feature]), closeNote: note)
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
