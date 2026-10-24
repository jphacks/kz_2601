import SwiftUI
import MetamonDSL

/// `badge`: 短いテキストの角丸ピル。color は semantic(背景は同色の薄塗り)。
struct BadgeLeaf: View {
    let c: Component

    var body: some View {
        let tint = Color.semantic(c.color)
        Text(c.text ?? "")
            .font(.caption.weight(.semibold))
            .lineLimit(1)
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .foregroundStyle(tint)
            .background(tint.opacity(0.15), in: Capsule())
    }
}
