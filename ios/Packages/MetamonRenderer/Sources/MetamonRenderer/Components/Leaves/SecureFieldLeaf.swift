import SwiftUI
import MetamonDSL

/// `secureField`: 伏せ字の文字列入力(PIN・パスワード)。
/// 見た目: 上ラベル + 鍵アイコン + フォーカス時ブランド色リング。
struct SecureFieldLeaf: View {
    let c: Component
    let ctx: RenderContext
    @FocusState private var focused: Bool

    var body: some View {
        if let form = ctx.form, let field = c.field {
            LabeledInput(label: c.label, focused: focused) {
                HStack(spacing: RenderTheme.Spacing.sm + 2) {
                    InputIcon(name: c.icon ?? "lock", focused: focused)
                    SecureField(c.placeholder ?? "", text: form.string(field))
                        .textContentType(.password)
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
