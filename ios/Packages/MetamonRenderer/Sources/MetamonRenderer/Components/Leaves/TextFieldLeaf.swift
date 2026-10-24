import SwiftUI
import MetamonDSL

/// `textField`: FormDraft の `field` に文字列を束縛。multiline で複数行。
struct TextFieldLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            LabeledInput(label: c.label) {
                TextField(c.placeholder ?? "", text: form.string(field), axis: c.multiline == true ? .vertical : .horizontal)
                    .lineLimit(c.multiline == true ? 3 ... 8 : 1 ... 1)
                    .textFieldStyle(.roundedBorder)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
