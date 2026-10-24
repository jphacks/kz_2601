import SwiftUI
import MetamonDSL

/// `rating`: 星 1〜max(既定 5)のタップ評価。同じ星をもう一度タップすると 0(未評価)に戻る。
/// 見た目: 上ラベル + 器の中にブランドグラデーションの塗り星(未選択は tertiary の輪郭)。タップで 0.97→1 のスプリング。
struct RatingLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let count = Swift.min(Swift.max(Int(c.max ?? 5), 1), 10)
            let value = form.double(field, fallback: 0)
            let current = Int(value.wrappedValue.rounded())
            LabeledInput(label: c.label) {
                HStack(spacing: RenderTheme.Spacing.sm) {
                    ForEach(1...count, id: \.self) { i in
                        Button {
                            withAnimation(RenderTheme.Motion.press) {
                                value.wrappedValue = (i == current) ? 0 : Double(i)
                            }
                        } label: {
                            Image(systemName: i <= current ? "star.fill" : "star")
                                .font(.title2.weight(.medium))
                                .foregroundStyle(i <= current ? AnyShapeStyle(RenderTheme.brandGradient) : AnyShapeStyle(.tertiary))
                                .scaleEffect(i == current ? 1.1 : 1)
                                .frame(minWidth: 32, minHeight: 32)
                                .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel("\(i)")
                    }
                    Spacer(minLength: 0)
                }
                .renderInputChrome()
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
