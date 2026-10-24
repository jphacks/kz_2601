import SwiftUI
import MetamonDSL

/// `picker`: enum field の options から選択。style(menu|segmented|wheel、省略時は 4 件以下なら segmented)。
struct PickerLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let options = form.entity.field(field)?.options ?? []
            let picker = Picker(c.label ?? "", selection: form.string(field)) {
                ForEach(options, id: \.self) { Text($0).tag($0) }
            }
            switch c.style ?? (options.count <= 4 ? "segmented" : "menu") {
            case "segmented":
                LabeledInput(label: c.label) { picker.pickerStyle(.segmented) }
            case "wheel":
                picker.pickerStyle(.wheel).frame(height: 120)
            default:
                // menu は List/Form の外ではラベルを描かないので、他の入力と同じく上にラベルを出す
                LabeledInput(label: c.label) { picker.pickerStyle(.menu).labelsHidden() }
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
