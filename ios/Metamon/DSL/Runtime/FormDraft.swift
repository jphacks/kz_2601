import Foundation
import Observation
import MetamonDSL

/// `screen.entity` を持つ画面が1件だけ持つドラフト。入力系 Leaf の `field` はこのキー。
/// recordId 付きで開かれたら既存レコードをロードして編集モード。
@MainActor
@Observable
final class FormDraft {
    let entity: Entity
    private(set) var recordId: String?
    var values: [String: JSONValue] = [:]

    init(entity: Entity, record: Record? = nil) {
        self.entity = entity
        reset()
        if let record { load(record) }
    }

    var isEditing: Bool { recordId != nil }

    func reset() {
        recordId = nil
        values = [:]
        for f in entity.fields {
            values[f.id] = FieldCoding.defaultValue(for: f)
        }
    }

    func load(_ record: Record) {
        recordId = record.id
        for f in entity.fields {
            values[f.id] = record.data[f.id] ?? FieldCoding.defaultValue(for: f)
        }
    }

    /// appconfig に送る `data`(Field.type に合わせて正規化・空は落とす)。
    var payload: [String: JSONValue] {
        var out: [String: JSONValue] = [:]
        for f in entity.fields {
            if let v = FieldCoding.normalize(values[f.id] ?? .null, for: f) { out[f.id] = v }
        }
        return out
    }

    /// required 未入力のフィールド名。
    var missingRequired: [String] {
        entity.fields.filter { ($0.required ?? false) && FieldCoding.normalize(values[$0.id] ?? .null, for: $0) == nil }.map(\.name)
    }
}
