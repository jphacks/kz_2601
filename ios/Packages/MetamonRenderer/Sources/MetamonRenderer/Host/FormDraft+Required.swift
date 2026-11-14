import MetamonDSL

// MARK: - 空の必須欄の印(M10)

extension FormDraft {
    /// 保存の前に呼ぶ。空の必須を印の対象にして、その名前を返す(空の配列なら保存してよい)。
    @discardableResult
    public func flagMissingRequired() -> [String] {
        let missing = missingRequiredFields
        flaggedFields = Set(missing.map(\.id))
        if !missing.isEmpty { flagCount += 1 }
        return missing.map(\.name)
    }

    /// 印を出す欄か。保存で弾かれた欄のうち、まだ空のもの(入れた欄からは印が消える)。
    func isFlagged(_ field: String) -> Bool {
        guard flaggedFields.contains(field), let f = entity.field(field) else { return false }
        return FieldCoding.normalize(values[field] ?? .null, for: f) == nil
    }

    var missingRequiredFields: [Field] {
        entity.storedFields.filter { ($0.required ?? false) && FieldCoding.normalize(values[$0.id] ?? .null, for: $0) == nil }
    }
}
