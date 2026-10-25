import SwiftUI
import MetamonDSL

/// `slider`: 数値 field を min...max のスライダーで入力。integer field は step 既定 1。
/// 見た目: 上ラベル + 器の中にブランド色トラックのスライダー + 右に値(rounded 等幅・ブランド色)。
struct SliderLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let lo = c.min ?? 0
            let hi = Swift.max(c.max ?? 100, lo + 0.001)
            let fieldDef = form.entity.field(field)
            let step = c.step ?? (fieldDef?.type == .number ? nil : 1)
            let value = form.double(field, fallback: lo)
            LabeledInput(label: c.label) {
                HStack(spacing: RenderTheme.Spacing.sm + 4) {
                    InputIcon(name: FieldIcon.infer(component: c, field: fieldDef))
                    Group {
                        if let step {
                            Slider(value: value, in: lo...hi, step: step)
                        } else {
                            Slider(value: value, in: lo...hi)
                        }
                    }
                    .tint(RenderTheme.Colors.brandA)
                    Text(displayText(.number(value.wrappedValue), field: fieldDef))
                        .font(.system(.body, design: .rounded).weight(.semibold).monospacedDigit())
                        .foregroundStyle(RenderTheme.Colors.brandText)
                        .frame(minWidth: 40, alignment: .trailing)
                        .contentTransition(.numericText())
                }
                .renderInputChrome()
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
