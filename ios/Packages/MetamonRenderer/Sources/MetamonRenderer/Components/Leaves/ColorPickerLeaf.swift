import SwiftUI
import MetamonDSL

/// `colorPicker`: システムのカラーウェル。string/text field に "#RRGGBB" で保存。
struct ColorPickerLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            ColorPicker(c.label ?? form.entity.field(field)?.name ?? field, selection: form.color(field), supportsOpacity: false)
        } else {
            NeedsForm(c: c)
        }
    }
}
