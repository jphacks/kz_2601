import Foundation

/// `aiChat` の会話の並び・役・送る依頼・保存するデータ。描画は Renderer の `AiChatLeaf`。
/// 1 発言 = `entity` の 1 レコード。役は `roleField`(user / assistant・backend は必須)。
/// roleField の無い古い DSL(stg で作られたもの)だけ古い順に自分 → AI の交互で読む(その時は対で保存して交互を崩さない)。
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

    /// 会話・参考の記録に使うのは自分の記録だけ。entity は backend の検証で private に限るが、古い・手で作った DSL でも他の人の分を混ぜない。
    public static func own(_ records: [Record], uid: String?, shared: Bool) -> [Record] {
        records.filter { isOwn($0, uid: uid, shared: shared) }
    }

    /// 自分の記録か。author(空文字は無いのと同じ)がサインイン中の uid と同じなら自分。
    /// author が無い時: private の entity は全部自分。shared / public はサーバーの規則が作った記録や古い記録も author が無いので、
    /// 端末で作ってまだ送っていない記録(`Record.unsent`)だけを自分とみなす。
    public static func isOwn(_ record: Record, uid: String?, shared: Bool) -> Bool {
        if let author = record.author, !author.isEmpty {
            guard let uid, !uid.isEmpty else { return false }
            return author == uid
        }
        return !shared || record.unsent
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

    /// `speak: true` で読み上げる返事: 保存した直後の並びで、本文が `reply` と同じ一番新しい AI の発言。
    /// 見つからない(保存に失敗・並びがまだ古い)時は nil(読み上げの印を付ける吹き出しが無い)。
    public static func spokenReply(_ turns: [Turn], reply: String) -> Turn? {
        let text = reply.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return nil }
        return turns.last { $0.role == .assistant && $0.text == text }
    }

    /// `POST /v1/text` の task chat。直近 20 発言・本文 16 KB に収まる分だけ(古い方から落とす)。
    /// 最後は利用者の発言(`next`)。それ 1 つで 16 KB を超える時はそこで切る。
    /// `saveTo` は aiChat saveTo の field の形(`AIChatSaveTo.request`・nil = 記録の案なし)。
    /// `material` は inputEntity の参考の記録(8 KB まで)。付ける時は会話を残りの `maxBytes - material` に収める(合計 16 KB)。
    public static func request(persona: String, history: [Turn], next: String, long: Bool, locale: String,
                               saveTo: AITextSaveTo? = nil, material: AITextMaterial? = nil) -> AITextRequest {
        let maxBytes = Self.maxBytes - (material?.text.utf8.count ?? 0)
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
                             length: long ? "long" : nil, messages: picked, saveTo: saveTo, material: material)
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
