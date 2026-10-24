import SwiftUI
import MetamonDSL

/// `stepper`: 数値 field を -/+ で増減。min/max 省略時は実質無制限、step 既定 1。
/// 見た目: 入力と同じ器の 1 行(アイコン + ラベル … 値(等幅)+ ブランド色の -/+)。
struct StepperLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let lo = c.min ?? -1_000_000_000
            let hi = Swift.max(c.max ?? 1_000_000_000, lo + 0.001)
            let fieldDef = form.entity.field(field)
            let value = form.double(field, fallback: Swift.max(0, lo))
            let label = c.label ?? fieldDef?.name ?? field
            InputRow(icon: FieldIcon.infer(component: c, field: fieldDef), label: label) {
                Text(FieldCoding.display(.number(value.wrappedValue), field: fieldDef))
                    .font(.system(.body, design: .rounded).weight(.semibold).monospacedDigit())
                    .foregroundStyle(RenderTheme.Colors.brandText)
                    .contentTransition(.numericText())
                    .animation(RenderTheme.Motion.select, value: value.wrappedValue)
                Stepper("", value: value, in: lo...hi, step: c.step ?? 1)
                    .labelsHidden()
                    .tint(RenderTheme.Colors.brandA)
                    .accessibilityLabel(label)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
