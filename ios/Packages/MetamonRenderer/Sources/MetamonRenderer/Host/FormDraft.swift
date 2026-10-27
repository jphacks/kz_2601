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
    /// 編集中レコードの投稿者 uid(public/shared entity のみ)。新規・private は nil。
    public private(set) var author: String?
    public var values: [String: JSONValue] = [:]

    public init(entity: Entity, record: Record? = nil) {
        self.entity = entity
        reset()
        if let record { load(record) }
    }

    public var isEditing: Bool { recordId != nil }

    public func reset() {
        recordId = nil
        author = nil
        values = [:]
        for f in entity.storedFields {
            values[f.id] = FieldCoding.defaultValue(for: f)
        }
    }

    public func load(_ record: Record) {
        recordId = record.id
        author = record.author
        for f in entity.storedFields {
            values[f.id] = record.data[f.id] ?? FieldCoding.defaultValue(for: f)
        }
    }

    /// `Field.hidden`(#262)の入力を出してよいか(新規 / 自分が投稿したレコード)。
    public func showsHiddenFields(currentUid: String?) -> Bool {
        guard isEditing else { return true }
        return HiddenFields.canEdit(record: Record(id: recordId ?? "", data: [:], author: author), currentUid: currentUid)
    }

    /// 部品 `c` が束縛する field が、このドラフトでは隠す hidden field か。
    public func hides(_ c: Component, currentUid: String?) -> Bool {
        guard let fid = c.field, c.entity == nil || c.entity == entity.id, entity.field(fid)?.hidden == true else { return false }
        return !showsHiddenFields(currentUid: currentUid)
    }

    /// appconfig に送る `data`(Field.type に合わせて正規化・空は落とす)。
    public var payload: [String: JSONValue] {
        var out: [String: JSONValue] = [:]
        // computed は保存しない(#101)
        for f in entity.storedFields {
            if let v = FieldCoding.normalize(values[f.id] ?? .null, for: f) { out[f.id] = v }
        }
        return out
    }

    /// required 未入力のフィールド名。
    public var missingRequired: [String] {
        entity.storedFields.filter { ($0.required ?? false) && FieldCoding.normalize(values[$0.id] ?? .null, for: $0) == nil }.map(\.name)
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
