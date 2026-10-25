import SwiftUI
import MetamonDSL

/// `numberField`: 数値入力。field 型が number なら小数キーパッド、unit は末尾に表示。
/// 見た目: 値は rounded・等幅で大きめ(金額が主役)。currency 型は `¥` プレフィックス(unit が「円」等なら unit は出さない・#150)。フォーカス時ブランド色リング。
struct NumberFieldLeaf: View {
    let c: Component
    let ctx: RenderContext
    @FocusState private var focused: Bool

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let fieldDef = form.entity.field(field)
            let ftype = fieldDef?.type ?? .number
            let isCurrency = ftype == .currency
            let icon = FieldIcon.infer(component: c, field: fieldDef)
            LabeledInput(label: c.label, focused: focused) {
                HStack(spacing: RenderTheme.Spacing.sm + 2) {
                    if !isCurrency {
                        InputIcon(name: icon, focused: focused)
                    }
                    if isCurrency {
                        Text("¥")
                            .font(RenderTheme.Fonts.numberPrefix)
                            .foregroundStyle(focused ? AnyShapeStyle(RenderTheme.Colors.brandText) : AnyShapeStyle(.secondary))
                            .accessibilityHidden(true)
                    }
                    TextField(c.placeholder ?? "0", text: form.number(field))
                        .keyboardType(ftype == .number ? .decimalPad : .numberPad)
                        .textFieldStyle(.plain)
                        .font(RenderTheme.Fonts.number)
                        .focused($focused)
                        .keyboardAnchor(c, $focused)
                    if let unit = Self.unitText(c.unit ?? fieldDef?.unit, isCurrency: isCurrency) {
                        Text(unit).font(.subheadline.weight(.medium)).foregroundStyle(.secondary)
                    }
                }
                .renderInputChrome(focused: focused)
            }
        } else {
            NeedsForm(c: c)
        }
    }

    /// 末尾の単位。currency は ¥ を前に出すので「円」等は落とし、「円/時」は「/時」だけ残す(#150)。
    static func unitText(_ unit: String?, isCurrency: Bool) -> String? {
        isCurrency ? FieldCoding.currencySuffix(unit: unit) : unit
    }
}
