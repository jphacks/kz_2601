import SwiftUI
import MetamonDSL

/// `slider`: 数値 field を min...max のスライダーで入力。integer field は step 既定 1。
/// 見た目: 上ラベル + 器の中にブランド色トラックのスライダー + 右に値(rounded 等幅・ブランド色)。
struct SliderLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            // min > max・step ≤ 0 も来うる(step 0 の Slider は刻みの数が無限になる)
            let bounds = NumericInput.range(min: c.min, max: c.max, defaultMin: 0, defaultMax: 100)
            let fieldDef = form.entity.field(field)
            let step = NumericInput.step(c.step) ?? (fieldDef?.type == .number ? nil : 1)
            let value = form.double(field, fallback: bounds.lowerBound)
            LabeledInput(label: c.label) {
                HStack(spacing: RenderTheme.Spacing.sm + 4) {
                    InputIcon(name: FieldIcon.infer(component: c, field: fieldDef))
                    Group {
                        if let step {
                            Slider(value: value, in: bounds, step: step)
                        } else {
                            Slider(value: value, in: bounds)
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
