import SwiftUI
import MetamonDSL

/// `datePicker`: field 型が datetime(or includeTime)なら時刻も。
struct DatePickerLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let includeTime = c.includeTime ?? (form.entity.field(field)?.type == .datetime)
            DatePicker(c.label ?? "", selection: form.date(field), displayedComponents: includeTime ? [.date, .hourAndMinute] : [.date])
        } else {
            NeedsForm(c: c)
        }
    }
}
