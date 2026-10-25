import SwiftUI
import MetamonDSL

/// `timePicker`: 時:分のみ。field が datetime なら RFC3339(今日の日付)で、string/text なら "HH:mm" で保存。
/// 見た目: 入力と同じ器の 1 行(時計アイコン + ラベル … コンパクトピッカー)。
/// 未入力なら表示中の今の時刻を保存値にする(見えている値 = 保存される値・#229)。
struct TimePickerLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            let isDatetime = form.entity.field(field)?.type == .datetime
            let label = c.label ?? form.entity.field(field)?.name ?? ""
            InputRow(icon: c.icon ?? "clock", label: label) {
                DatePicker("", selection: isDatetime ? form.date(field) : form.time(field), displayedComponents: [.hourAndMinute])
                    .labelsHidden()
                    .tint(RenderTheme.Colors.brandA)
                    .accessibilityLabel(label)
            }
            .onAppear { form.seedTimeIfEmpty(field) }
        } else {
            NeedsForm(c: c)
        }
    }
}
