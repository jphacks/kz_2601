import SwiftUI
import Observation
import MetamonDSL

/// `screen.entity` を持つ画面が1件だけ持つドラフト。入力系 Leaf の `field` はこのキー。
/// recordId 付きで開かれたら既存レコードをロードして編集モード。
@MainActor
@Observable
public final class FormDraft {
    public let entity: Entity
    public private(set) var recordId: String?
    public var values: [String: JSONValue] = [:]

    public init(entity: Entity, record: Record? = nil) {
        self.entity = entity
        reset()
        if let record { load(record) }
    }

    public var isEditing: Bool { recordId != nil }

    public func reset() {
        recordId = nil
        values = [:]
        for f in entity.fields {
            values[f.id] = FieldCoding.defaultValue(for: f)
        }
    }

    public func load(_ record: Record) {
        recordId = record.id
        for f in entity.fields {
            values[f.id] = record.data[f.id] ?? FieldCoding.defaultValue(for: f)
        }
    }

    /// appconfig に送る `data`(Field.type に合わせて正規化・空は落とす)。
    public var payload: [String: JSONValue] {
        var out: [String: JSONValue] = [:]
        for f in entity.fields {
            if let v = FieldCoding.normalize(values[f.id] ?? .null, for: f) { out[f.id] = v }
        }
        return out
    }

    /// required 未入力のフィールド名。
    public var missingRequired: [String] {
        entity.fields.filter { ($0.required ?? false) && FieldCoding.normalize(values[$0.id] ?? .null, for: $0) == nil }.map(\.name)
    }
}

// MARK: - SwiftUI Binding(入力系 Leaf が `field` で束縛する)

extension FormDraft {
    func value(_ field: String) -> Binding<JSONValue> {
        Binding(get: { self.values[field] ?? .null }, set: { self.values[field] = $0 })
    }

    func string(_ field: String) -> Binding<String> {
        Binding(get: { (self.values[field] ?? .null).displayString }, set: { self.values[field] = .string($0) })
    }

    func number(_ field: String) -> Binding<String> {
        Binding(
            get: { (self.values[field] ?? .null).displayString },
            set: { s in
                let cleaned = s.replacingOccurrences(of: ",", with: "")
                self.values[field] = cleaned.isEmpty ? .null : (Double(cleaned).map { .number($0) } ?? .string(cleaned))
            })
    }

    func bool(_ field: String) -> Binding<Bool> {
        Binding(get: { (self.values[field] ?? .null).truthy }, set: { self.values[field] = .bool($0) })
    }

    func date(_ field: String) -> Binding<Date> {
        let f = entity.field(field)
        return Binding(
            get: { FieldCoding.date(from: self.values[field] ?? .null) ?? Date() },
            set: { d in
                guard let f else { return }
                self.values[field] = .string(FieldCoding.encode(d, for: f))
            })
    }
}
