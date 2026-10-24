import SwiftUI
import MetamonDSL

/// `numberField`: 数値入力。field 型が number なら小数キーパッド、unit は末尾に表示。
struct NumberFieldLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let ftype = form.entity.field(field)?.type ?? .number
            LabeledInput(label: c.label) {
                HStack {
                    TextField(c.placeholder ?? "0", text: form.number(field))
                        .keyboardType(ftype == .number ? .decimalPad : .numberPad)
                        .textFieldStyle(.roundedBorder)
                    if let unit = c.unit ?? form.entity.field(field)?.unit {
                        Text(unit).foregroundStyle(.secondary)
                    }
                }
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
