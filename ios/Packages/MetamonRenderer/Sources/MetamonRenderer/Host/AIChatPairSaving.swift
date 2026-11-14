import MetamonDSL

/// `aiChat` の 1 往復(自分の発言 → AI の返事)を対で保存する。`roleField` の無い古い DSL は役を並び順(自分 → AI の交互)で読むので、
/// 片方だけ残ると以後の役がすべて入れ替わる。対応するホストは、片方が保存できない・サーバーに断られた時にもう片方も消す。
/// `DSLRuntime` 本体には足さず、対応するホストだけが準拠する(未対応のランタイムは `createRecord` を 2 回)。
@MainActor
public protocol AIChatPairSaving: AnyObject {
    /// 2 件とも保存が決まったら true。false の時は何も残さない。
    func saveAIChatPair(entity: String, user: [String: JSONValue], assistant: [String: JSONValue]) async -> Bool
}

@MainActor
extension DSLRuntime {
    /// 自分の発言と AI の返事を続けて保存する。`AIChatPairSaving` ならホストが対で扱う。
    func saveTurnPair(entity: String, user: [String: JSONValue], assistant: [String: JSONValue]) async -> Bool {
        if let host = self as? AIChatPairSaving { return await host.saveAIChatPair(entity: entity, user: user, assistant: assistant) }
        guard await createRecord(entity: entity, data: user) else { return false }
        try? await Task.sleep(for: .milliseconds(5)) // 作成時刻で並べるので、自分 → AI の順を同じ時刻にしない
        return await createRecord(entity: entity, data: assistant)
    }
}
