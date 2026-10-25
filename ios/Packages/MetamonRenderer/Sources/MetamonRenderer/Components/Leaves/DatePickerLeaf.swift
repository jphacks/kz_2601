import SwiftUI
import MetamonDSL

/// `datePicker`: field 型が datetime(or includeTime)なら時刻も。
/// 見た目: 入力と同じ器の 1 行(カレンダーアイコン + ラベル … コンパクトピッカー)。
struct DatePickerLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let fieldDef = form.entity.field(field)
            let includeTime = c.includeTime ?? (fieldDef?.type == .datetime)
            let label = c.label ?? fieldDef?.name ?? ""
            InputRow(icon: c.icon ?? (includeTime ? "calendar.badge.clock" : "calendar"), label: label) {
                DatePicker("", selection: form.date(field), displayedComponents: includeTime ? [.date, .hourAndMinute] : [.date])
                    .labelsHidden()
                    .tint(RenderTheme.Colors.brandA)
                    .accessibilityLabel(label)
            }
        } else {
            NeedsForm(c: c)
        }
    }
}
