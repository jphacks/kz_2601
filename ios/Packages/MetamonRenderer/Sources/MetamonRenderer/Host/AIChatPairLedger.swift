import MetamonDSL

/// `AIChatPairSaving` の中身(iOS の `AppRuntime` と Mac の `MacAppRuntime` で共有)。
/// 自分の発言 → AI の返事の順に 2 件作り、id(仮 id)の対を覚える。返事を作れなければ自分の発言も消し、
/// 後でどちらかがサーバーに断られたら `takePartner` で相方を引いて消す(役は並び順で読むので交互を崩さない)。
@MainActor
public final class AIChatPairLedger {
    public struct Partner: Equatable, Sendable {
        public let appId: String
        public let entity: String
        public let partner: String
    }

    /// 仮 id → 相方。どちらの id からも引ける
    public private(set) var partners: [String: Partner] = [:]

    public init() {}

    /// 2 件を順に作る。`create` は作った行の id(保存できなければ nil)、`remove` は作った行を消す。
    /// `isCurrent` が false(作る間に別のアプリへ切り替わった)なら返事を作らず、自分の発言を消す。
    /// どちらかを保存できなければ何も残さず nil。
    public func save(appId: String, entity: String, user: [String: JSONValue], assistant: [String: JSONValue],
                     isCurrent: () -> Bool = { true },
                     create: ([String: JSONValue]) async -> String?,
                     remove: (String) async -> Void) async -> (user: String, assistant: String)? {
        guard let userId = await create(user) else { return nil }
        try? await Task.sleep(for: .milliseconds(5)) // 作成時刻で並べるので、自分 → AI の順を同じ時刻にしない
        guard isCurrent(), let aiId = await create(assistant) else {
            await remove(userId)
            return nil
        }
        partners[userId] = Partner(appId: appId, entity: entity, partner: aiId)
        partners[aiId] = Partner(appId: appId, entity: entity, partner: userId)
        return (userId, aiId)
    }

    /// 断られた行が対の片方なら、対を忘れて相方の id を返す(呼び手が消す)。対でなければ nil。
    public func takePartner(appId: String, entity: String, recordId: String) -> String? {
        guard let pair = partners[recordId], pair.appId == appId, pair.entity == entity else { return nil }
        partners[recordId] = nil
        partners[pair.partner] = nil
        return pair.partner
    }
}
