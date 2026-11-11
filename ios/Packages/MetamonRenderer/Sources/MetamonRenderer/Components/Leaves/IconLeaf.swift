import SwiftUI
import MetamonDSL

/// `icon`: SF Symbol 1 個。size(small|medium|large)と semantic color。装飾なので VoiceOver からは隠す(読むべき意味は隣の text/label が持つ)。
/// Mac に無い名前は `questionmark.circle` にする(#941)。
struct IconLeaf: View {
    let c: Component

    var body: some View {
        Image(systemName: FieldIcon.resolve(c.name, fallback: "questionmark.circle"))
            .font(.system(size: c.symbolPointSize))
            .foregroundStyle(Color.semantic(c.color))
            .frame(maxWidth: .infinity)
            .accessibilityHidden(true)
    }
}
