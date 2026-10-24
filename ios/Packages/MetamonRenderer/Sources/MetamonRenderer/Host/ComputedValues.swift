import Foundation
import MetamonDSL

/// 計算フィールド(#101)を Renderer に流し込む共通口。
/// 部品は `rt.records(for:)` の代わりに `rt.viewRecords(for:)` を使う → computed が data に合成済みなので、
/// list / table / stat(aggregate) / chart(groupBy・field)/ sort はそのまま computed を扱える。保存レコードには入らない。
@MainActor
public extension DSLRuntime {
    /// レコードと entity 定義が変わらない間は同じ評価器(= computed / agg のメモ)を使い回す。
    /// 署名を取るために全 entity の records(for:) を読むので、Observation の追跡も自然に張られる。
    func evaluator() -> ExprEvaluator {
        var h = Hasher()
        for e in dsl.entities {
            h.combine(e.id)
            for f in e.fields { h.combine(f.id); h.combine(f.computed) }
            h.combine(records(for: e.id))
        }
        let signature = h.finalize()
        let key = ObjectIdentifier(self)
        if let hit = EvaluatorCache.entries[key], hit.signature == signature { return hit.evaluator }
        let ev = ExprEvaluator(dsl: dsl, records: { [weak self] in self?.records(for: $0) ?? [] })
        EvaluatorCache.entries[key] = (signature, ev)
        return ev
    }

    /// computed 合成済みのレコード。
    func viewRecords(for entity: String) -> [Record] {
        guard dsl.entity(entity)?.computedFields.isEmpty == false else { return records(for: entity) }
        return evaluator().viewRecords(for: entity)
    }

    /// 部品の entity + filter(単体/配列・value の Expr 可)を適用した表示用レコード。
    func filteredRecords(_ c: Component) -> [Record] {
        let entityId = c.entity ?? ""
        let rows = viewRecords(for: entityId)
        guard !c.filters.isEmpty else { return rows }
        if c.filters.allSatisfy({ $0.valueExpr == nil }) {
            let entity = dsl.entity(entityId)
            return rows.filter { r in c.filters.allSatisfy { Aggregator.matches($0, record: r, field: entity?.field($0.field)) } }
        }
        return evaluator().apply(c.filters, entity: dsl.entity(entityId), to: rows)
    }

    /// 表示用の文字列。type=ref は参照先レコードの見出しに解決する(未取得・削除済みは「—」)。
    func display(_ value: JSONValue, field: Field?) -> String {
        guard let field, field.type == .ref else { return FieldCoding.display(value, field: field, locale: LanguagePreference.shared.locale) }
        guard let id = value.stringValue, !id.isEmpty else { return "" }
        return refTitle(id, entity: field.refEntity) ?? FieldCoding.unavailable
    }

    /// 参照先 entity のレコード id → 見出し(titleField / 最初の string field)。
    func refTitle(_ id: String, entity refEntity: String?) -> String? {
        guard let refEntity, let target = dsl.entity(refEntity),
              let record = records(for: refEntity).first(where: { $0.id == id }) else { return nil }
        guard let tf = target.displayTitleField else { return record.id }
        return FieldCoding.display(record[tf], field: target.field(tf), locale: LanguagePreference.shared.locale)
    }
}

@MainActor
private enum EvaluatorCache {
    static var entries: [ObjectIdentifier: (signature: Int, evaluator: ExprEvaluator)] = [:]
}
