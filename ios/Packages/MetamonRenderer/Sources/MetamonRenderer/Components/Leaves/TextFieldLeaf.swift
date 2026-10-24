import SwiftUI
import MetamonDSL

/// `textField`: FormDraft の `field` に文字列を束縛。multiline で複数行。
/// 見た目: 上ラベル + 左アイコン(field 型から推定)+ フォーカス時ブランド色リング。
struct TextFieldLeaf: View {
    let c: Component
    let ctx: RenderContext
    @FocusState private var focused: Bool

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let icon = FieldIcon.infer(component: c, field: form.entity.field(field))
            LabeledInput(label: c.label, focused: focused) {
                HStack(alignment: c.multiline == true ? .top : .center, spacing: RenderTheme.Spacing.sm + 2) {
                    InputIcon(name: icon, focused: focused)
                        .padding(.top, c.multiline == true ? 2 : 0)
                    TextField(c.placeholder ?? "", text: form.string(field), axis: c.multiline == true ? .vertical : .horizontal)
                        .lineLimit(c.multiline == true ? 3 ... 8 : 1 ... 1)
                        .textFieldStyle(.plain)
                        .font(RenderTheme.Fonts.inputValue)
                        .focused($focused)
                }
                .renderInputChrome(focused: focused)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
