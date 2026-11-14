import Foundation

/// 別の生成アプリのフォームを、値を入れた状態で新規に開く時の下ごしらえ(#2235・openApp #1893 の本体の口。DSL は変えない)。
/// 開く・戻るはホスト(iOS は `AppRuntime.openForm`)、ここは「どの画面を開き、どの項目に何を入れるか」の純粋ロジックだけ。
/// 値の合わせ方は記録の貼り付け(`RecordClipboard`)と同じ: 項目の id → 名前(前後の空白・大文字小文字を無視)で探し、
/// 貼り付けと同じ規則で型を合わせる。合わない値は入れずに `skipped` に残す。
public enum OpenFormPrefill {
    public struct Plan: Equatable, Sendable {
        /// 開く入力画面
        public var screen: String
        /// 入れる値(貼る先の field id → 値)
        public var values: [String: JSONValue]
        /// 入らなかった項目(渡したキー・名前順)
        public var skipped: [String]

        public init(screen: String, values: [String: JSONValue], skipped: [String]) {
            self.screen = screen
            self.values = values
            self.skipped = skipped
        }
    }

    public enum Failure: Error, Equatable, Sendable {
        /// そのアプリにその entity が無い
        case noEntity
        /// その entity の入力画面(`screen.entity`)が無い
        case noForm
    }

    /// entity の入力画面(DSL の順)
    public static func formScreens(for entityId: String, in dsl: AppDSL) -> [String] {
        dsl.screens.filter { $0.entity == entityId }.map(\.id)
    }

    /// 開く画面と入れる値を決める。入力画面が複数あれば DSL の先頭(コントロール #1993 と同じ)。
    /// `values` のキーは項目の id か名前。`isAdmin` が true でなければ管理者だけの項目には入れない。
    public static func plan(entityId: String, values: [String: JSONValue], in dsl: AppDSL, isAdmin: Bool = false) -> Result<Plan, Failure> {
        guard let entity = dsl.entity(entityId) else { return .failure(.noEntity) }
        guard let screen = formScreens(for: entityId, in: dsl).first else { return .failure(.noForm) }
        let targets = RecordClipboard.copyableFields(of: entity).filter { isAdmin || !$0.isAdminOnly }
        var out: [String: JSONValue] = [:]
        var skipped: [String] = []
        // id で合うものを先に入れる(名前で合わせる値に先に取られないように)
        let keys = values.keys.sorted()
        let byId = keys.filter { k in targets.contains { $0.id == k } }
        let byName = keys.filter { k in !byId.contains(k) }
        for key in byId + byName {
            guard let value = values[key] else { continue }
            let target = targets.first { $0.id == key && out[$0.id] == nil }
                ?? targets.first { out[$0.id] == nil && RecordClipboard.nameKey($0.name) == RecordClipboard.nameKey(key) }
            guard let target, let v = fill(value, into: target) else {
                skipped.append(key)
                continue
            }
            out[target.id] = v
        }
        return .success(Plan(screen: screen, values: out, skipped: skipped.sorted()))
    }

    /// 値を項目の型に合わせる: まず項目と同じ型として確かめ、だめなら値の形(文字・数・真偽)から変換する
    static func fill(_ value: JSONValue, into target: Field) -> JSONValue? {
        guard !value.isNull else { return nil }
        if let v = RecordClipboard.coerce(value, from: target.type.rawValue, to: target) { return v }
        let shape: FieldType? = switch value {
        case .string: .string
        case .number: .number
        case .bool: .boolean
        default: nil
        }
        return shape.flatMap { RecordClipboard.coerce(value, from: $0.rawValue, to: target) }
    }
}
