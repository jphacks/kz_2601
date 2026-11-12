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
                    TextField(c.placeholder ?? "0", text: Self.text(form.number(field), c: c, type: ftype))
                        .numericKeyboard(decimal: ftype == .number)
                        .textFieldStyle(.plain)
                        .font(RenderTheme.Fonts.number)
                        .focused($focused)
                        .keyboardAnchor(c, $focused)
                        .macArrowStepping(form.number(field), c: c, type: ftype)
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

    /// iOS はそのまま。Mac は数字以外(型により小数点・マイナス)を入れた時点で落とす(#942)。
    static func text(_ binding: Binding<String>, c: Component, type: FieldType) -> Binding<String> {
        #if os(macOS)
        let decimal = type == .number
        let negative = (c.min ?? -1) < 0
        return Binding(get: { binding.wrappedValue },
                       set: { binding.wrappedValue = MacNumberInput.filter($0, allowsDecimal: decimal, allowsNegative: negative) })
        #else
        return binding
        #endif
    }

    /// 末尾の単位。currency は ¥ を前に出すので「円」等は落とし、「円/時」は「/時」だけ残す(#150)。
    static func unitText(_ unit: String?, isCurrency: Bool) -> String? {
        isCurrency ? FieldCoding.currencySuffix(unit: unit) : unit
    }
}

private extension View {
    /// Mac: ↑↓ で step ずつ増減(min/max に収める・#942)。iOS は何もしない。
    @ViewBuilder
    func macArrowStepping(_ binding: Binding<String>, c: Component, type: FieldType) -> some View {
        #if os(macOS)
        let decimal = type == .number
        onKeyPress(keys: [.upArrow, .downArrow]) { press in
            let dir = press.key == .upArrow ? 1 : -1
            binding.wrappedValue = MacNumberInput.stepped(binding.wrappedValue, direction: dir, min: c.min, max: c.max,
                                                          step: c.step, allowsDecimal: decimal)
            return .handled
        }
        #else
        self
        #endif
    }
}
