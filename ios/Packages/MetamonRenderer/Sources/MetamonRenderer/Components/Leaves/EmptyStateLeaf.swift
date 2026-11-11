import SwiftUI
import MetamonDSL

/// `emptyState`: アイコン + 見出し + 説明の空状態(ContentUnavailableView)。Mac に無い記号は `tray` に落とす(#941)。
struct EmptyStateLeaf: View {
    let c: Component

    var body: some View {
        ContentUnavailableView {
            Label(c.title ?? "", systemImage: FieldIcon.resolve(c.icon, fallback: "tray"))
                .foregroundStyle(.primary, RenderTheme.Colors.brandA.opacity(0.7))
        } description: {
            if let message = c.message { Text(message) }
        }
        .frame(maxWidth: .infinity)
        .macTextSelection()
    }
}
