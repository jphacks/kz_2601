import SwiftUI
import MetamonDSL

/// `textField`: FormDraft の `field` に文字列を束縛。
/// 複数行(`TextInputRules.isMultiline`)は文字に合わせて伸び return で改行、1 行は return で次の欄へ(#196)。
/// 見た目: 上ラベル + 左アイコン(field 型から推定)+ フォーカス時ブランド色リング。
struct TextFieldLeaf: View {
    let c: Component
    let ctx: RenderContext
    @FocusState private var focused: Bool

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let def = form.entity.field(field)
            let icon = FieldIcon.infer(component: c, field: def)
            let multiline = TextInputRules.isMultiline(component: c, field: def, titleField: form.entity.titleField)
            LabeledInput(label: c.label, focused: focused) {
                HStack(alignment: multiline ? .top : .center, spacing: RenderTheme.Spacing.sm + 2) {
                    InputIcon(name: icon, focused: focused)
                        .padding(.top, multiline ? 1 : 0) // 1 行目の文字の高さに合わせる
                    if multiline {
                        TextField(c.placeholder ?? "", text: form.string(field), axis: .vertical)
                            .lineLimit(TextInputRules.multilineLines)
                            .accessibilityIdentifier("field.\(field)")
                            .textFieldStyle(.plain)
                            .font(RenderTheme.Fonts.inputValue)
                            .focused($focused)
                            .keyboardAnchor(c, $focused)
                    } else {
                        TextField(c.placeholder ?? "", text: form.string(field))
                            .textFieldStyle(.plain)
                            .font(RenderTheme.Fonts.inputValue)
                            .singleLineSubmit()
                            .focused($focused)
                            .keyboardAnchor(c, $focused)
                    }
                }
                .renderInputChrome(focused: focused)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
