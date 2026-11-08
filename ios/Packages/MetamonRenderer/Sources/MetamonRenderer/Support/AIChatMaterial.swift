import Foundation
import MetamonDSL

/// `aiChat.inputEntity`: 会話に添える「参考の記録」(`POST /v1/text` の `material`)。検証は backend `dsl/ai_chat_input.go`。
/// 送るのは自分の記録だけ(`AIChat.own` = author が無いかサインイン中の uid と同じ。shared / public でも他の人の分は送らない・#733 の aiButton と同じ規則)。
/// 新しい順に `limit` 件(既定・上限 20)、1 件 1 行の「名前: 値」(aiButton の `AIRequestBuilder.describe`)で 8 KB まで。
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

    /// 添える記録(自分の分・新しい順・limit 件)。
    static func select(_ records: [Record], uid: String?, limit: Int?) -> [Record] {
        let n = Swift.min(Swift.max(limit ?? maxRecords, 1), maxRecords)
        return Array(AIChat.own(records, uid: uid)
            .sorted { ($0.updatedAt ?? $0.createdAt ?? "") > ($1.updatedAt ?? $1.createdAt ?? "") }
            .prefix(n))
    }

    /// 依頼に付ける material。送れる記録・値が無ければ nil(添えずに話す)。
    static func request(entity: Entity, records: [Record], uid: String?, limit: Int?) -> AITextMaterial? {
        let fs = fields(of: entity)
        guard !fs.isEmpty else { return nil }
        var lines: [String] = []
        var used = 0
        for r in select(records, uid: uid, limit: limit) {
            let body = AIRequestBuilder.describe(fs, value: { r.data[$0] ?? .null }, separator: " / ")
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
