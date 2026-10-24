import SwiftUI
import MetamonDSL

/// `secureField`: 伏せ字の文字列入力(PIN・パスワード)。
struct SecureFieldLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            LabeledInput(label: c.label) {
                SecureField(c.placeholder ?? "", text: form.string(field))
                    .textContentType(.password)
                    .textFieldStyle(.roundedBorder)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
