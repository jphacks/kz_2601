import SwiftUI
import Observation
import MetamonDSL

/// 画面 1 枚に閉じた UI 状態(レコードに保存しない値)。`searchBar.field` の検索文字列など。
/// `ScreenView` が `@State` で 1 個持ち、`RenderContext.locals` で子に配る。
@MainActor
@Observable
public final class ScreenLocals {
    public var values: [String: String] = [:]

    public init() {}

    func text(_ key: String) -> Binding<String> {
        Binding(get: { self.values[key] ?? "" }, set: { self.values[key] = $0 })
    }
}

/// 「次に開くフォームへ初期値を渡す」ための受け渡し口(例: calendar の日付タップ → 新規フォームの dateField)。
/// `Action` は params を持たず、ホストの `navigate` も recordId しか運ばないため、Renderer 内で完結させる。
/// 直後に開いた新規フォーム(recordId 無し)が 1 回だけ消費する。数秒で失効するので古い値が別画面に漏れない。
@MainActor
public enum FormPrefill {
    private static var pending: (values: [String: JSONValue], at: Date)?
    private static let ttl: TimeInterval = 3

    public static func set(_ values: [String: JSONValue]) { pending = (values, Date()) }

    /// entity に存在する field だけ返し、消費する。
    static func consume(for entity: Entity) -> [String: JSONValue] {
        guard let p = pending else { return [:] }
        pending = nil
        guard Date().timeIntervalSince(p.at) < ttl else { return [:] }
        return p.values.filter { entity.field($0.key) != nil }
    }
}
