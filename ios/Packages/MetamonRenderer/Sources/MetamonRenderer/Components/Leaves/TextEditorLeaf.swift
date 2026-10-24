import SwiftUI
import MetamonDSL

/// `textEditor`: 複数行の自由記述。minLines(既定 4)ぶんの高さを確保し、空ならプレースホルダを重ねる。
struct TextEditorLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let text = form.string(field)
            let minHeight = CGFloat(Swift.max(c.minLines ?? 4, 1)) * 22 + 16
            ZStack(alignment: .topLeading) {
                TextEditor(text: text)
                    .scrollContentBackground(.hidden)
                    .padding(4)
                if text.wrappedValue.isEmpty, let placeholder = c.placeholder, !placeholder.isEmpty {
                    Text(placeholder)
                        .foregroundStyle(.tertiary)
                        .padding(.top, 12)
                        .padding(.leading, 9)
                        .allowsHitTesting(false)
                }
            }
            .frame(minHeight: minHeight)
            .renderInputChrome(padding: false)
        } else {
            NeedsForm(c: c)
        }
    }
}
