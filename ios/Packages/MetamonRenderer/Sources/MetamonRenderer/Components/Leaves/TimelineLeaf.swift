import SwiftUI
import MetamonDSL

/// `timeline`: dateField で新しい順に並べた縦の時系列(左に日付レール + ドット)。filter/limit はクライアント側(list と同じ経路)。
struct TimelineLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component

    var body: some View {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let dateField = c.dateField ?? ""
        let sort = Sort(field: dateField, direction: "desc")
        let all = Aggregator.sorted(Aggregator.apply(c.filter, entity: entity, to: rt.records(for: entityId)), by: sort, entity: entity)
        let rows = (c.limit ?? 0) > 0 ? Array(all.prefix(c.limit!)) : all
        if rows.isEmpty {
            let loading = rt.loading.contains(entityId)
            EmptyPlaceholder(icon: "clock", text: loading ? "読み込み中…" : "まだありません", loading: loading)
        } else {
            VStack(alignment: .leading, spacing: 0) {
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    HStack(alignment: .top, spacing: 12) {
                        Text(FieldCoding.display(row[dateField], field: entity?.field(dateField)))
                            .font(.caption.monospacedDigit())
                            .foregroundStyle(.secondary)
                            .frame(width: 84, alignment: .trailing)
                        VStack(spacing: 0) {
                            Circle().fill(RenderTheme.brandGradient).frame(width: 10, height: 10).padding(.top, 4)
                            if index < rows.count - 1 {
                                Rectangle().fill(RenderTheme.Colors.hairline).frame(width: 2).frame(maxHeight: .infinity)
                            }
                        }
                        VStack(alignment: .leading, spacing: 2) {
                            Text(FieldCoding.display(row[c.titleField ?? ""], field: entity?.field(c.titleField ?? ""))).font(.subheadline.weight(.semibold))
                            if let sub = c.subtitleField {
                                let s = FieldCoding.display(row[sub], field: entity?.field(sub))
                                if !s.isEmpty { Text(s).font(.caption).foregroundStyle(.secondary) }
                            }
                        }
                        .padding(.bottom, 16)
                        Spacer(minLength: 0)
                    }
                    .fixedSize(horizontal: false, vertical: true)
                }
            }
        .renderCard()
        }
    }
}
