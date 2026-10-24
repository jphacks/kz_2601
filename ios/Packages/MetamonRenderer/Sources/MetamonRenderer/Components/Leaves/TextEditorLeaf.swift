import SwiftUI
import MetamonDSL

/// `textEditor`: 複数行の自由記述。minLines(既定 4)ぶんの高さを確保し、空ならプレースホルダを重ねる。
/// 見た目: 上ラベル + 左上アイコン + フォーカス時ブランド色リング。
struct TextEditorLeaf: View {
    let c: Component
    let ctx: RenderContext
    @FocusState private var focused: Bool

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let text = form.string(field)
            let minHeight = CGFloat(Swift.max(c.minLines ?? 4, 1)) * 22 + 16
            let icon = FieldIcon.infer(component: c, field: form.entity.field(field))
            LabeledInput(label: c.label, focused: focused) {
                HStack(alignment: .top, spacing: RenderTheme.Spacing.sm + 2) {
                    InputIcon(name: icon, focused: focused)
                        .padding(.top, RenderTheme.Spacing.inputV + 2)
                        .padding(.leading, RenderTheme.Spacing.inputH)
                    ZStack(alignment: .topLeading) {
                        TextEditor(text: text)
                            .scrollContentBackground(.hidden)
                            .font(RenderTheme.Fonts.inputValue)
                            .padding(.vertical, 4)
                            .focused($focused)
                            .keyboardAnchor(c, $focused)
                        if text.wrappedValue.isEmpty, let placeholder = c.placeholder, !placeholder.isEmpty {
                            Text(placeholder)
                                .font(RenderTheme.Fonts.inputValue)
                                .foregroundStyle(.tertiary)
                                .padding(.top, 12)
                                .padding(.leading, 5)
                                .allowsHitTesting(false)
                        }
                    }
                    .padding(.trailing, RenderTheme.Spacing.sm)
                }
                .frame(minHeight: minHeight)
                .renderInputChrome(padding: false, focused: focused)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
