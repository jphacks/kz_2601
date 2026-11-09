import Foundation
import MetamonDSL

/// `aiChat.inputEntity`: 会話に添える「参考の記録」(`POST /v1/text` の `material`)。検証は backend `dsl/ai_chat_input.go`。
/// 送るのは自分の記録だけ(`AIChat.isOwn` = author がサインイン中の uid と同じ。author の無い記録は private の entity なら自分、
/// shared / public では未送信の分だけ自分。他の人の分・サーバーの規則が作った記録は送らない・#733 の aiButton と同じ規則)。
/// 新しい順に `limit` 件(既定・上限 20)、1 件 1 行の「名前: 値」(aiButton の `AIRequestBuilder.describe`)で 8 KB まで。
/// 値の中の改行は「 ⏎ 」にする(1 件が何件にも見えないように)。
/// hidden の field は誰の記録でも送らない。写真・位置・参照など読めない型と computed も送らない。
enum AIChatMaterial {
    /// backend `dsl.MaxAIChatInputRecords` / `dsl.MaxAIChatMaterialBytes`。
    static let maxRecords = 20
    static let maxBytes = 8 * 1024
    /// backend `dsl.AIChatInputFieldTypes`。
    static let fieldTypes: Set<FieldType> = [.string, .text, .integer, .number, .boolean, .date, .datetime, .enum, .currency]
    /// backend の `material.name` の上限(rune)。
    static let maxNameScalars = 40

    /// AI に読ませる field(DSL の順)。
    static func fields(of entity: Entity) -> [Field] {
        entity.storedFields.filter { !$0.hidden && fieldTypes.contains($0.type) }
    }

    /// 添える記録(自分の分・新しい順・limit 件)。時刻は updatedAt → createdAt を日時として比べ、どちらも無い(作ったばかりの)記録は先頭。
    static func select(_ records: [Record], uid: String?, shared: Bool, limit: Int?) -> [Record] {
        let n = Swift.min(Swift.max(limit ?? maxRecords, 1), maxRecords)
        return Array(AIChat.own(records, uid: uid, shared: shared)
            .map { ($0, ChatThread.date($0.updatedAt) ?? ChatThread.date($0.createdAt)) }
            .sorted { a, b in
                switch (a.1, b.1) {
                case let (x?, y?) where x != y: x > y
                case (nil, _?): true
                case (_?, nil): false
                default: a.0.id < b.0.id
                }
            }
            .prefix(n)
            .map(\.0))
    }

    /// 1 件を 1 行に保つ: 値の中の改行を「 ⏎ 」にする。
    static func oneLine(_ s: String) -> String {
        s.split(omittingEmptySubsequences: false, whereSeparator: \.isNewline).joined(separator: " ⏎ ")
    }

    /// 依頼に付ける material。送れる記録・値が無ければ nil(添えずに話す)。
    static func request(entity: Entity, records: [Record], uid: String?, limit: Int?) -> AITextMaterial? {
        let fs = fields(of: entity)
        guard !fs.isEmpty else { return nil }
        var lines: [String] = []
        var used = 0
        for r in select(records, uid: uid, shared: entity.isShared, limit: limit) {
            let body = oneLine(AIRequestBuilder.describe(fs, value: { r.data[$0] ?? .null }, separator: " / "))
            guard !body.isEmpty else { continue }
            let line = "- " + body
            let sep = lines.isEmpty ? 0 : 1
            if used + sep + line.utf8.count <= maxBytes {
                lines.append(line)
                used += sep + line.utf8.count
                continue
            }
            if lines.isEmpty { lines.append(AITextLimits.clip(line, maxBytes: maxBytes)) } // 1 件目だけは切って入れる
            break // 入りきらない古い記録は落とす
        }
        guard !lines.isEmpty else { return nil }
        let name = String(String.UnicodeScalarView(entity.name.trimmingCharacters(in: .whitespacesAndNewlines)
            .unicodeScalars.prefix(maxNameScalars))).trimmingCharacters(in: .whitespacesAndNewlines)
        return AITextMaterial(entity: entity.id, name: name.isEmpty ? nil : name, text: lines.joined(separator: "\n"))
    }
}
