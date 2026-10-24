import SwiftUI
import MetamonDSL

/// `searchBar`: 画面ローカルの検索文字列(`field` キー)。同画面の `list.searchField` がこの値で絞り込む。
/// レコードには保存しない(`ScreenLocals`)。
struct SearchBarLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let locals = ctx.locals, let key = c.field {
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass").foregroundStyle(.secondary)
                TextField(c.placeholder ?? "検索", text: locals.text(key))
                    .textFieldStyle(.plain)
                    .autocorrectionDisabled()
                    .submitLabel(.search)
                if !(locals.values[key] ?? "").isEmpty {
                    Button { locals.values[key] = "" } label: {
                        Image(systemName: "xmark.circle.fill").foregroundStyle(.secondary)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 12))
        } else {
            UnsupportedComponent(label: "searchBar は field が必要です")
        }
    }
}
