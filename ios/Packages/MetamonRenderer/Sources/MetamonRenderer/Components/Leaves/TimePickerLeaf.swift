import SwiftUI
import MetamonDSL

/// `timePicker`: 時:分のみ。field が datetime なら RFC3339(今日の日付)で、string/text なら "HH:mm" で保存。
struct TimePickerLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let isDatetime = form.entity.field(field)?.type == .datetime
            DatePicker(c.label ?? "", selection: isDatetime ? form.date(field) : form.time(field), displayedComponents: [.hourAndMinute])
        } else {
            NeedsForm(c: c)
        }
    }
}
