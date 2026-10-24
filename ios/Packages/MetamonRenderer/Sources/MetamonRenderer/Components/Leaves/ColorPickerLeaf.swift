import SwiftUI
import MetamonDSL

/// `colorPicker`: システムのカラーウェル。string/text field に "#RRGGBB" で保存。
/// 見た目: 入力と同じ器の 1 行(パレットアイコン + ラベル … カラーウェル)。
struct ColorPickerLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let label = c.label ?? form.entity.field(field)?.name ?? field
            InputRow(icon: c.icon ?? "paintpalette", label: label) {
                ColorPicker("", selection: form.color(field), supportsOpacity: false)
                    .labelsHidden()
                    .accessibilityLabel(label)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
