import SwiftUI
import MetamonDSL

/// `badge`: 短いテキストの角丸ピル。color は semantic(背景は同色の薄塗り)。
/// 文字色は `SemanticPalette` のトーン(薄塗りピルの上でも 4.5:1 以上)。Dynamic Type では折り返さず縮小して 1 行に収める。
struct BadgeLeaf: View {
    let c: Component

    var body: some View {
        let tint = Color.semantic(c.color)
        Text(c.text ?? "")
            .font(.caption.weight(.semibold))
            .lineLimit(1)
            .minimumScaleFactor(0.75)
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .foregroundStyle(tint)
            .background(tint.opacity(SemanticPalette.badgeFill), in: Capsule())
    }
}
