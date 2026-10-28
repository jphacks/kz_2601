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

    /// computed 合成済みの表示用レコード。hidden(#262)の値は落とす(computed はその前に元の値で評価済み)。
    func viewRecords(for entity: String) -> [Record] {
        HiddenFields.redact(unredactedViewRecords(for: entity), in: dsl.entity(entity))
    }

    /// 部品の entity + filter(単体/配列・value の Expr 可)を適用した表示用レコード。filter は hidden を落とす前に当てる。
    /// `{"me": true}`(#251)はサインイン中の uid に置き換えて投稿者と比べる。
    func filteredRecords(_ c: Component) -> [Record] {
        let entityId = c.entity ?? ""
        let entity = dsl.entity(entityId)
        let rows = unredactedViewRecords(for: entityId)
        guard !c.filters.isEmpty else { return HiddenFields.redact(rows, in: entity) }
        let filters = c.filters.map { $0.resolvingMe(uid: currentUserId) }
        let filtered: [Record]
        if filters.allSatisfy({ $0.valueExpr == nil }) {
            filtered = rows.filter { r in filters.allSatisfy { Aggregator.matches($0, record: r, field: entity?.field($0.field)) } }
        } else {
            filtered = evaluator().apply(filters, entity: entity, to: rows)
        }
        return HiddenFields.redact(filtered, in: entity)
    }

    private func unredactedViewRecords(for entity: String) -> [Record] {
        guard dsl.entity(entity)?.computedFields.isEmpty == false else { return records(for: entity) }
        return evaluator().viewRecords(for: entity)
    }

    /// 表示用の文字列。type=ref は参照先レコードの見出しに解決する(未取得・削除済みは「—」)。
    func display(_ value: JSONValue, field: Field?) -> String {
        if let field, field.type == .refs { return refsTitle(value, field: field) }
        guard let field, field.type == .ref else { return displayText(value, field: field) }
        guard let id = value.stringValue, !id.isEmpty else { return "" }
        // enum から ref に変えた後の古い記録は選択肢の名前そのものを持つので、そのまま出す(#194)
        return refTitle(id, entity: field.refEntity) ?? (field.options?.contains(id) == true ? id : FieldCoding.unavailable)
    }

    /// type=refs(#231): 選んだレコードの見出しを「、」で連結。削除済みは飛ばし、未選択(残り 0 件)は「全員」。
    func refsTitle(_ value: JSONValue, field: Field) -> String {
        let names = Aggregator.refIds(value).compactMap { refTitle($0, entity: field.refEntity) }
        guard !names.isEmpty else { return L("全員") }
        let list = ListFormatter()
        list.locale = LanguagePreference.shared.locale
        return list.string(from: names) ?? names.joined(separator: ", ")
    }

    /// 参照先 entity のレコード id → 見出し(titleField / 最初の string field)。
    func refTitle(_ id: String, entity refEntity: String?) -> String? {
        guard let refEntity, let target = dsl.entity(refEntity),
              let record = records(for: refEntity).first(where: { $0.id == id }) else { return nil }
        guard let tf = target.displayTitleField else { return record.id }
        return displayText(record[tf], field: target.field(tf))
    }
}

@MainActor
private enum EvaluatorCache {
    static var entries: [ObjectIdentifier: (signature: Int, evaluator: ExprEvaluator)] = [:]
}
