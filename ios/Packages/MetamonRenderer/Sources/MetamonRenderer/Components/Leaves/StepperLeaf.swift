import SwiftUI
import MetamonDSL

/// `stepper`: 数値 field を -/+ で増減。min/max 省略時は実質無制限、step 既定 1。
struct StepperLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let lo = c.min ?? -1_000_000_000
            let hi = Swift.max(c.max ?? 1_000_000_000, lo + 0.001)
            let value = form.double(field, fallback: Swift.max(0, lo))
            Stepper(value: value, in: lo...hi, step: c.step ?? 1) {
                HStack {
                    Text(c.label ?? form.entity.field(field)?.name ?? field)
                    Spacer()
                    Text(FieldCoding.display(.number(value.wrappedValue), field: form.entity.field(field)))
                        .font(.body.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
