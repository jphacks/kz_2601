import SwiftUI
import MetamonDSL

/// `rating`: 星 1〜max(既定 5)のタップ評価。同じ星をもう一度タップすると 0(未評価)に戻る。
struct RatingLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let count = Swift.min(Swift.max(Int(c.max ?? 5), 1), 10)
            let value = form.double(field, fallback: 0)
            let current = Int(value.wrappedValue.rounded())
            LabeledInput(label: c.label) {
                HStack(spacing: 6) {
                    ForEach(1...count, id: \.self) { i in
                        Button {
                            value.wrappedValue = (i == current) ? 0 : Double(i)
                        } label: {
                            Image(systemName: i <= current ? "star.fill" : "star")
                                .font(.title2)
                                .foregroundStyle(i <= current ? Color.yellow : Color.secondary)
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel("\(i)")
                    }
                }
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
