import SwiftUI
import MetamonDSL

/// `slider`: 数値 field を min...max のスライダーで入力。integer field は step 既定 1。
struct SliderLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let lo = c.min ?? 0
            let hi = Swift.max(c.max ?? 100, lo + 0.001)
            let step = c.step ?? (form.entity.field(field)?.type == .number ? nil : 1)
            let value = form.double(field, fallback: lo)
            LabeledInput(label: c.label) {
                HStack(spacing: 12) {
                    if let step {
                        Slider(value: value, in: lo...hi, step: step)
                    } else {
                        Slider(value: value, in: lo...hi)
                    }
                    Text(FieldCoding.display(.number(value.wrappedValue), field: form.entity.field(field)))
                        .font(.body.monospacedDigit())
                        .foregroundStyle(.secondary)
                        .frame(minWidth: 40, alignment: .trailing)
                }
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
