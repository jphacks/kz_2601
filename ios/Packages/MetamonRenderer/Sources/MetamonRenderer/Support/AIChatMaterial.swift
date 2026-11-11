import Foundation
import MetamonDSL

/// `aiChat.inputEntity`: 会話に添える「参考の記録」(`POST /v1/text` の `material`)。検証は backend `dsl/ai_chat_input.go`。
/// 送るのは自分の記録だけ(`AIChat.isOwn` = author がサインイン中の uid と同じ。author の無い記録は private の entity なら自分、
/// shared / public では未送信の分だけ自分。他の人の分・サーバーの規則が作った記録は送らない・#733 の aiButton と同じ規則)。
/// 最後の発言に近い記録から(`AIChatRelevance`: 語が重なる記録を重なりの大きい順・残りは新しい順)`limit` 件(既定・上限 20)、
/// 1 件 1 行の「名前: 値」(aiButton の `AIRequestBuilder.describe`)で 8 KB まで。発言が無い・語が重ならなければ新しい順(#748 と同じ)。
/// 入りきらない記録は飛ばし、選んだ limit 件のうち後ろの入る記録で埋める(大きい記録が関係の深い小さい記録を塞がない)。
/// 値の中の改行は「 ⏎ 」にする(1 件が何件にも見えないように)。
/// hidden と暗号化して保存する(#233)field は誰の記録でも送らない。写真・位置・参照など読めない型と computed も送らない。
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
        entity.storedFields.filter { !$0.hidden && !$0.encrypted && fieldTypes.contains($0.type) }
    }

    /// 添える記録(自分の分・limit 件)。並びは `query` に近い記録が先(同点は新しい順)、残りは新しい順。
    /// 時刻は updatedAt → createdAt を日時として比べ、どちらも無い(作ったばかりの)記録は先頭。同じ時刻は id 順。
    static func select(_ records: [Record], uid: String?, shared: Bool, limit: Int?, query: String? = nil,
                       fields: [Field] = []) -> [Record] {
        let n = Swift.min(Swift.max(limit ?? maxRecords, 1), maxRecords)
        let newest = AIChat.own(records, uid: uid, shared: shared)
            .map { ($0, ChatThread.date($0.updatedAt) ?? ChatThread.date($0.createdAt)) }
            .sorted { a, b in
                switch (a.1, b.1) {
                case let (x?, y?) where x != y: x > y
                case (nil, _?): true
                case (_?, nil): false
                default: a.0.id < b.0.id
                }
            }
            .map(\.0)
        let q = AIChatRelevance.tokens(query ?? "")
        guard !q.isEmpty, !fields.isEmpty else { return Array(newest.prefix(n)) }
        let scores = AIChatRelevance.scores(query: q, documents: newest.map { r in values(fs: fields, of: r) })
        // 点のある記録を点の高い順(同点は新しい順 = newest の位置)、続いて点の無い記録を新しい順。安定で端末ごとに同じ
        let order = newest.indices.sorted { a, b in
            scores[a] != scores[b] ? scores[a] > scores[b] : a < b
        }
        return order.prefix(n).map { newest[$0] }
    }

    /// 点を付ける文: 送る field の値だけ(field 名は全件同じなので入れない)。
    static func values(fs: [Field], of r: Record) -> String {
        fs.map { FieldCoding.display(r.data[$0.id] ?? .null, field: $0) }.joined(separator: "\n")
    }

    /// 1 件を 1 行に保つ: 値の中の改行を「 ⏎ 」にする。
    static func oneLine(_ s: String) -> String {
        s.split(omittingEmptySubsequences: false, whereSeparator: \.isNewline).joined(separator: " ⏎ ")
    }

    /// 依頼に付ける material。送れる記録・値が無ければ nil(添えずに話す)。`query` は利用者の最後の発言。
    static func request(entity: Entity, records: [Record], uid: String?, limit: Int?, query: String? = nil) -> AITextMaterial? {
        let fs = fields(of: entity)
        guard !fs.isEmpty else { return nil }
        let q = AIChatRelevance.tokens(query ?? "")
        var lines: [String] = []
        var used = 0
        for r in select(records, uid: uid, shared: entity.isShared, limit: limit, query: query, fields: fs) {
            let body = oneLine(AIRequestBuilder.describe(fs, value: { r.data[$0] ?? .null }, separator: " / "))
            guard !body.isEmpty else { continue }
            let line = "- " + body
            let sep = lines.isEmpty ? 0 : 1
            if used + sep + line.utf8.count <= maxBytes {
                lines.append(line)
                used += sep + line.utf8.count
                continue
            }
            // 1 件目だけは切って入れる(取り込んだ PDF など。発言に近い所があればそこから)。8 KB を使い切るので終わり
            if lines.isEmpty {
                lines.append(AIChatRelevance.window(line, query: q, maxBytes: maxBytes))
                break
            }
            // 入りきらない記録は飛ばして次へ(後ろの小さい記録で埋める)
        }
        guard !lines.isEmpty else { return nil }
        let name = String(String.UnicodeScalarView(entity.name.trimmingCharacters(in: .whitespacesAndNewlines)
            .unicodeScalars.prefix(maxNameScalars))).trimmingCharacters(in: .whitespacesAndNewlines)
        return AITextMaterial(entity: entity.id, name: name.isEmpty ? nil : name, text: lines.joined(separator: "\n"))
    }
}
