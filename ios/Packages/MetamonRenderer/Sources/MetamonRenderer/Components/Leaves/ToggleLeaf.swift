import SwiftUI
import MetamonDSL

/// `toggle`: boolean field の On/Off。
/// 見た目: 入力と同じ器の 1 行(アイコン + ラベル … ブランド色のスイッチ)。
struct ToggleLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let label = c.label ?? form.entity.field(field)?.name ?? field
            InputRow(icon: c.icon ?? "checkmark.circle", label: label) {
                Toggle("", isOn: form.bool(field))
                    .labelsHidden()
                    .tint(RenderTheme.Colors.brandA)
                    .accessibilityLabel(label)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
