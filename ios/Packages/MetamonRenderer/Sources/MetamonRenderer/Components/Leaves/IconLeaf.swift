import SwiftUI
import MetamonDSL

/// `icon`: SF Symbol 1 個。size(small|medium|large)と semantic color。
struct IconLeaf: View {
    let c: Component

    var body: some View {
        Image(systemName: c.name ?? "questionmark.circle")
            .font(.system(size: c.symbolPointSize))
            .foregroundStyle(Color.semantic(c.color))
            .frame(maxWidth: .infinity)
    }
}
