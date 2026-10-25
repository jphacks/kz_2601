import SwiftUI
import MetamonDSL

/// `textEditor`: 複数行の自由記述。minLines(既定 4)行ぶんの高さから文字に合わせて伸び、return で改行(#196)。
/// `TextEditor` は内側に独自の余白があり、重ねたプレースホルダと入力文字の位置がずれるので、縦に伸びる `TextField` で描く。
/// 見た目: 上ラベル + 左アイコン + フォーカス時ブランド色リング(textField と同じ器・余白)。
struct TextEditorLeaf: View {
    let c: Component
    let ctx: RenderContext
    @FocusState private var focused: Bool

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let icon = FieldIcon.infer(component: c, field: form.entity.field(field))
            LabeledInput(label: c.label, focused: focused) {
                HStack(alignment: .top, spacing: RenderTheme.Spacing.sm + 2) {
                    InputIcon(name: icon, focused: focused)
                        .padding(.top, 1) // 1 行目の文字の高さに合わせる
                    TextField(c.placeholder ?? "", text: form.string(field), axis: .vertical)
                        .lineLimit(TextInputRules.editorLines(minLines: c.minLines))
                        .accessibilityIdentifier("field.\(field)") // 複数行の欄は placeholder が XCUITest に出ないので id で探す
                        .textFieldStyle(.plain)
                        .font(RenderTheme.Fonts.inputValue)
                        .focused($focused)
                        .keyboardAnchor(c, $focused)
                }
                .renderInputChrome(focused: focused)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
