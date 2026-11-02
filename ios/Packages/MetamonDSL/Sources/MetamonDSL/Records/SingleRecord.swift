import Foundation

/// single の entity(設定値の置き場・#267)の 1 件。docs/dsl/README.md §3.11。
/// ふつうは 0〜1 件。single を知らない古い版が足して 2 件以上ある時は、createdAt が最も新しい 1 件(同じなら後の方)。
/// backend の `expreval.SingleRecord` と同じ。
public enum SingleRecord {
    public static func pick(_ records: [Record]) -> Record? {
        var best: Record?
        for r in records where best == nil || (r.createdAt ?? "") >= (best?.createdAt ?? "") { best = r }
        return best
    }

    /// 保存前の 1 件の data: 保存する field の defaultValue(date / datetime の "now" は当てない)。
    public static func defaults(of entity: Entity) -> [String: JSONValue] {
        var out: [String: JSONValue] = [:]
        for f in entity.storedFields {
            if let v = defaultValue(f) { out[f.id] = v }
        }
        return out
    }

    /// field の defaultValue(式で読む値)。null・date の "now" は nil。
    public static func defaultValue(_ f: Field) -> JSONValue? {
        guard let v = f.defaultValue, !v.isNull else { return nil }
        if f.type.isDate, v.stringValue == "now" { return nil }
        return v
    }
}
