import Foundation
import UniformTypeIdentifiers
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

    /// 行を外へドラッグする時(Mac・#1334)に渡すもの。中身はコピーと同じテキスト。空なら何も載せない。
    static func dragItem(_ text: String) -> NSItemProvider {
        text.isEmpty ? NSItemProvider() : NSItemProvider(object: text as NSString)
    }

    /// 複数の行(Mac・#1398)を ⌘C・ドラッグで持ち出す時の CSV。列と値の形は書き出し(`export` の CSV)と同じ
    /// (写真・動画は入れない・隠し項目は自分が書いた記録だけ)。BOM は付けない(テキストとして貼るため)。
    @MainActor
    static func csv(records: [Record], entity: Entity?, rt: any DSLRuntime) -> String {
        guard let entity, !records.isEmpty else { return "" }
        let table = RecordExport.table(title: entity.name, entity: entity, records: records, fields: nil,
                                       currentUid: rt.currentUserId) { rt.display($0, field: $1) }
        return CSVWriter.string(table)
    }

    /// 複数の行を外へドラッグする時に渡すもの: CSV のファイル(Finder・Numbers)と、同じ中身のテキスト(メモ・メール)。
    static func csvDragItem(_ csv: String, title: String) -> NSItemProvider {
        let provider = NSItemProvider()
        guard !csv.isEmpty else { return provider }
        let name = RecordExport.fileName(title, ext: "csv")
        let data = CSVWriter.bom + Data(csv.utf8)
        provider.suggestedName = name
        provider.registerFileRepresentation(forTypeIdentifier: UTType.commaSeparatedText.identifier, visibility: .all) { done in
            do {
                let dir = ExportFiles.directory.appending(path: UUID().uuidString, directoryHint: .isDirectory)
                try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
                let url = dir.appending(path: name)
                try data.write(to: url, options: .atomic)
                done(url, false, nil)
            } catch {
                done(nil, false, error)
            }
            return nil
        }
        provider.registerObject(csv as NSString, visibility: .all)
        return provider
    }

    private static let mediaTypes: Set<FieldType> = [.photo, .video, .audio]
}
