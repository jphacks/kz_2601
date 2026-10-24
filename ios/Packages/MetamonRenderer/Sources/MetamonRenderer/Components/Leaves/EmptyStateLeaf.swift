import SwiftUI
import MetamonDSL

/// `emptyState`: アイコン + 見出し + 説明の空状態(ContentUnavailableView)。
struct EmptyStateLeaf: View {
    let c: Component

    var body: some View {
        ContentUnavailableView {
            Label(c.title ?? "", systemImage: c.icon ?? "tray")
                .foregroundStyle(.primary, RenderTheme.Colors.brandA.opacity(0.7))
        } description: {
            if let message = c.message { Text(message) }
        }
        .frame(maxWidth: .infinity)
    }
}
