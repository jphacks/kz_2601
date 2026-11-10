import MetamonDSL

/// 一覧の行の「コピー」(#1332)で書くテキスト。行に見えている項目(title / subtitle / trailing)を
/// 「項目名: 値」で 1 行ずつ。値は画面と同じ `rt.display`。空・重複・隠し項目・メディア(写真/動画/録音)は出さない。
enum ListRowCopyText {
    @MainActor
    static func text(item: ListItem?, record: Record, entity: Entity?, rt: any DSLRuntime) -> String {
        let ids = [item?.title ?? entity?.displayTitleField, item?.subtitle, item?.trailing].compactMap { $0 }
        var seen: Set<String> = []
        var lines: [String] = []
        for id in ids where seen.insert(id).inserted {
            let field = entity?.field(id)
            if let field, field.hidden || mediaTypes.contains(field.type) { continue }
            let value = rt.display(record[id], field: field).trimmingCharacters(in: .whitespacesAndNewlines)
            guard !value.isEmpty else { continue }
            let name = field?.name ?? ""
            lines.append(name.isEmpty ? value : "\(name): \(value)")
        }
        return lines.joined(separator: "\n")
    }

    private static let mediaTypes: Set<FieldType> = [.photo, .video, .audio]
}
