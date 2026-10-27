import Foundation
import MetamonDSL

/// NavigationStack の path 要素。画面 id + 編集対象 recordId + どのスタック(main / tab:xxx / sheet / full)に属するか。
public struct Route: Hashable, Identifiable, Sendable {
    public var screen: String
    public var recordId: String?
    public var stack: String
    public var id: String { "\(stack)/\(screen)/\(recordId ?? "")" }

    public init(screen: String, recordId: String? = nil, stack: String = "main") {
        self.screen = screen
        self.recordId = recordId
        self.stack = stack
    }

    /// タブ用スタック名。
    public static func tabStack(_ screen: String) -> String { "tab:" + screen }
}

/// アクション実行時の文脈(どの画面・どのフォーム・どのレコードから起動されたか)。
public struct ActionContext {
    public var route: Route
    public var form: FormDraft?
    /// list の行タップ時に iOS が暗黙に渡す recordId
    public var recordId: String?

    public init(route: Route, form: FormDraft? = nil, recordId: String? = nil) {
        self.route = route
        self.form = form
        self.recordId = recordId
    }

    /// 今のレコード: 行タップならその行、フォーム画面の button / toolbar なら編集中のレコード(新規・未保存なら nil)。
    @MainActor public var currentRecordId: String? { recordId ?? form?.recordId }
}

/// 描画中の文脈。画面 route と(フォーム画面なら)ドラフト、画面ローカル状態(searchBar など)。
public struct RenderContext {
    public var route: Route
    public var form: FormDraft?
    /// 画面に閉じた UI 状態(`searchBar.field` → 検索文字列)。レコードには保存しない。
    public var locals: ScreenLocals?

    public init(route: Route, form: FormDraft? = nil, locals: ScreenLocals? = nil) {
        self.route = route
        self.form = form
        self.locals = locals
    }

    public func actionContext(recordId: String? = nil) -> ActionContext {
        ActionContext(route: route, form: form, recordId: recordId)
    }
}
