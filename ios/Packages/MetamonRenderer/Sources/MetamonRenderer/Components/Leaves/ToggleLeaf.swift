import SwiftUI
import MetamonDSL

/// `toggle`: boolean field の On/Off。
struct ToggleLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            Toggle(c.label ?? "", isOn: form.bool(field))
        } else {
            NeedsForm(c: c)
        }
    }
}
