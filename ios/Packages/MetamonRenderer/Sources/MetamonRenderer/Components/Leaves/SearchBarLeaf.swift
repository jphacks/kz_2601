import SwiftUI
import MetamonDSL

/// `searchBar`: 画面ローカルの検索文字列(`field` キー)。同画面の `list.searchField` がこの値で絞り込む。
/// レコードには保存しない(`ScreenLocals`)。見た目: 入力と同じ器(フォーカス時ブランド色リング)。
struct SearchBarLeaf: View {
    let c: Component
    let ctx: RenderContext
    @FocusState private var focused: Bool

    var body: some View {
        if let locals = ctx.locals, let key = c.field {
            HStack(spacing: RenderTheme.Spacing.sm + 2) {
                InputIcon(name: "magnifyingglass", focused: focused)
                TextField(c.placeholder ?? L("検索"), text: locals.text(key))
                    .textFieldStyle(.plain)
                    .font(RenderTheme.Fonts.inputValue)
                    .autocorrectionDisabled()
                    .submitLabel(.search)
                    .focused($focused)
                    .debugAutoFocus(c, $focused)
                if !(locals.values[key] ?? "").isEmpty {
                    Button { locals.values[key] = "" } label: {
                        Image(systemName: "xmark.circle.fill").foregroundStyle(.tertiary)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(L("クリア"))
                }
            }
            .renderInputChrome(focused: focused)
        } else {
            UnsupportedComponent(label: L("searchBar は field が必要です"))
        }
    }
}
