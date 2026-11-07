import Foundation

/// `aiChat` の会話の並び・役・送る依頼・保存するデータ。描画は Renderer の `AiChatLeaf`。
/// 1 発言 = `entity` の 1 レコード。役は `roleField`(user / assistant)、無ければ古い順に自分 → AI の交互。
public enum AIChat {
    public enum Role: String, Sendable { case user, assistant }

    public struct Turn: Equatable, Sendable {
        public var id: String
        public var role: Role
        public var text: String

        public init(id: String, role: Role, text: String) {
            self.id = id
            self.role = role
            self.text = text
        }
    }

    /// backend `dsl.MaxAIChatTurns` / `dsl.MaxAIChatBytes`。
    public static let maxTurns = 20
    public static let maxBytes = 16 * 1024

    /// 会話に使うのは自分の記録だけ: author が無い(private の entity・未送信)か、サインイン中の uid と同じもの。
    /// entity は backend の検証で private に限るが、古い・手で作った DSL でも他の人の発言を履歴に混ぜない。
    public static func own(_ records: [Record], uid: String?) -> [Record] {
        records.filter { $0.author == nil || ($0.author == uid && uid != nil) }
    }

    /// 古い順の発言。本文の無いレコードは飛ばす(交互の数え方にも入れない)。
    public static func turns(_ records: [Record], textField: String, roleField: String?) -> [Turn] {
        var out: [Turn] = []
        for r in ChatThread.ordered(records) {
            let text = (r[textField].stringValue ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !text.isEmpty else { continue }
            let role: Role
            if let roleField {
                role = r[roleField].stringValue == Role.assistant.rawValue ? .assistant : .user
            } else {
                role = out.count.isMultiple(of: 2) ? .user : .assistant
            }
            out.append(Turn(id: r.id, role: role, text: text))
        }
        return out
    }

    /// `POST /v1/text` の task chat。直近 20 発言・本文 16 KB に収まる分だけ(古い方から落とす)。
    /// 最後は利用者の発言(`next`)。それ 1 つで 16 KB を超える時はそこで切る。
    public static func request(persona: String, history: [Turn], next: String, long: Bool, locale: String) -> AITextRequest {
        let last = AITextLimits.clip(next.trimmingCharacters(in: .whitespacesAndNewlines), maxBytes: maxBytes)
        var picked = [AITextMessage(role: Role.user.rawValue, text: last)]
        var used = last.utf8.count
        for t in history.reversed() {
            guard picked.count < maxTurns, used + t.text.utf8.count <= maxBytes else { break }
            picked.insert(AITextMessage(role: t.role.rawValue, text: t.text), at: 0)
            used += t.text.utf8.count
        }
        while picked.first?.role == Role.assistant.rawValue { picked.removeFirst() } // 先頭は利用者から
        let instruction = String(persona.trimmingCharacters(in: .whitespacesAndNewlines).prefix(AITextLimits.maxInstruction))
        return AITextRequest(instruction: instruction, shape: .line, locale: locale, task: "chat",
                             length: long ? "long" : nil, messages: picked)
    }

    /// 保存する 1 件の data。本文が空なら nil。`base` は entity の他の field の既定値。
    public static func record(text: String, role: Role, textField: String, roleField: String?,
                              base: [String: JSONValue] = [:]) -> [String: JSONValue]? {
        let body = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !body.isEmpty else { return nil }
        var data = base
        data[textField] = .string(body)
        if let roleField { data[roleField] = .string(role.rawValue) }
        return data
    }
}
